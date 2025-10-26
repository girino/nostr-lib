package stats

import (
	"encoding/json"
	"sync"

	jsonlib "github.com/girino/nostr-lib/json"
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

// GetStatsAsOrderedJSON returns stats as an ordered JSON structure
// This preserves the insertion order of fields for consistent output
func (sc *StatsCollector) GetStatsAsOrderedJSON() (*jsonlib.JsonObject, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	obj := jsonlib.NewJsonObject()

	// Add stats in registration order
	for name, provider := range sc.providers {
		stats := provider.GetStats()
		
		// Convert stats to JsonEntity
		entity, err := convertToJsonEntity(stats)
		if err != nil {
			return nil, err
		}
		
		obj.Set(name, entity)
	}

	return obj, nil
}

// convertToJsonEntity converts a Go value to JsonEntity
func convertToJsonEntity(v interface{}) (jsonlib.JsonEntity, error) {
	switch val := v.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64, bool, nil:
		return jsonlib.NewJsonValue(val), nil
	case map[string]interface{}:
		obj := jsonlib.NewJsonObject()
		for k, vi := range val {
			entity, err := convertToJsonEntity(vi)
			if err != nil {
				return nil, err
			}
			obj.Set(k, entity)
		}
		return obj, nil
	case []interface{}:
		list := jsonlib.NewJsonList()
		for _, vi := range val {
			entity, err := convertToJsonEntity(vi)
			if err != nil {
				return nil, err
			}
			list.Append(entity)
		}
		return list, nil
	default:
		// For complex types, marshal to JSON and parse back
		data, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		return jsonlib.Unmarshal(data)
	}
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
