package broadcaster

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
)

// BroadcasterStats represents broadcaster statistics
type BroadcasterStats struct {
	MandatoryRelays int        `json:"mandatory_relays"`
	Queue           QueueStats `json:"queue"`
	Cache           CacheStats `json:"cache"`
}

// QueueStats represents queue statistics
type QueueStats struct {
	WorkerCount        int     `json:"worker_count"`
	ChannelSize        int     `json:"channel_size"`
	ChannelCapacity    int     `json:"channel_capacity"`
	ChannelUtilization float64 `json:"channel_utilization"`
	OverflowSize       int     `json:"overflow_size"`
	TotalQueued        int64   `json:"total_queued"`
	PeakSize           int64   `json:"peak_size"`
	SaturationCount    int64   `json:"saturation_count"`
	IsSaturated        bool    `json:"is_saturated"`
	LastSaturation     string  `json:"last_saturation"`
}

// CacheStats represents cache statistics
type CacheStats struct {
	Size           int     `json:"size"`
	MaxSize        int     `json:"max_size"`
	UtilizationPct float64 `json:"utilization_pct"`
	Hits           int64   `json:"hits"`
	Misses         int64   `json:"misses"`
	HitRatePct     float64 `json:"hit_rate_pct"`
}

type cacheEntry struct {
	timestamp time.Time
}

type Broadcaster struct {
	relayProvider   RelayProvider
	resultTracker   PublishResultTracker
	mandatoryRelays []string
	eventQueue      chan *nostr.Event
	overflowQueue   []*nostr.Event
	overflowMutex   sync.Mutex
	channelCapacity int
	totalQueued     int64
	peakQueueSize   int64
	saturationCount int64
	lastSaturation  time.Time
	workerCount     int
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	// Event deduplication cache
	eventCache   map[string]cacheEntry
	cacheMutex   sync.RWMutex
	cacheMaxSize int
	cacheTTL     time.Duration
	cacheHits    int64
	cacheMisses  int64
}

func NewBroadcaster(relayProvider RelayProvider, resultTracker PublishResultTracker, mandatoryRelays []string, workerCount int, cacheTTL time.Duration) *Broadcaster {
	logging.DebugMethod("broadcaster", "NewBroadcaster", "Initializing broadcaster with %d workers", workerCount)
	if len(mandatoryRelays) > 0 {
		logging.Info("Broadcaster: Configured with %d mandatory relays", len(mandatoryRelays))
	}

	ctx, cancel := context.WithCancel(context.Background())
	channelCapacity := workerCount * 10
	cacheMaxSize := 100000 // ~10MB: 100K event IDs @ ~100 bytes each

	logging.Info("Broadcaster: Channel capacity set to %d (10 * %d workers)", channelCapacity, workerCount)
	logging.Info("Broadcaster: Event cache initialized with max size %d (~10MB), TTL %v", cacheMaxSize, cacheTTL)

	return &Broadcaster{
		relayProvider:   relayProvider,
		resultTracker:   resultTracker,
		mandatoryRelays: mandatoryRelays,
		eventQueue:      make(chan *nostr.Event, channelCapacity),
		overflowQueue:   make([]*nostr.Event, 0),
		channelCapacity: channelCapacity,
		totalQueued:     0,
		peakQueueSize:   0,
		saturationCount: 0,
		workerCount:     workerCount,
		ctx:             ctx,
		cancel:          cancel,
		eventCache:      make(map[string]cacheEntry),
		cacheMaxSize:    cacheMaxSize,
		cacheTTL:        cacheTTL,
		cacheHits:       0,
		cacheMisses:     0,
	}
}

// Start initializes and starts the worker pool
func (b *Broadcaster) Start() {
	logging.Info("Broadcaster: Starting %d workers", b.workerCount)
	for i := 0; i < b.workerCount; i++ {
		b.wg.Add(1)
		go b.worker(i)
	}

	// Start cache cleanup goroutine
	b.wg.Add(1)
	go b.cacheCleanup()
}

