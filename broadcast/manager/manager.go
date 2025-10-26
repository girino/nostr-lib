package manager

import (
	"sort"
	"sync"
	"time"

	"github.com/girino/nostr-lib/json"
	"github.com/girino/nostr-lib/logging"
)

type RelayInfo struct {
	URL                string
	AvgResponseTime    time.Duration
	SuccessRate        float64
	TotalAttempts      int64
	SuccessfulAttempts int64
	LastChecked        time.Time
	IsMandatory        bool
}

type Manager struct {
	relays      map[string]*RelayInfo
	mu          sync.RWMutex
	decay       float64
	topN        int
	initialized bool
}

func NewManager(topN int, decay float64) *Manager {
	logging.Debug("Manager: Initializing manager: topN=%d, decay=%.2f", topN, decay)
	return &Manager{
		relays:      make(map[string]*RelayInfo),
		decay:       decay,
		topN:        topN,
		initialized: false,
	}
}

// AddRelay adds a new relay to the manager
func (m *Manager) AddRelay(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.relays[url]; !exists {
		m.relays[url] = &RelayInfo{
			URL:                url,
			AvgResponseTime:    0,
			SuccessRate:        1.0, // Start optimistic
			TotalAttempts:      0,
			SuccessfulAttempts: 0,
			LastChecked:        time.Now(),
			IsMandatory:        false,
		}
		logging.Debug("Manager: Added new relay: %s (total relays: %d)", url, len(m.relays))
	} else {
		logging.Debug("Manager: Relay already exists: %s", url)
	}
}

// AddMandatoryRelay adds a mandatory relay to the manager
func (m *Manager) AddMandatoryRelay(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if relay, exists := m.relays[url]; exists {
		relay.IsMandatory = true
		logging.Debug("Manager: Marked relay as mandatory: %s", url)
	} else {
		m.relays[url] = &RelayInfo{
			URL:                url,
			AvgResponseTime:    0,
			SuccessRate:        1.0, // Start optimistic
			TotalAttempts:      0,
			SuccessfulAttempts: 0,
			LastChecked:        time.Now(),
			IsMandatory:        true,
		}
		logging.Debug("Manager: Added new mandatory relay: %s (total relays: %d)", url, len(m.relays))
	}
}

// UpdateHealth updates relay health after an initial check
func (m *Manager) UpdateHealth(url string, success bool, responseTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	relay, exists := m.relays[url]
	if !exists {
		logging.Warn("Manager: UpdateHealth called for unknown relay: %s", url)
		return
	}

	oldSuccessRate := relay.SuccessRate
	relay.TotalAttempts++

	if success {
		relay.SuccessfulAttempts++

		// Update average response time using exponential moving average
		if relay.AvgResponseTime == 0 {
			relay.AvgResponseTime = responseTime
		} else {
			relay.AvgResponseTime = time.Duration(
				float64(relay.AvgResponseTime)*0.7 + float64(responseTime)*0.3,
			)
		}

		logging.Debug("Manager: Health update SUCCESS: %s | attempts=%d/%d | responseTime=%.2fms",
			url, relay.SuccessfulAttempts, relay.TotalAttempts, responseTime.Seconds()*1000)
	} else {
		logging.Debug("Manager: Health update FAILED: %s | attempts=%d/%d",
			url, relay.SuccessfulAttempts, relay.TotalAttempts)
	}

	relay.LastChecked = time.Now()

	// After initialization, use exponential decay for success rate
	if m.initialized {
		successValue := 0.0
		if success {
			successValue = 1.0
		}
		relay.SuccessRate = relay.SuccessRate*m.decay + successValue*(1-m.decay)
		logging.Debug("Manager: Success rate updated (exponential decay): %s | %.4f -> %.4f",
			url, oldSuccessRate, relay.SuccessRate)
	} else {
		// During initialization, use simple success rate
		if relay.TotalAttempts > 0 {
			relay.SuccessRate = float64(relay.SuccessfulAttempts) / float64(relay.TotalAttempts)
			logging.Debug("Manager: Success rate updated (simple): %s | %.4f -> %.4f",
				url, oldSuccessRate, relay.SuccessRate)
		}
	}
}

