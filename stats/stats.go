package stats

import (
	"encoding/json"
	"sync"
)

// StatsProvider defines the interface for modules that provide statistics
type StatsProvider interface {
	// GetStatsName returns a unique name for this stats provider
	GetStatsName() string

	// GetStats returns the statistics data for this provider
	GetStats() interface{}
}

// StatsCollector manages multiple stats providers and aggregates their data
type StatsCollector struct {
	providers map[string]StatsProvider
	mu        sync.RWMutex
}

// NewStatsCollector creates a new stats collector
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		providers: make(map[string]StatsProvider),
	}
}

// RegisterProvider registers a stats provider with the collector
func (sc *StatsCollector) RegisterProvider(provider StatsProvider) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	name := provider.GetStatsName()
	sc.providers[name] = provider
}

// UnregisterProvider removes a stats provider from the collector
func (sc *StatsCollector) UnregisterProvider(name string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	delete(sc.providers, name)
}

// GetAllStats collects statistics from all registered providers
func (sc *StatsCollector) GetAllStats() map[string]interface{} {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	allStats := make(map[string]interface{})

	for name, provider := range sc.providers {
		allStats[name] = provider.GetStats()
	}

	return allStats
}

// GetStatsAsJSON returns all stats as formatted JSON
func (sc *StatsCollector) GetStatsAsJSON() ([]byte, error) {
	stats := sc.GetAllStats()
	return json.MarshalIndent(stats, "", "  ")
}

// GetProviderNames returns a list of all registered provider names
func (sc *StatsCollector) GetProviderNames() []string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	names := make([]string, 0, len(sc.providers))
	for name := range sc.providers {
		names = append(names, name)
	}

	return names
}

// GetProviderCount returns the number of registered providers
func (sc *StatsCollector) GetProviderCount() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return len(sc.providers)
}