// Stop gracefully shuts down the worker pool
func (b *Broadcaster) Stop() {
	logging.Info("Broadcaster: Stopping worker pool")
	b.cancel()
	close(b.eventQueue)
	b.wg.Wait()
	logging.Info("Broadcaster: All workers stopped")
}

// worker processes events from the queue
func (b *Broadcaster) worker(id int) {
	defer b.wg.Done()
	logging.DebugMethod("broadcaster", "worker", "Worker %d started", id)

	for {
		select {
		case <-b.ctx.Done():
			logging.DebugMethod("broadcaster", "worker", "Worker %d shutting down (context cancelled)", id)
			return
		case event, ok := <-b.eventQueue:
			if !ok {
				logging.DebugMethod("broadcaster", "worker", "Worker %d shutting down (queue closed)", id)
				return
			}
			// Decrement total queued count
			atomic.AddInt64(&b.totalQueued, -1)

			// Try to backfill from overflow
			b.backfillChannel()

			// Broadcast the event
			b.broadcastEvent(event)
		}
	}
}

// backfillChannel attempts to move events from overflow queue to channel
func (b *Broadcaster) backfillChannel() {
	b.overflowMutex.Lock()
	defer b.overflowMutex.Unlock()

	// Move events from overflow to channel while there's space and overflow has events
	for len(b.overflowQueue) > 0 {
		select {
		case b.eventQueue <- b.overflowQueue[0]:
			// Successfully moved to channel, remove from overflow
			b.overflowQueue = b.overflowQueue[1:]
		default:
			// Channel is full, stop trying
			return
		}
	}
}

// isEventCached checks if an event has already been broadcast and not expired
func (b *Broadcaster) isEventCached(eventID string) bool {
	b.cacheMutex.RLock()
	defer b.cacheMutex.RUnlock()

	entry, exists := b.eventCache[eventID]
	if !exists {
		atomic.AddInt64(&b.cacheMisses, 1)
		return false
	}

	// Check if entry has expired
	if time.Since(entry.timestamp) > b.cacheTTL {
		atomic.AddInt64(&b.cacheMisses, 1)
		return false
	}

	atomic.AddInt64(&b.cacheHits, 1)
	return true
}

// IsEventCached checks if an event ID is in the cache (public method for relay)
func (b *Broadcaster) IsEventCached(eventID string) bool {
	return b.isEventCached(eventID)
}

// cacheCleanup periodically removes expired entries from the cache
func (b *Broadcaster) cacheCleanup() {
	defer b.wg.Done()

	// Run cleanup every 1/10th of the TTL or every 5 minutes, whichever is less
	cleanupInterval := b.cacheTTL / 10
	if cleanupInterval > 5*time.Minute {
		cleanupInterval = 5 * time.Minute
	}
	if cleanupInterval < 30*time.Second {
		cleanupInterval = 30 * time.Second
	}

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	logging.DebugMethod("broadcaster", "cacheCleanup", "Cache cleanup started, interval: %v", cleanupInterval)

	for {
		select {
		case <-b.ctx.Done():
			logging.DebugMethod("broadcaster", "cacheCleanup", "Cache cleanup shutting down")
			return
		case <-ticker.C:
			b.cacheMutex.Lock()
			now := time.Now()
			removed := 0
			for key, entry := range b.eventCache {
				if now.Sub(entry.timestamp) > b.cacheTTL {
					delete(b.eventCache, key)
					removed++
				}
			}
			b.cacheMutex.Unlock()

			if removed > 0 {
				logging.DebugMethod("broadcaster", "cacheCleanup", "Removed %d expired entries (cache size: %d)", removed, len(b.eventCache))
			}
		}
	}
}