// MarkInitialized marks the manager as initialized
func (m *Manager) MarkInitialized() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initialized = true
	logging.Info("Manager: Initialization complete - switching to exponential decay mode")
	logging.Debug("Manager: Decay factor=%.2f, Total relays=%d", m.decay, len(m.relays))
}

// GetTopRelays returns the top N relays based on composite score
func (m *Manager) GetTopRelays() []*RelayInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	relays := make([]*RelayInfo, 0, len(m.relays))
	untested := 0
	for _, relay := range m.relays {
		// Only include relays that have been tested at least once
		if relay.TotalAttempts > 0 {
			relays = append(relays, relay)
		} else {
			untested++
		}
	}

	logging.Debug("Manager: GetTopRelays - %d tested relays, %d untested", len(relays), untested)

	// Sort by composite score
	sort.Slice(relays, func(i, j int) bool {
		scoreI := m.calculateScore(relays[i])
		scoreJ := m.calculateScore(relays[j])
		return scoreI > scoreJ
	})

	// Log top 5 for visibility (in verbose mode)
	if logging.Verbose {
		logCount := 5
		if len(relays) < logCount {
			logCount = len(relays)
		}
		for i := 0; i < logCount; i++ {
			r := relays[i]
			score := m.calculateScore(r)
			logging.Debug("Manager:   Top #%d: %s | score=%.2f | success=%.2f%% | avg_time=%.2fms | attempts=%d",
				i+1, r.URL, score, r.SuccessRate*100, r.AvgResponseTime.Seconds()*1000, r.TotalAttempts)
		}
	}

	// Return top N
	if len(relays) > m.topN {
		logging.Debug("Manager: Returning top %d out of %d tested relays", m.topN, len(relays))
		return relays[:m.topN]
	}
	logging.Debug("Manager: Returning all %d tested relays (less than topN=%d)", len(relays), m.topN)
	return relays
}

// CalculateScore computes a composite score for ranking
// Higher is better
func (m *Manager) CalculateScore(relay *RelayInfo) float64 {
	// Success rate weight
	successWeight := 100.0

	// Response time penalty (convert to seconds, higher response time = lower score)
	responseTimePenalty := 0.0
	if relay.AvgResponseTime > 0 {
		responseTimePenalty = relay.AvgResponseTime.Seconds() * 10.0
	}

	score := relay.SuccessRate*successWeight - responseTimePenalty

	// Penalize relays with very few attempts during initialization
	if !m.initialized && relay.TotalAttempts < 3 {
		score *= 0.5
	}

	return score
}

// calculateScore is the internal version
func (m *Manager) calculateScore(relay *RelayInfo) float64 {
	return m.CalculateScore(relay)
}

// GetAllRelays returns all relays (for discovery purposes)
func (m *Manager) GetAllRelays() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	urls := make([]string, 0, len(m.relays))
	for url := range m.relays {
		urls = append(urls, url)
	}
	return urls
}

// GetRelayCount returns the number of tracked relays
func (m *Manager) GetRelayCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.relays)
}

// RemoveRelay removes a relay from the manager
func (m *Manager) RemoveRelay(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.relays, url)
	logging.Info("Manager: Removed relay: %s", url)
}

// GetRelayInfo returns info about a specific relay
func (m *Manager) GetRelayInfo(url string) interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if relay, exists := m.relays[url]; exists {
		// Return a copy
		relayCopy := *relay
		return &relayCopy
	}
	return nil
}

// GetMandatoryRelays returns all mandatory relays
func (m *Manager) GetMandatoryRelays() []*RelayInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	relays := make([]*RelayInfo, 0)
	for _, relay := range m.relays {
		if relay.IsMandatory {
			relays = append(relays, relay)
		}
	}
	return relays
}

// GetBroadcastRelays returns the top relays for broadcasting
func (m *Manager) GetBroadcastRelays() []string {
	topRelays := m.GetTopRelays()
	relayURLs := make([]string, len(topRelays))
	for i, relay := range topRelays {
		relayURLs[i] = relay.URL
	}
	return relayURLs
}

