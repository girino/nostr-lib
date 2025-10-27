// Copyright (c) 2025 Girino Vey.
//
// This software is licensed under Girino's Anarchist License (GAL).
// See LICENSE file for full license text.
// License available at: https://license.girino.org/
//
// BroadcastStore - Nostr eventstore that broadcasts events using BroadcastSystem.
package broadcaststore

import (
	"context"
	"sync/atomic"

	"github.com/girino/nostr-lib/broadcast"
	jsonlib "github.com/girino/nostr-lib/json"
	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
)

// BroadcastStore implements eventstore.Store interface for broadcasting events
type BroadcastStore struct {
	broadcastSystem *broadcast.BroadcastSystem

	// Stats tracking
	attempts               int64
	successes              int64
	failures               int64
	consecutiveFailures    int64
	maxConsecutiveFailures int64
}

// NewBroadcastStore creates a new BroadcastStore with the given configuration
func NewBroadcastStore(cfg *broadcast.Config, maxConsecutiveFailures int64) *BroadcastStore {
	bs := &BroadcastStore{
		broadcastSystem:        broadcast.NewBroadcastSystem(cfg),
		maxConsecutiveFailures: maxConsecutiveFailures,
	}
	return bs
}

// Init initializes the broadcast store and starts the broadcast system
func (bs *BroadcastStore) Init() error {
	logging.DebugMethod("broadcaststore", "Init", "Initializing broadcast store")
	bs.broadcastSystem.Start()
	return nil
}

// Close stops the broadcast system and cleans up resources
func (bs *BroadcastStore) Close() {
	logging.DebugMethod("broadcaststore", "Close", "Closing broadcast store")
	bs.broadcastSystem.Stop()
}

// GetBroadcastSystem returns the underlying broadcast system
func (bs *BroadcastStore) GetBroadcastSystem() *broadcast.BroadcastSystem {
	return bs.broadcastSystem
}

// SaveEvent broadcasts an event if it hasn't been cached recently
func (bs *BroadcastStore) SaveEvent(ctx context.Context, evt *nostr.Event) error {
	// Check if event is cached using broadcaster's cache
	if bs.broadcastSystem.IsEventCached(evt.ID) {
		logging.DebugMethod("broadcaststore", "SaveEvent", "Event %s is cached, skipping broadcast", evt.ID)
		return nil
	}

	// Increment attempts
	atomic.AddInt64(&bs.attempts, 1)

	// Broadcast the event
	bs.broadcastSystem.BroadcastEvent(evt)

	// For now, we consider broadcast as successful
	// In a real implementation, we might want feedback from the broadcaster
	atomic.AddInt64(&bs.successes, 1)
	atomic.StoreInt64(&bs.consecutiveFailures, 0)

	logging.DebugMethod("broadcaststore", "SaveEvent", "Broadcast event %s", evt.ID)
	return nil
}

// QueryEvents returns an empty closed channel since we don't store events locally
func (bs *BroadcastStore) QueryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
	logging.DebugMethod("broadcaststore", "QueryEvents", "QueryEvents called but returning empty channel")
	ch := make(chan *nostr.Event)
	close(ch)
	return ch, nil
}

// DeleteEvent broadcasts a delete event (kind 5)
func (bs *BroadcastStore) DeleteEvent(ctx context.Context, evt *nostr.Event) error {
	logging.DebugMethod("broadcaststore", "DeleteEvent", "DeleteEvent called for event %s", evt.ID)

	// For now, we just return nil as the event was already deleted
	// In a full implementation, we would broadcast a delete event (kind 5) here
	return nil
}

// ReplaceEvent replaces an event (atomically)
func (bs *BroadcastStore) ReplaceEvent(ctx context.Context, evt *nostr.Event) error {
	// For replaceable events, we just save the new event
	return bs.SaveEvent(ctx, evt)
}

// RejectEvent checks if an event should be rejected based on whether it's already cached
func (bs *BroadcastStore) RejectEvent(ctx context.Context, evt *nostr.Event) (bool, string) {
	// If event is already cached, reject it as a duplicate
	if bs.broadcastSystem.IsEventCached(evt.ID) {
		logging.DebugMethod("broadcaststore", "RejectEvent", "Event %s is already cached, rejecting as duplicate", evt.ID)
		return true, "duplicate: event already exists"
	}
	return false, ""
}

// GetStatsName returns the name of this stats provider
func (bs *BroadcastStore) GetStatsName() string {
	return "broadcaststore"
}

// GetStats returns stats as JsonEntity
func (bs *BroadcastStore) GetStats() jsonlib.JsonEntity {
	obj := jsonlib.NewJsonObject()

	// Only return our local BroadcastStore-specific stats
	// The broadcaster and manager stats are already registered globally
	// and will appear at the top level in GetAllStats()
	obj.Set("attempts", jsonlib.NewJsonValue(atomic.LoadInt64(&bs.attempts)))
	obj.Set("successes", jsonlib.NewJsonValue(atomic.LoadInt64(&bs.successes)))
	obj.Set("failures", jsonlib.NewJsonValue(atomic.LoadInt64(&bs.failures)))
	obj.Set("consecutive_failures", jsonlib.NewJsonValue(atomic.LoadInt64(&bs.consecutiveFailures)))

	// Add health state
	consecutiveFailures := atomic.LoadInt64(&bs.consecutiveFailures)
	maxFailures := atomic.LoadInt64(&bs.maxConsecutiveFailures)
	healthState := getHealthState(consecutiveFailures, maxFailures)
	isHealthy := consecutiveFailures < maxFailures

	obj.Set("health_state", jsonlib.NewJsonValue(healthState))
	obj.Set("is_healthy", jsonlib.NewJsonValue(isHealthy))

	return obj
}

// getHealthState determines the health state based on consecutive failures
func getHealthState(consecutiveFailures, maxFailures int64) string {
	threshold := maxFailures / 2 // yellow threshold at 50% of max
	if consecutiveFailures == 0 {
		return "GREEN"
	} else if consecutiveFailures <= threshold {
		return "YELLOW"
	} else if consecutiveFailures < maxFailures {
		return "YELLOW"
	}
	return "RED"
}
