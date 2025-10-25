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
	mirrorAttempts            int64
	mirrorSuccesses           int64
	mirrorFailures            int64
	consecutiveMirrorFailures int64
	// relay health tracking
	liveRelays int64
	deadRelays int64
}

// MirrorStats holds runtime counters for mirroring operations
type MirrorStats struct {
	MirroredEvents            int64  `json:"mirrored_events"`
	MirrorAttempts            int64  `json:"mirror_attempts"`
	MirrorSuccesses           int64  `json:"mirror_successes"`
	MirrorFailures            int64  `json:"mirror_failures"`
	ConsecutiveMirrorFailures int64  `json:"consecutive_mirror_failures"`
	MirrorHealthState         string `json:"mirror_health_state"`
	// Relay health statistics
	LiveRelays int64 `json:"live_relays"`
	DeadRelays int64 `json:"dead_relays"`
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

// Stats returns a snapshot of the MirrorManager counters
func (m *MirrorManager) Stats() MirrorStats {
	consecutiveMirrorFailures := atomic.LoadInt64(&m.consecutiveMirrorFailures)
	mirrorHealthState := m.getHealthState(consecutiveMirrorFailures)

	return MirrorStats{
		MirroredEvents:            atomic.LoadInt64(&m.mirroredEvents),
		MirrorAttempts:            atomic.LoadInt64(&m.mirrorAttempts),
		MirrorSuccesses:           atomic.LoadInt64(&m.mirrorSuccesses),
		MirrorFailures:            atomic.LoadInt64(&m.mirrorFailures),
		ConsecutiveMirrorFailures: consecutiveMirrorFailures,
		MirrorHealthState:         mirrorHealthState,
		LiveRelays:                atomic.LoadInt64(&m.liveRelays),
		DeadRelays:                atomic.LoadInt64(&m.deadRelays),
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

// StartMirroring begins continuous mirroring of events from query relays to the khatru relay
func (m *MirrorManager) StartMirroring(relay *khatru.Relay) error {
	if m.mirrorCtx != nil {
		// already started
		return nil
	}

	if len(m.queryUrls) == 0 {
		// No query relays configured - this is OK, relay can work without mirroring
		logging.DebugMethod("mirror", "StartMirroring", "no query relays configured, skipping mirroring")
		return nil
	}

	// Check connectivity to all query relays first
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
		// Query relays are configured but none are available - this is a fatal error
		return fmt.Errorf("no query relays are available (configured: %d)", len(m.queryUrls))
	}

	logging.DebugMethod("mirror", "StartMirroring", "starting event mirroring from %d query relays (%d/%d available)", len(m.queryUrls), liveCount, len(m.queryUrls))

	m.mirrorCtx, m.mirrorCancel = context.WithCancel(context.Background())

	// start single mirroring goroutine for all query relays
	go m.mirrorFromRelays(m.mirrorCtx, relay)

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

// mirrorFromRelays continuously mirrors events from all query relays
func (m *MirrorManager) mirrorFromRelays(ctx context.Context, relay *khatru.Relay) {
	logging.DebugMethod("mirror", "mirrorFromRelays", "starting mirror from %d query relays: %v", len(m.queryUrls), m.queryUrls)

	// create a filter that gets all events since now
	now := nostr.Now()
	filter := nostr.Filter{Since: &now}

	// subscribe to all query relays at once (handles deduplication)
	sub := m.pool.SubscribeMany(ctx, m.queryUrls, filter)

	// Start relay health monitoring goroutine
	go m.monitorRelayHealth(ctx)

	for {
		select {
		case <-ctx.Done():
			logging.DebugMethod("mirror", "mirrorFromRelays", "mirror from query relays stopped (context cancelled)")
			return
		case relayEvent, ok := <-sub:
			if !ok {
				logging.DebugMethod("mirror", "mirrorFromRelays", "mirror subscription closed")
				return
			}

			if relayEvent.Event != nil {
				// broadcast the event to all connected clients
				clientCount := relay.BroadcastEvent(relayEvent.Event)
				atomic.AddInt64(&m.mirroredEvents, 1)
				atomic.AddInt64(&m.mirrorSuccesses, 1)
				logging.DebugMethod("mirror", "mirrorFromRelays", "mirrored event %s from %s to %d clients", relayEvent.Event.ID, relayEvent.Relay, clientCount)
			}
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
