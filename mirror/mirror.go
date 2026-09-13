// Copyright (c) 2025 Girino Vey.
//
// This software is licensed under Girino's Anarchist License (GAL).
// See LICENSE file for full license text.
// License available at: https://license.girino.org/
//
// Mirror - Nostr relay mirroring functionality.
package mirror

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/girino/nostr-lib/fanout"
	jsonlib "github.com/girino/nostr-lib/json"
	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
)

// MirrorManager handles continuous mirroring of events from query relays to the khatru relay
type MirrorManager struct {
	// queryUrls are the remotes used for mirroring events
	queryUrls []string
	// pool manages connections for query remotes
	pool *nostr.SimplePool
	// mirroring state
	mirrorCtx      context.Context
	mirrorCancel   context.CancelFunc
	mirroredEvents int64
	// mirroring health tracking
	mirrorSuccesses           int64
	mirrorFailures            int64
	consecutiveMirrorFailures int64
	// relay health tracking
	liveRelays int64
	deadRelays int64
	// events dropped because BroadcastEvent was slower than ingest
	droppedEvents int64
	lastDropLog   int64 // unix seconds
	broadcast     func(*nostr.Event) int
	listeners     func() int64
}

// MirrorStats holds runtime counters for mirroring operations
type MirrorStats struct {
	MirroredEvents            int64  `json:"mirrored_events"`
	MirrorSuccesses           int64  `json:"mirror_successes"`
	MirrorFailures            int64  `json:"mirror_failures"`
	ConsecutiveMirrorFailures int64  `json:"consecutive_mirror_failures"`
	MirrorHealthState         string `json:"mirror_health_state"`
	// Relay health statistics
	LiveRelays    int64 `json:"live_relays"`
	DeadRelays    int64 `json:"dead_relays"`
	DroppedEvents int64 `json:"dropped_events"`
}

// Health state constants
const (
	HealthGreen  = "GREEN"
	HealthYellow = "YELLOW"
	HealthRed    = "RED"
)

// NewMirrorManager creates a new MirrorManager with the provided query URLs
func NewMirrorManager(queryUrls []string) *MirrorManager {
	return &MirrorManager{
		queryUrls: queryUrls,
	}
}

// Init initializes the mirror manager
func (m *MirrorManager) Init() error {
	// No default query remotes - must be provided
	if len(m.queryUrls) == 0 {
		return fmt.Errorf("no query remotes provided - mirror manager requires query remotes")
	}

	// create a SimplePool for queries
	m.pool = nostr.NewSimplePool(context.Background(), nostr.WithPenaltyBox())

	logging.DebugMethod("mirror", "Init", "query remotes: %v", m.queryUrls)
	return nil
}

// Close closes the mirror manager
func (m *MirrorManager) Close() {
	if m.mirrorCancel != nil {
		m.StopMirroring()
	}
}

// GetStatsName returns the name of this stats provider
func (m *MirrorManager) GetStatsName() string {
	return "mirror"
}

// GetStats returns stats as JsonEntity
func (m *MirrorManager) GetStats() jsonlib.JsonEntity {
	s := m.Stats()
	obj := jsonlib.NewJsonObject()
	obj.Set("mirrored_events", jsonlib.NewJsonValue(s.MirroredEvents))
	obj.Set("mirror_successes", jsonlib.NewJsonValue(s.MirrorSuccesses))
	obj.Set("mirror_failures", jsonlib.NewJsonValue(s.MirrorFailures))
	obj.Set("consecutive_mirror_failures", jsonlib.NewJsonValue(s.ConsecutiveMirrorFailures))
	obj.Set("mirror_health_state", jsonlib.NewJsonValue(s.MirrorHealthState))
	obj.Set("live_relays", jsonlib.NewJsonValue(s.LiveRelays))
	obj.Set("dead_relays", jsonlib.NewJsonValue(s.DeadRelays))
	obj.Set("dropped_events", jsonlib.NewJsonValue(s.DroppedEvents))
	return obj
}

