package stats

import (
	"sync"

	"github.com/girino/nostr-lib/json"
)

var (
	globalCollector StatsCollector
	collectorOnce   sync.Once
)

// GetCollector returns the global stats collector singleton
func GetCollector() StatsCollector {
	collectorOnce.Do(func() {
		globalCollector = NewStatsCollector()
	})
	return globalCollector
}

// StatsProvider defines the interface for modules that provide statistics
type StatsProvider interface {
	// GetStatsName returns a unique name for this stats provider
	GetStatsName() string

	// GetStats returns the statistics data as a JsonEntity
	GetStats() json.JsonEntity
}

// StatsCollector defines the interface for collecting statistics from providers
type StatsCollector interface {
	// RegisterProvider registers a stats provider with the collector
	RegisterProvider(provider StatsProvider)

	// UnregisterProvider removes a stats provider from the collector
	UnregisterProvider(name string)

	// GetAllStats collects statistics from all registered providers
	// Returns a JsonObject with ordered keys
	GetAllStats() *json.JsonObject

	// GetStatsAsJSON returns all stats as formatted JSON
	GetStatsAsJSON() ([]byte, error)
}

// defaultStatsCollector is the default implementation of StatsCollector
type defaultStatsCollector struct {
	providers map[string]StatsProvider
	mu        sync.RWMutex
}

// NewStatsCollector creates a new stats collector
func NewStatsCollector() StatsCollector {
	return &defaultStatsCollector{
		providers: make(map[string]StatsProvider),
	}
}

// RegisterProvider registers a stats provider with the collector
func (sc *defaultStatsCollector) RegisterProvider(provider StatsProvider) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	name := provider.GetStatsName()
	sc.providers[name] = provider
}

// UnregisterProvider removes a stats provider from the collector
func (sc *defaultStatsCollector) UnregisterProvider(name string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	delete(sc.providers, name)
}

// GetAllStats collects statistics from all registered providers
// Returns a JsonObject with ordered keys
func (sc *defaultStatsCollector) GetAllStats() *json.JsonObject {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	obj := json.NewJsonObject()

	// Add stats in registration order
	for name, provider := range sc.providers {
		entity := provider.GetStats()
		obj.Set(name, entity)
	}

	return obj
}

// GetStatsAsJSON returns all stats as formatted JSON
func (sc *defaultStatsCollector) GetStatsAsJSON() ([]byte, error) {
	stats := sc.GetAllStats()
	return json.MarshalIndent(stats, "", "  ")
}
