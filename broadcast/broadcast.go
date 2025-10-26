package broadcast

import (
	"context"
	"time"

	"github.com/girino/nostr-lib/broadcast/broadcaster"
	"github.com/girino/nostr-lib/broadcast/discovery"
	"github.com/girino/nostr-lib/broadcast/health"
	"github.com/girino/nostr-lib/broadcast/manager"
	"github.com/girino/nostr-lib/logging"
	"github.com/girino/nostr-lib/stats"
	"github.com/nbd-wtf/go-nostr"
)

// BroadcastStats represents the complete statistics from the broadcast system
type BroadcastStats struct {
	Broadcaster broadcaster.BroadcasterStats `json:"broadcaster"`
	Manager     manager.ManagerStats         `json:"manager"`
	Timestamp   int64                        `json:"timestamp"`
}

// BroadcastSystem provides a unified interface for relay broadcasting
type BroadcastSystem struct {
	manager        *manager.Manager
	discovery      *discovery.Discovery
	broadcaster    *broadcaster.Broadcaster
	healthChecker  *health.Checker
	statsCollector *stats.StatsCollector
}

// Config holds configuration for the broadcast system
type Config struct {
	TopNRelays       int
	SuccessRateDecay float64
	MandatoryRelays  []string
	WorkerCount      int
	CacheTTL         time.Duration
	InitialTimeout   time.Duration
}

// NewBroadcastSystem creates a new broadcast system with all components
func NewBroadcastSystem(cfg *Config) *BroadcastSystem {
	logging.Debug("BroadcastSystem: Initializing broadcast system")

	// Create manager
	mgr := manager.NewManager(cfg.TopNRelays, cfg.SuccessRateDecay)

	// Create health checker
	healthChecker := health.NewChecker(mgr, cfg.InitialTimeout)

	// Create discovery with manager as registry and health checker
	disc := discovery.NewDiscovery(mgr, healthChecker)

	// Create broadcaster with manager as relay provider and result tracker
	bc := broadcaster.NewBroadcaster(mgr, mgr, cfg.MandatoryRelays, cfg.WorkerCount, cfg.CacheTTL)

	// Create stats collector and register providers
	statsCollector := stats.NewStatsCollector()
	statsCollector.RegisterProvider(mgr)
	statsCollector.RegisterProvider(bc)

	return &BroadcastSystem{
		manager:        mgr,
		discovery:      disc,
		broadcaster:    bc,
		healthChecker:  healthChecker,
		statsCollector: statsCollector,
	}
}

// Start initializes and starts the broadcast system
func (bs *BroadcastSystem) Start() {
	logging.Info("BroadcastSystem: Starting broadcast system")
	bs.broadcaster.Start()
}

// Stop gracefully stops the broadcast system
func (bs *BroadcastSystem) Stop() {
	logging.Info("BroadcastSystem: Stopping broadcast system")
	bs.broadcaster.Stop()
}

// DiscoverFromSeeds performs relay discovery from seed relays
func (bs *BroadcastSystem) DiscoverFromSeeds(ctx context.Context, seedRelays []string) {
	bs.discovery.DiscoverFromSeeds(ctx, seedRelays)
}

// MarkInitialized marks the system as initialized
func (bs *BroadcastSystem) MarkInitialized() {
	bs.manager.MarkInitialized()
}

// BroadcastEvent broadcasts an event to the top relays
func (bs *BroadcastSystem) BroadcastEvent(event *nostr.Event) {
	bs.broadcaster.Broadcast(event)
}

// GetStats returns comprehensive statistics in structured format
func (bs *BroadcastSystem) GetStats() BroadcastStats {
	// Get stats from each provider
	broadcasterStats := bs.broadcaster.GetBroadcasterStats()
	managerStats := bs.manager.GetStats().(manager.ManagerStats)

	return BroadcastStats{
		Broadcaster: broadcasterStats,
		Manager:     managerStats,
		Timestamp:   time.Now().Unix(),
	}
}

// GetStatsCollector returns the stats collector for external use
func (bs *BroadcastSystem) GetStatsCollector() *stats.StatsCollector {
	return bs.statsCollector
}

// GetStatsAsJSON returns all stats as formatted JSON
func (bs *BroadcastSystem) GetStatsAsJSON() ([]byte, error) {
	return bs.statsCollector.GetStatsAsJSON()
}

// AddMandatoryRelays adds mandatory relays to the system
func (bs *BroadcastSystem) AddMandatoryRelays(urls []string) {
	for _, url := range urls {
		bs.manager.AddMandatoryRelay(url)
	}
}

// GetTopRelays returns the top relays
func (bs *BroadcastSystem) GetTopRelays() []*manager.RelayInfo {
	return bs.manager.GetTopRelays()
}

// GetRelayCount returns the number of tracked relays
func (bs *BroadcastSystem) GetRelayCount() int {
	return bs.manager.GetRelayCount()
}

// GetManager returns the underlying manager for external health checking
func (bs *BroadcastSystem) GetManager() *manager.Manager {
	return bs.manager
}

// GetHealthChecker returns the health checker for external use
func (bs *BroadcastSystem) GetHealthChecker() *health.Checker {
	return bs.healthChecker
}

// IsEventCached checks if an event is cached (for duplicate detection)
func (bs *BroadcastSystem) IsEventCached(eventID string) bool {
	return bs.broadcaster.IsEventCached(eventID)
}

// ExtractRelaysFromEvent extracts relay URLs from an event
func (bs *BroadcastSystem) ExtractRelaysFromEvent(event *nostr.Event) []string {
	return bs.discovery.ExtractRelaysFromEvent(event)
}

// AddRelayIfNew adds a relay if it's not already known
func (bs *BroadcastSystem) AddRelayIfNew(url string) {
	bs.discovery.AddRelayIfNew(url)
}