// TrackPublishResult tracks the result of a publish operation
func (m *Manager) TrackPublishResult(url string, success bool, responseTime time.Duration, err error) {
	m.UpdateHealth(url, success, responseTime)
}

// CheckBatch performs health checks on multiple relays
func (m *Manager) CheckBatch(urls []string) {
	// This is a placeholder - the actual health checking logic
	// would be implemented by the health package
	logging.Debug("Manager: CheckBatch called with %d relays", len(urls))
}

// CheckInitial performs an initial health check on a relay
func (m *Manager) CheckInitial(url string) bool {
	// This is a placeholder - the actual health checking logic
	// would be implemented by the health package
	logging.Debug("Manager: CheckInitial called for relay %s", url)
	return true
}

// ManagerStats represents manager statistics
type ManagerStats struct {
	MandatoryRelayList []RelayStats `json:"mandatory_relay_list"`
	TopRelays          []RelayStats `json:"top_relays"`
	TotalRelays        int          `json:"total_relays"`
	TopN               int          `json:"top_n"`
	Decay              float64      `json:"decay"`
	Initialized        bool         `json:"initialized"`
}

// RelayStats represents individual relay statistics for JSON output
type RelayStats struct {
	URL           string  `json:"url"`
	Score         float64 `json:"score"`
	SuccessRate   float64 `json:"success_rate"`
	AvgResponseMs int64   `json:"avg_response_ms"`
	TotalAttempts int64   `json:"total_attempts"`
	IsMandatory   bool    `json:"is_mandatory"`
	LastChecked   string  `json:"last_checked"`
}

// GetStatsName returns the name for this stats provider
func (m *Manager) GetStatsName() string {
	return "manager"
}

// GetStats returns manager-specific statistics as a JsonEntity
func (m *Manager) GetStats() json.JsonEntity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	obj := json.NewJsonObject()

	// Add basic stats
	obj.Set("total_relays", json.NewJsonValue(len(m.relays)))
	obj.Set("top_n", json.NewJsonValue(m.topN))
	obj.Set("decay", json.NewJsonValue(m.decay))
	obj.Set("initialized", json.NewJsonValue(m.initialized))

	topRelays := m.GetTopRelays()
	mandatoryRelays := m.GetMandatoryRelays()

	// Convert top relays to JsonList
	topRelayList := json.NewJsonList()
	for _, relay := range topRelays {
		relayObj := json.NewJsonObject()
		score := m.CalculateScore(relay)
		relayObj.Set("url", json.NewJsonValue(relay.URL))
		relayObj.Set("score", json.NewJsonValue(score))
		relayObj.Set("success_rate", json.NewJsonValue(relay.SuccessRate))
		relayObj.Set("avg_response_ms", json.NewJsonValue(relay.AvgResponseTime.Milliseconds()))
		relayObj.Set("total_attempts", json.NewJsonValue(relay.TotalAttempts))
		relayObj.Set("is_mandatory", json.NewJsonValue(relay.IsMandatory))
		relayObj.Set("last_checked", json.NewJsonValue(relay.LastChecked.Format(time.RFC3339)))
		topRelayList.Append(relayObj)
	}

	// Convert mandatory relays to JsonList
	mandatoryRelayList := json.NewJsonList()
	for _, relay := range mandatoryRelays {
		relayObj := json.NewJsonObject()
		score := m.CalculateScore(relay)
		relayObj.Set("url", json.NewJsonValue(relay.URL))
		relayObj.Set("score", json.NewJsonValue(score))
		relayObj.Set("success_rate", json.NewJsonValue(relay.SuccessRate))
		relayObj.Set("avg_response_ms", json.NewJsonValue(relay.AvgResponseTime.Milliseconds()))
		relayObj.Set("total_attempts", json.NewJsonValue(relay.TotalAttempts))
		relayObj.Set("is_mandatory", json.NewJsonValue(relay.IsMandatory))
		relayObj.Set("last_checked", json.NewJsonValue(relay.LastChecked.Format(time.RFC3339)))
		mandatoryRelayList.Append(relayObj)
	}

	obj.Set("top_relays", topRelayList)
	obj.Set("mandatory_relays", mandatoryRelayList)

	return obj
}
