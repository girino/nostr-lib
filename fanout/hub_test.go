package fanout

import (
	"testing"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

func TestTrySendDropsWhenFull(t *testing.T) {
	w := &clientWriter{ch: make(chan nostr.EventEnvelope, 1), ws: &khatru.WebSocket{}}
	if !w.trySend(nostr.EventEnvelope{}) {
		t.Fatal("first send")
	}
	if w.trySend(nostr.EventEnvelope{}) {
		t.Fatal("full queue should drop only that client")
	}
}

func TestBroadcastMatchesFilter(t *testing.T) {
	h := &Hub{writers: make(map[*khatru.WebSocket]*clientWriter), writeQueue: 4}
	ws := &khatru.WebSocket{}
	w := &clientWriter{ws: ws, ch: make(chan nostr.EventEnvelope, 4)}
	h.writers[ws] = w
	h.subs = []liveSub{{ws: ws, id: "sub1", filter: nostr.Filter{Kinds: []int{1}}}}

	if n := h.BroadcastEvent(&nostr.Event{Kind: 1, ID: "a"}); n != 1 {
		t.Fatalf("kind1 matched=%d want 1", n)
	}
	if n := h.BroadcastEvent(&nostr.Event{Kind: 7, ID: "b"}); n != 0 {
		t.Fatalf("kind7 matched=%d want 0", n)
	}
	if !h.shouldSkipSyncWrite(ws) {
		t.Fatal("managed sockets should skip khatru sync writes")
	}
	if h.shouldSkipSyncWrite(&khatru.WebSocket{}) {
		t.Fatal("unknown sockets should not skip")
	}
}

func TestRejectConnAtCap(t *testing.T) {
	h := &Hub{maxConns: 1}
	h.conns.Store(1)
	if !h.rejectConn(nil) {
		t.Fatal("expected reject at cap")
	}
	h.conns.Store(0)
	if h.rejectConn(nil) {
		t.Fatal("expected accept under cap")
	}
}