// addEventToCache adds an event ID to the cache with current timestamp
func (b *Broadcaster) addEventToCache(eventID string) {
	b.cacheMutex.Lock()
	defer b.cacheMutex.Unlock()

	logging.DebugMethod("broadcaster", "addEventToCache", "Adding event %s to cache (current size: %d)", eventID, len(b.eventCache))

	// Check if cache is at max capacity
	if len(b.eventCache) >= b.cacheMaxSize {
		// Clear 20% of the oldest entries
		toRemove := b.cacheMaxSize / 5
		removed := 0
		for key := range b.eventCache {
			delete(b.eventCache, key)
			removed++
			if removed >= toRemove {
				break
			}
		}
		logging.Info("Broadcaster: Cache full, removed %d old entries (cache size: %d)", removed, len(b.eventCache))
		logging.DebugMethod("broadcaster", "addEventToCache", "Cache cleanup complete, removed %d entries", removed)
	}

	b.eventCache[eventID] = cacheEntry{
		timestamp: time.Now(),
	}
}

// Broadcast enqueues an event for broadcasting
func (b *Broadcaster) Broadcast(event *nostr.Event) {
	// Check if shutting down
	select {
	case <-b.ctx.Done():
		logging.Warn("Broadcaster: Cannot queue event %s, broadcaster is shutting down", event.ID)
		return
	default:
	}

	// Add to cache (should not be cached yet since relay rejects duplicates)
	b.addEventToCache(event.ID)

	// Try to add to channel first (fast path)
	select {
	case b.eventQueue <- event:
		// Successfully queued to channel
		newTotal := atomic.AddInt64(&b.totalQueued, 1)
		logging.DebugMethod("broadcaster", "Broadcast", "Event %s (kind %d) queued to channel (total: %d)",
			event.ID, event.Kind, newTotal)

		// Update peak size
		for {
			peak := atomic.LoadInt64(&b.peakQueueSize)
			if newTotal <= peak || atomic.CompareAndSwapInt64(&b.peakQueueSize, peak, newTotal) {
				break
			}
		}
		return
	default:
		// Channel is full, add to overflow queue (slow path)
		b.overflowMutex.Lock()
		defer b.overflowMutex.Unlock()

		b.overflowQueue = append(b.overflowQueue, event)
		newTotal := atomic.AddInt64(&b.totalQueued, 1)

		// Track saturation
		if len(b.overflowQueue) == 1 {
			// First overflow, log warning
			atomic.AddInt64(&b.saturationCount, 1)
			b.lastSaturation = time.Now()
			logging.Warn("Broadcaster: Channel saturated (%d/%d), using overflow queue",
				len(b.eventQueue), b.channelCapacity)
		}

		logging.DebugMethod("broadcaster", "Broadcast", "Event %s (kind %d) queued to overflow (overflow: %d, total: %d)",
			event.ID, event.Kind, len(b.overflowQueue), newTotal)

		// Update peak size
		for {
			peak := atomic.LoadInt64(&b.peakQueueSize)
			if newTotal <= peak || atomic.CompareAndSwapInt64(&b.peakQueueSize, peak, newTotal) {
				break
			}
		}
	}
}

// broadcastEvent sends an event to the top N relays concurrently
func (b *Broadcaster) broadcastEvent(event *nostr.Event) {
	topRelayURLs := b.relayProvider.GetBroadcastRelays()

	// Build complete relay list: mandatory + top N (deduplicated)
	relayURLs := make(map[string]bool)

	// Add mandatory relays first
	for _, url := range b.mandatoryRelays {
		relayURLs[url] = true
	}

	// Add top N relays
	for _, url := range topRelayURLs {
		relayURLs[url] = true
	}

	// Convert to slice
	broadcastRelays := make([]string, 0, len(relayURLs))
	for url := range relayURLs {
		broadcastRelays = append(broadcastRelays, url)
	}

	if len(broadcastRelays) == 0 {
		logging.Warn("Broadcaster: No relays available for broadcasting event %s (kind %d)", event.ID, event.Kind)
		return
	}

	logging.DebugMethod("broadcaster", "broadcastEvent", "Broadcasting event %s (kind %d) to %d relays (%d mandatory + %d top)",
		event.ID, event.Kind, len(broadcastRelays), len(b.mandatoryRelays), len(topRelayURLs))

	var wg sync.WaitGroup
	successCount := 0
	failCount := 0
	var mu sync.Mutex

	for _, url := range broadcastRelays {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			success := b.publishToRelay(u, event)
			mu.Lock()
			if success {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()
		}(url)
	}

	// Track results in background
	go func() {
		wg.Wait()
		logging.DebugMethod("broadcaster", "broadcastEvent", "Broadcast complete for event %s | success=%d, failed=%d, total=%d",
			event.ID, successCount, failCount, len(broadcastRelays))
	}()
}