// Stats returns a snapshot of the MirrorManager counters (kept for backward compatibility)
func (m *MirrorManager) Stats() MirrorStats {
	consecutiveMirrorFailures := atomic.LoadInt64(&m.consecutiveMirrorFailures)
	mirrorHealthState := m.getHealthState(consecutiveMirrorFailures)

	return MirrorStats{
		MirroredEvents:            atomic.LoadInt64(&m.mirroredEvents),
		MirrorSuccesses:           atomic.LoadInt64(&m.mirrorSuccesses),
		MirrorFailures:            atomic.LoadInt64(&m.mirrorFailures),
		ConsecutiveMirrorFailures: consecutiveMirrorFailures,
		MirrorHealthState:         mirrorHealthState,
		LiveRelays:                atomic.LoadInt64(&m.liveRelays),
		DeadRelays:                atomic.LoadInt64(&m.deadRelays),
		DroppedEvents:             atomic.LoadInt64(&m.droppedEvents),
	}
}

// getHealthState determines the health state based on consecutive failures
func (m *MirrorManager) getHealthState(consecutiveFailures int64) string {
	if consecutiveFailures <= 2 {
		return HealthGreen
	} else if consecutiveFailures < 10 {
		return HealthYellow
	}
	return HealthRed
}

// StartMirroring begins continuous mirroring using khatru.BroadcastEvent.
func (m *MirrorManager) StartMirroring(relay *khatru.Relay) error {
	return m.startMirroring(relay.BroadcastEvent, nil)
}

// StartMirroringHub is the same as StartMirroring but delivers via hub
// (per-client queues) and only pulls the firehose while hub.ListenerCount() > 0.
func (m *MirrorManager) StartMirroringHub(relay *khatru.Relay, hub *fanout.Hub) error {
	if hub == nil {
		return m.StartMirroring(relay)
	}
	return m.startMirroring(hub.BroadcastEvent, hub.ListenerCount)
}

func (m *MirrorManager) startMirroring(broadcast func(*nostr.Event) int, listeners func() int64) error {
	if m.mirrorCtx != nil {
		return nil
	}
	if len(m.queryUrls) == 0 {
		logging.DebugMethod("mirror", "StartMirroring", "no query relays configured, skipping mirroring")
		return nil
	}

	liveCount := 0
	for _, url := range m.queryUrls {
		_, err := m.pool.EnsureRelay(url)
		if err != nil {
			logging.DebugMethod("mirror", "StartMirroring", "failed initial connect to %s: %v", url, err)
		} else {
			liveCount++
		}
	}
	if liveCount == 0 {
		return fmt.Errorf("no query relays are available (configured: %d)", len(m.queryUrls))
	}

	logging.DebugMethod("mirror", "StartMirroring", "starting event mirroring from %d query relays (%d/%d available)", len(m.queryUrls), liveCount, len(m.queryUrls))

	m.broadcast = broadcast
	m.listeners = listeners
	m.mirrorCtx, m.mirrorCancel = context.WithCancel(context.Background())
	go m.mirrorFromRelays(m.mirrorCtx)
	return nil
}

// StopMirroring stops the continuous mirroring of events
func (m *MirrorManager) StopMirroring() {
	if m.mirrorCancel != nil {
		logging.DebugMethod("mirror", "StopMirroring", "stopping event mirroring")
		m.mirrorCancel()
		m.mirrorCtx = nil
		m.mirrorCancel = nil
	}
}

// ingestQueueSize bounds events waiting for BroadcastEvent. go-nostr
// dispatchEvent spawns a goroutine per EVENT on an unbuffered channel; if
// BroadcastEvent blocks (slow websocket), those goroutines grow without bound.
const ingestQueueSize = 4096

const dropLogInterval = 10 * time.Second

func enqueueMirrorEvent(ch chan *nostr.Event, evt *nostr.Event) bool {
	select {
	case ch <- evt:
		return true
	default:
		return false
	}
}

func (m *MirrorManager) noteDrop() {
	n := atomic.AddInt64(&m.droppedEvents, 1)
	now := time.Now().Unix()
	last := atomic.LoadInt64(&m.lastDropLog)
	if last != 0 && now-last < int64(dropLogInterval.Seconds()) {
		return
	}
	if atomic.CompareAndSwapInt64(&m.lastDropLog, last, now) {
		logging.Warn("mirror ingest queue full; dropped %d events total (BroadcastEvent backpressure)", n)
	}
}

