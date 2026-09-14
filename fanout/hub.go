// Copyright (c) 2026 Girino Vey.
package fanout

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fiatjaf/khatru"
	jsonlib "github.com/girino/nostr-lib/json"
	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
)

type liveSub struct {
	ws     *khatru.WebSocket
	id     string
	filter nostr.Filter
}

type clientWriter struct {
	ws      *khatru.WebSocket
	ch      chan nostr.EventEnvelope
	busyAt  atomic.Int64
	skipped atomic.Int64
	closed  atomic.Bool
}

// Hub fans events out to khatru clients on per-socket queues.
type Hub struct {
	mu         sync.Mutex
	writers    map[*khatru.WebSocket]*clientWriter
	subs       []liveSub
	skipped    atomic.Int64
	slow       atomic.Int64
	kicked     atomic.Int64
	conns      atomic.Int64
	maxConns   int
	writeQueue int
	slowWrite  time.Duration
	stuckWrite time.Duration
	stop       chan struct{}
	stopOnce   sync.Once
}

// Attach installs fan-out hooks on relay and returns the hub.
func Attach(relay *khatru.Relay, opts ...Option) *Hub {
	h := &Hub{
		writers:    make(map[*khatru.WebSocket]*clientWriter),
		maxConns:   defaultMaxConnections,
		writeQueue: defaultWriteQueue,
		slowWrite:  defaultSlowWrite,
		stuckWrite: defaultStuckWrite,
		stop:       make(chan struct{}),
	}
	for _, opt := range opts {
		opt(h)
	}

	relay.RejectFilter = append(relay.RejectFilter, func(ctx context.Context, filter nostr.Filter) (bool, string) {
		h.noteREQ(ctx, filter)
		return false, ""
	})
	relay.OnDisconnect = append(relay.OnDisconnect, func(ctx context.Context) {
		h.removeWS(khatru.GetConnection(ctx))
		if n := h.conns.Add(-1); n < 0 {
			h.conns.Store(0)
		}
	})
	relay.OnConnect = append(relay.OnConnect, func(context.Context) {
		h.conns.Add(1)
	})
	relay.PreventBroadcast = append(relay.PreventBroadcast, func(ws *khatru.WebSocket, _ *nostr.Event) bool {
		return h.shouldSkipSyncWrite(ws)
	})
	relay.OnEventSaved = append(relay.OnEventSaved, func(_ context.Context, evt *nostr.Event) {
		h.BroadcastEvent(evt)
	})
	if h.maxConns > 0 {
		relay.RejectConnection = append(relay.RejectConnection, h.rejectConn)
	}
	go h.watchStuck()
	return h
}

func (h *Hub) rejectConn(r *http.Request) bool {
	if h.maxConns <= 0 || h.conns.Load() < int64(h.maxConns) {
		return false
	}
	from := ""
	if r != nil {
		from = khatru.GetIPFromRequest(r)
	}
	logging.Warn("fanout: connection rejected: at cap (%d), from=%s", h.maxConns, from)
	return true
}

// Close stops the stuck-write watchdog.
func (h *Hub) Close() {
	h.stopOnce.Do(func() { close(h.stop) })
}

// ListenerCount is the number of allowed REQs still associated with a socket.
func (h *Hub) ListenerCount() int64 {
	h.mu.Lock()
	n := int64(len(h.subs))
	h.mu.Unlock()
	return n
}

// GetStatsName implements stats.StatsProvider.
func (h *Hub) GetStatsName() string { return "fanout" }

// GetStats implements stats.StatsProvider.
func (h *Hub) GetStats() jsonlib.JsonEntity {
	obj := jsonlib.NewJsonObject()
	obj.Set("listeners", jsonlib.NewJsonValue(h.ListenerCount()))
	obj.Set("client_queue_skips", jsonlib.NewJsonValue(h.skipped.Load()))
	obj.Set("slow_writes", jsonlib.NewJsonValue(h.slow.Load()))
	obj.Set("slow_disconnects", jsonlib.NewJsonValue(h.kicked.Load()))
	obj.Set("connections", jsonlib.NewJsonValue(h.conns.Load()))
	return obj
}

func safeSubID(ctx context.Context) (id string) {
	defer func() {
		if recover() != nil {
			id = ""
		}
	}()
	return khatru.GetSubscriptionID(ctx)
}

func stringPtr(s string) *string { return &s }

