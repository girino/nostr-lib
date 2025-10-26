package stats

import (
	"sync"

	"github.com/girino/nostr-lib/json"
)

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

	// GetStatsAsJSONString returns all stats as a formatted JSON string
	GetStatsAsJSONString() (string, error)

	// GetProviderNames returns a list of all registered provider names
	GetProviderNames() []string

	// GetProviderCount returns the number of registered providers
	GetProviderCount() int
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

// GetStatsAsJSONString returns all stats as a formatted JSON string
func (sc *defaultStatsCollector) GetStatsAsJSONString() (string, error) {
	data, err := sc.GetStatsAsJSON()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetProviderNames returns a list of all registered provider names
func (sc *defaultStatsCollector) GetProviderNames() []string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	names := make([]string, 0, len(sc.providers))
	for name := range sc.providers {
		names = append(names, name)
	}

	return names
}

// GetProviderCount returns the number of registered providers
func (sc *defaultStatsCollector) GetProviderCount() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return len(sc.providers)
}