const idlePollInterval = 250 * time.Millisecond

// mirrorFromRelays continuously mirrors events from all query relays
func (m *MirrorManager) mirrorFromRelays(ctx context.Context) {
	logging.DebugMethod("mirror", "mirrorFromRelays", "starting mirror from %d query relays: %v", len(m.queryUrls), m.queryUrls)

	go m.monitorRelayHealth(ctx)

	queue := make(chan *nostr.Event, ingestQueueSize)
	go m.broadcastLoop(ctx, queue)

	if m.listeners == nil {
		m.ingestSubscription(ctx, queue)
		return
	}

	for ctx.Err() == nil {
		if m.listeners() == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(idlePollInterval):
				continue
			}
		}
		subCtx, cancel := context.WithCancel(ctx)
		go func() {
			t := time.NewTicker(idlePollInterval)
			defer t.Stop()
			for {
				select {
				case <-subCtx.Done():
					return
				case <-t.C:
					if m.listeners() == 0 {
						cancel()
						return
					}
				}
			}
		}()
		m.ingestSubscription(subCtx, queue)
		cancel()
	}
}

func (m *MirrorManager) ingestSubscription(ctx context.Context, queue chan *nostr.Event) {
	now := nostr.Now()
	filter := nostr.Filter{Since: &now}
	sub := m.pool.SubscribeMany(ctx, m.queryUrls, filter)
	for {
		select {
		case <-ctx.Done():
			return
		case relayEvent, ok := <-sub:
			if !ok {
				return
			}
			if relayEvent.Event == nil {
				continue
			}
			if m.listeners != nil && m.listeners() == 0 {
				continue
			}
			if !enqueueMirrorEvent(queue, relayEvent.Event) {
				m.noteDrop()
			}
		}
	}
}

func (m *MirrorManager) broadcastLoop(ctx context.Context, queue chan *nostr.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-queue:
			if m.listeners != nil && m.listeners() == 0 {
				continue
			}
			n := 0
			if m.broadcast != nil {
				n = m.broadcast(evt)
			}
			atomic.AddInt64(&m.mirroredEvents, 1)
			atomic.AddInt64(&m.mirrorSuccesses, 1)
			logging.DebugMethod("mirror", "broadcastLoop", "mirrored event %s to %d clients", evt.ID, n)
		}
	}
}

// monitorRelayHealth periodically checks the health of all query relays
func (m *MirrorManager) monitorRelayHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkRelayHealth()
		}
	}
}

// checkRelayHealth checks each relay and updates health counters
func (m *MirrorManager) checkRelayHealth() {
	if len(m.queryUrls) == 0 {
		return
	}

	deadCount := int64(0)

	for _, url := range m.queryUrls {
		_, err := m.pool.EnsureRelay(url)
		if err != nil {
			deadCount++
			logging.DebugMethod("mirror", "monitorRelayHealth", "relay %s is dead: %v", url, err)
		}
	}

	// Calculate live count from total and dead
	totalRelays := int64(len(m.queryUrls))
	liveCount := totalRelays - deadCount

	// Update counters
	atomic.StoreInt64(&m.liveRelays, liveCount)
	atomic.StoreInt64(&m.deadRelays, deadCount)

	// Check if more than half are dead
	threshold := totalRelays / 2

	if deadCount > threshold {
		// More than half are dead - count as failure
		atomic.AddInt64(&m.mirrorFailures, 1)
		atomic.AddInt64(&m.consecutiveMirrorFailures, 1)
		logging.DebugMethod("mirror", "monitorRelayHealth", "mirror health check failed: %d/%d relays dead", deadCount, totalRelays)
	} else {
		// Half or less are dead (more than half are alive) - reset failures
		atomic.StoreInt64(&m.consecutiveMirrorFailures, 0)
		logging.DebugMethod("mirror", "monitorRelayHealth", "mirror health check passed: %d/%d relays alive", liveCount, totalRelays)
	}
}