func (h *Hub) noteREQ(ctx context.Context, filter nostr.Filter) {
	ws := khatru.GetConnection(ctx)
	if ws == nil {
		return
	}
	id := safeSubID(ctx)
	h.mu.Lock()
	h.subs = append(h.subs, liveSub{ws: ws, id: id, filter: filter})
	if _, ok := h.writers[ws]; !ok {
		w := &clientWriter{
			ws: ws,
			ch: make(chan nostr.EventEnvelope, h.writeQueue),
		}
		h.writers[ws] = w
		go w.loop(h)
	}
	h.mu.Unlock()
}

func (h *Hub) removeWS(ws *khatru.WebSocket) {
	if ws == nil {
		return
	}
	h.mu.Lock()
	kept := h.subs[:0]
	for _, s := range h.subs {
		if s.ws != ws {
			kept = append(kept, s)
		}
	}
	h.subs = kept
	if w, ok := h.writers[ws]; ok {
		w.close()
		delete(h.writers, ws)
	}
	h.mu.Unlock()
}

func (h *Hub) writerFor(ws *khatru.WebSocket) *clientWriter {
	h.mu.Lock()
	w := h.writers[ws]
	h.mu.Unlock()
	return w
}

func (h *Hub) shouldSkipSyncWrite(ws *khatru.WebSocket) bool {
	return h.writerFor(ws) != nil
}

// BroadcastEvent matches khatru.Relay.BroadcastEvent. It never blocks on a slow socket.
func (h *Hub) BroadcastEvent(evt *nostr.Event) int {
	if evt == nil {
		return 0
	}
	h.mu.Lock()
	subs := append([]liveSub(nil), h.subs...)
	h.mu.Unlock()

	matched := 0
	for i := range subs {
		s := &subs[i]
		if !s.filter.Matches(evt) {
			continue
		}
		w := h.writerFor(s.ws)
		if w == nil {
			continue
		}
		env := nostr.EventEnvelope{
			SubscriptionID: stringPtr(s.id),
			Event:          *evt,
		}
		if w.trySend(env) {
			matched++
		} else {
			h.skipped.Add(1)
		}
	}
	return matched
}

func (w *clientWriter) trySend(env nostr.EventEnvelope) bool {
	if w.closed.Load() {
		return false
	}
	select {
	case w.ch <- env:
		return true
	default:
		w.skipped.Add(1)
		return false
	}
}

func (w *clientWriter) close() {
	if w.closed.CompareAndSwap(false, true) {
		close(w.ch)
	}
}

func wsIP(ws *khatru.WebSocket) string {
	if ws == nil || ws.Request == nil {
		return ""
	}
	return khatru.GetIPFromRequest(ws.Request)
}

func (w *clientWriter) loop(h *Hub) {
	for env := range w.ch {
		w.busyAt.Store(time.Now().UnixNano())
		start := time.Now()
		err := w.ws.WriteJSON(env)
		dur := time.Since(start)
		w.busyAt.Store(0)

		ip := wsIP(w.ws)
		pk := w.ws.AuthedPublicKey
		if h.slowWrite > 0 && dur >= h.slowWrite {
			h.slow.Add(1)
			logging.Warn("fanout: slow websocket write: dur=%s ip=%s pubkey=%s queue=%d err=%v",
				dur.Round(time.Millisecond), ip, pk, len(w.ch), err)
		}
		if err != nil {
			logging.Info("fanout: websocket write failed: ip=%s pubkey=%s err=%v", ip, pk, err)
			disconnectClient(w.ws, "write failed")
			return
		}
		if h.stuckWrite > 0 && dur >= h.stuckWrite {
			h.kicked.Add(1)
			logging.Warn("fanout: disconnecting slow websocket: dur=%s ip=%s pubkey=%s",
				dur.Round(time.Millisecond), ip, pk)
			disconnectClient(w.ws, "slow websocket write")
			return
		}
	}
}

func (h *Hub) watchStuck() {
	if h.stuckWrite <= 0 {
		return
	}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			h.kickStuckWriters()
		}
	}
}

func (h *Hub) kickStuckWriters() {
	now := time.Now().UnixNano()
	h.mu.Lock()
	var stuck []*khatru.WebSocket
	for ws, w := range h.writers {
		at := w.busyAt.Load()
		if at == 0 {
			continue
		}
		blocked := time.Duration(now - at)
		if blocked < h.stuckWrite {
			continue
		}
		stuck = append(stuck, ws)
		logging.Warn("fanout: websocket write stuck for %s ip=%s pubkey=%s; disconnecting that client only",
			blocked.Round(time.Millisecond), wsIP(ws), ws.AuthedPublicKey)
	}
	h.mu.Unlock()
	for _, ws := range stuck {
		h.kicked.Add(1)
		disconnectClient(ws, "stuck websocket write")
	}
}