// publishToRelay publishes an event to a single relay and tracks the result
func (b *Broadcaster) publishToRelay(url string, event *nostr.Event) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()

	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		logging.DebugMethod("broadcaster", "publishToRelay", "Failed to connect to %s: %v", url, err)
		// Track publish result
		if b.resultTracker != nil {
			b.resultTracker.TrackPublishResult(url, false, 0, err)
		}
		return false
	}
	defer relay.Close()

	err = relay.Publish(ctx, *event)
	elapsed := time.Since(start)

	success := err == nil

	// Track publish result
	if b.resultTracker != nil {
		b.resultTracker.TrackPublishResult(url, success, elapsed, err)
	}

	if success {
		logging.DebugMethod("broadcaster", "publishToRelay", "Published event %s to %s (%.2fms)",
			event.ID, url, elapsed.Seconds()*1000)
	} else {
		logging.DebugMethod("broadcaster", "publishToRelay", "Failed to publish to %s: %v (%.2fms)",
			url, err, elapsed.Seconds()*1000)
	}

	return success
}

// GetStatsName returns the name for this stats provider
func (b *Broadcaster) GetStatsName() string {
	return "broadcaster"
}

// GetStats returns broadcaster-specific statistics in structured format
func (b *Broadcaster) GetStats() interface{} {
	return b.GetBroadcasterStats()
}

// GetBroadcasterStats returns broadcaster-specific statistics in structured format
func (b *Broadcaster) GetBroadcasterStats() BroadcasterStats {
	// Get queue stats
	b.overflowMutex.Lock()
	overflowSize := len(b.overflowQueue)
	b.overflowMutex.Unlock()

	channelSize := len(b.eventQueue)
	totalQueued := atomic.LoadInt64(&b.totalQueued)
	peakSize := atomic.LoadInt64(&b.peakQueueSize)
	saturationCount := atomic.LoadInt64(&b.saturationCount)
	channelUtilization := float64(channelSize) / float64(b.channelCapacity) * 100.0
	isSaturated := overflowSize > 0

	// Get cache stats
	b.cacheMutex.RLock()
	cacheSize := len(b.eventCache)
	b.cacheMutex.RUnlock()
	cacheHits := atomic.LoadInt64(&b.cacheHits)
	cacheMisses := atomic.LoadInt64(&b.cacheMisses)
	totalCacheAccess := cacheHits + cacheMisses
	cacheHitRate := 0.0
	if totalCacheAccess > 0 {
		cacheHitRate = float64(cacheHits) / float64(totalCacheAccess) * 100.0
	}
	cacheUtilization := float64(cacheSize) / float64(b.cacheMaxSize) * 100.0

	return BroadcasterStats{
		MandatoryRelays: len(b.mandatoryRelays),
		Queue: QueueStats{
			WorkerCount:        b.workerCount,
			ChannelSize:        channelSize,
			ChannelCapacity:    b.channelCapacity,
			ChannelUtilization: channelUtilization,
			OverflowSize:       overflowSize,
			TotalQueued:        totalQueued,
			PeakSize:           peakSize,
			SaturationCount:    saturationCount,
			IsSaturated:        isSaturated,
			LastSaturation:     b.lastSaturation.Format(time.RFC3339),
		},
		Cache: CacheStats{
			Size:           cacheSize,
			MaxSize:        b.cacheMaxSize,
			UtilizationPct: cacheUtilization,
			Hits:           cacheHits,
			Misses:         cacheMisses,
			HitRatePct:     cacheHitRate,
		},
	}
}
