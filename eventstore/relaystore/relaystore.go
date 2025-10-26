// Copyright (c) 2025 Girino Vey.
//
// This software is licensed under Girino's Anarchist License (GAL).
// See LICENSE file for full license text.
// License available at: https://license.girino.org/
//
// RelayStore - Nostr relay aggregation and forwarding functionality.
package relaystore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fiatjaf/eventstore"
	"github.com/fiatjaf/khatru"
	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
)

// Health state constants
const (
	HealthGreen  = "GREEN"
	HealthYellow = "YELLOW"
	HealthRed    = "RED"
)

// Query timeout duration for both QueryEvents and CountEvents
const QueryTimeoutDuration = 5 * time.Second

// PrefixedError represents an error with a machine-readable prefix from NIP-01
type PrefixedError struct {
	Prefix   string
	Message  string
	RelayURL string
}

func (e PrefixedError) Error() string {
	if e.Prefix != "" {
		if e.RelayURL != "" {
			return e.Prefix + ": " + e.Message + " (" + e.RelayURL + ")"
		}
		return e.Prefix + ": " + e.Message
	}
	return e.Message
}

// parseErrorPrefix extracts the machine-readable prefix from a relay error message
// NIP-01 standardized prefixes: duplicate, pow, blocked, rate-limited, invalid, restricted, mute, error, auth-required
func parseErrorPrefix(err error) (prefix, message string) {
	if err == nil {
		return "", ""
	}

	errStr := err.Error()

	// Remove "msg: " prefix that Publish() always adds
	errStr = strings.TrimPrefix(errStr, "msg: ")

	// Look for pattern "prefix: message" where prefix is before the first colon
	if colonIdx := strings.Index(errStr, ": "); colonIdx > 0 {
		prefix = strings.TrimSpace(errStr[:colonIdx])
		message = strings.TrimSpace(errStr[colonIdx+2:])

		// Validate that the prefix is one of the standardized NIP-01 prefixes
		validPrefixes := []string{"duplicate", "pow", "blocked", "rate-limited", "invalid", "restricted", "mute", "error", "auth-required"}
		for _, validPrefix := range validPrefixes {
			if prefix == validPrefix {
				return prefix, message
			}
		}
	}

	// If no valid prefix found, return the whole error as message
	return "", errStr
}

// handleError handles error collection, logging, and counting
func (r *RelayStore) handleError(errsMu *sync.Mutex, errs *[]error, prefixedErrs *[]PrefixedError, url string, err error, context string) {
	errsMu.Lock()
	*errs = append(*errs, fmt.Errorf("%s: %w", url, err))
	// parse error prefix for structured error handling
	if prefix, msg := parseErrorPrefix(err); prefix != "" {
		*prefixedErrs = append(*prefixedErrs, PrefixedError{Prefix: prefix, Message: msg, RelayURL: url})
	}
	errsMu.Unlock()
	// count failure
	atomic.AddInt64(&r.publishFailures, 1)
	logging.DebugMethod("relaystore", "logError", "%s to %s failed: %v", context, url, err)
}

type RelayStore struct {
	urls   []string
	relays map[string]*nostr.Relay
	// queryUrls are the remotes used for answering queries/subscriptions
	queryUrls []string
	// pool manages connections for query remotes
	pool *nostr.SimplePool
	mu   sync.RWMutex
	// publish timeout per remote
	publishTimeout time.Duration
	// relaySecKey is the private key used for authenticating to upstream relays
	relaySecKey string
	// stats
	publishAttempts     int64
	publishSuccesses    int64
	publishFailures     int64
	queryRequests       int64
	queryInternal       int64
	queryExternal       int64
	queryEventsReturned int64
	queryFailures       int64
	// separate counters for CountEvents
	countRequests       int64
	countInternal       int64
	countExternal       int64
	countEventsReturned int64
	countFailures       int64
	// subset of queryUrls that advertise NIP-45 in their NIP-11
	countableQueryUrls []string
	// health check tracking
	consecutivePublishFailures int64
	consecutiveQueryFailures   int64
	maxConsecutiveFailures     int64
	// timing statistics
	totalPublishDurationNs int64
	totalQueryDurationNs   int64
	totalCountDurationNs   int64
	publishCount           int64
	queryCount             int64
	countCount             int64
}

// Stats holds runtime counters exported by RelayStore
type Stats struct {
	PublishAttempts     int64 `json:"publish_attempts"`
	PublishSuccesses    int64 `json:"publish_successes"`
	PublishFailures     int64 `json:"publish_failures"`
	QueryRequests       int64 `json:"query_requests"`
	QueryInternal       int64 `json:"query_internal_requests"`
	QueryExternal       int64 `json:"query_external_requests"`
	QueryEventsReturned int64 `json:"query_events_returned"`
	QueryFailures       int64 `json:"query_failures"`
	// CountEvents-specific counters
	CountRequests       int64 `json:"count_requests"`
	CountInternal       int64 `json:"count_internal_requests"`
	CountExternal       int64 `json:"count_external_requests"`
	CountEventsReturned int64 `json:"count_events_returned"`
	CountFailures       int64 `json:"count_failures"`
	// Health check fields
	ConsecutivePublishFailures int64  `json:"consecutive_publish_failures"`
	ConsecutiveQueryFailures   int64  `json:"consecutive_query_failures"`
	IsHealthy                  bool   `json:"is_healthy"`
	HealthStatus               string `json:"health_status"`
	// Detailed health indicators
	PublishHealthState string `json:"publish_health_state"`
	QueryHealthState   string `json:"query_health_state"`
	MainHealthState    string `json:"main_health_state"`
	// Timing statistics
	AveragePublishDurationMs float64 `json:"average_publish_duration_ms"`
	AverageQueryDurationMs   float64 `json:"average_query_duration_ms"`
	AverageCountDurationMs   float64 `json:"average_count_duration_ms"`
	TotalPublishDurationMs   int64   `json:"total_publish_duration_ms"`
	TotalQueryDurationMs     int64   `json:"total_query_duration_ms"`
	TotalCountDurationMs     int64   `json:"total_count_duration_ms"`
}

// getHealthState determines the health state based on consecutive failures
func getHealthState(consecutiveFailures int64) string {
	if consecutiveFailures <= 2 {
		return HealthGreen
	} else if consecutiveFailures < 10 {
		return HealthYellow
	}
	return HealthRed
}

// getWorstHealthState returns the worst health state between three states
func getWorstHealthState(state1, state2, state3 string) string {
	if state1 == HealthRed || state2 == HealthRed || state3 == HealthRed {
		return HealthRed
	}
	if state1 == HealthYellow || state2 == HealthYellow || state3 == HealthYellow {
		return HealthYellow
	}
	return HealthGreen
}

// Stats returns a snapshot of the RelayStore counters
func (r *RelayStore) Stats() Stats {
	consecutivePublishFailures := atomic.LoadInt64(&r.consecutivePublishFailures)
	consecutiveQueryFailures := atomic.LoadInt64(&r.consecutiveQueryFailures)
	maxFailures := atomic.LoadInt64(&r.maxConsecutiveFailures)

	isHealthy := consecutivePublishFailures < maxFailures && consecutiveQueryFailures < maxFailures
	healthStatus := "healthy"
	if !isHealthy {
		healthStatus = "unhealthy"
	}

	// Determine individual health states
	publishHealthState := getHealthState(consecutivePublishFailures)
	queryHealthState := getHealthState(consecutiveQueryFailures)
	mainHealthState := getWorstHealthState(publishHealthState, queryHealthState, HealthGreen)

	// Calculate timing statistics
	totalPublishDurationNs := atomic.LoadInt64(&r.totalPublishDurationNs)
	totalQueryDurationNs := atomic.LoadInt64(&r.totalQueryDurationNs)
	totalCountDurationNs := atomic.LoadInt64(&r.totalCountDurationNs)
	publishCount := atomic.LoadInt64(&r.publishCount)
	queryCount := atomic.LoadInt64(&r.queryCount)
	countCount := atomic.LoadInt64(&r.countCount)

	var averagePublishDurationMs float64
	var averageQueryDurationMs float64
	var averageCountDurationMs float64

	if publishCount > 0 {
		averagePublishDurationMs = float64(totalPublishDurationNs) / float64(publishCount) / 1e6 // Convert ns to ms
	}
	if queryCount > 0 {
		averageQueryDurationMs = float64(totalQueryDurationNs) / float64(queryCount) / 1e6 // Convert ns to ms
	}
	if countCount > 0 {
		averageCountDurationMs = float64(totalCountDurationNs) / float64(countCount) / 1e6 // Convert ns to ms
	}

	return Stats{
		PublishAttempts:            atomic.LoadInt64(&r.publishAttempts),
		PublishSuccesses:           atomic.LoadInt64(&r.publishSuccesses),
		PublishFailures:            atomic.LoadInt64(&r.publishFailures),
		QueryRequests:              atomic.LoadInt64(&r.queryRequests),
		QueryInternal:              atomic.LoadInt64(&r.queryInternal),
		QueryExternal:              atomic.LoadInt64(&r.queryExternal),
		QueryEventsReturned:        atomic.LoadInt64(&r.queryEventsReturned),
		QueryFailures:              atomic.LoadInt64(&r.queryFailures),
		CountRequests:              atomic.LoadInt64(&r.countRequests),
		CountInternal:              atomic.LoadInt64(&r.countInternal),
		CountExternal:              atomic.LoadInt64(&r.countExternal),
		CountEventsReturned:        atomic.LoadInt64(&r.countEventsReturned),
		CountFailures:              atomic.LoadInt64(&r.countFailures),
		ConsecutivePublishFailures: consecutivePublishFailures,
		ConsecutiveQueryFailures:   consecutiveQueryFailures,
		IsHealthy:                  isHealthy,
		HealthStatus:               healthStatus,
		PublishHealthState:         publishHealthState,
		QueryHealthState:           queryHealthState,
		MainHealthState:            mainHealthState,
		// Timing statistics
		AveragePublishDurationMs: averagePublishDurationMs,
		AverageQueryDurationMs:   averageQueryDurationMs,
		AverageCountDurationMs:   averageCountDurationMs,
		TotalPublishDurationMs:   totalPublishDurationNs / 1e6, // Convert ns to ms
		TotalQueryDurationMs:     totalQueryDurationNs / 1e6,   // Convert ns to ms
		TotalCountDurationMs:     totalCountDurationNs / 1e6,   // Convert ns to ms
	}
}

// New creates a RelayStore with mandatory query relays, optional publish relays, and optional relay authentication key.
func New(queryUrls []string, publishUrls []string, relaySecKey string) *RelayStore {
	if len(queryUrls) == 0 {
		panic("query relays are mandatory - at least one query relay must be provided")
	}

	rs := &RelayStore{
		urls:                   publishUrls,
		queryUrls:              queryUrls,
		relays:                 make(map[string]*nostr.Relay),
		publishTimeout:         7 * time.Second,
		maxConsecutiveFailures: 10, // Default threshold: 10 consecutive failures
		relaySecKey:            relaySecKey,
	}
	return rs
}

func (r *RelayStore) Init() error {
	// setup query pool: create pool even if no queryUrls provided
	// create a SimplePool for queries
	r.pool = nostr.NewSimplePool(context.Background(), nostr.WithPenaltyBox())

	// build countableQueryUrls by probing each query relay's NIP-11 to see if
	// it advertises support for NIP-45. We do a best-effort HTTP(S) GET to the
	// relay's /.well-known/nostr.json or the host root as per NIP-11. If the
	// probe fails, we skip the relay for counting but keep it as a query
	// remote for FetchMany.
	r.countableQueryUrls = []string{}
	for _, q := range r.queryUrls {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		// derive a well-formed URL to probe NIP-11 via Accept header: GET / with
		// Accept: application/nostr+json. Convert ws(s):// to http(s):// as
		// needed and probe the root path.
		u := q
		if strings.HasPrefix(u, "ws://") {
			u = "http://" + strings.TrimPrefix(u, "ws://")
		} else if strings.HasPrefix(u, "wss://") {
			u = "https://" + strings.TrimPrefix(u, "wss://")
		}
		parsed, err := neturl.Parse(u)
		if err != nil {
			logging.DebugMethod("relaystore", "Init", "cannot parse query url %s: %v", q, err)
			continue
		}
		// ensure root path
		parsed.Path = "/"
		probeURL := parsed.String()

		logging.DebugMethod("relaystore", "Init", "probing NIP-11 for %s -> %s", q, probeURL)
		client := &http.Client{Timeout: 4 * time.Second}
		req, err := http.NewRequest("GET", probeURL, nil)
		if err != nil {
			logging.DebugMethod("relaystore", "Init", "failed to build NIP-11 probe request for %s: %v", q, err)
			continue
		}
		// NIP-01 requires Accept: application/nostr+json
		req.Header.Set("Accept", "application/nostr+json")
		resp, err := client.Do(req)
		if err != nil {
			logging.DebugMethod("relaystore", "Init", "failed probing NIP-11 for %s: %v", q, err)
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				logging.DebugMethod("relaystore", "Init", "non-200 NIP-11 response from %s: %d", q, resp.StatusCode)
				return
			}
			var doc map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
				logging.DebugMethod("relaystore", "Init", "failed to decode NIP-11 from %s: %v", q, err)
				return
			}
			// check supported_nips (NIP-11) for 45
			if s, ok := doc["supported_nips"]; ok {
				switch arr := s.(type) {
				case []interface{}:
					for _, v := range arr {
						// JSON numbers decode to float64
						if num, ok := v.(float64); ok {
							if int(num) == 45 {
								r.countableQueryUrls = append(r.countableQueryUrls, q)
								logging.DebugMethod("relaystore", "Init", "relay %s advertises NIP-45; added to countable list", q)
								return
							}
						}
					}
				case []int:
					for _, nip := range arr {
						if nip == 45 {
							r.countableQueryUrls = append(r.countableQueryUrls, q)
							logging.DebugMethod("relaystore", "Init", "relay %s advertises NIP-45; added to countable list", q)
							return
						}
					}
				}
			}
			logging.DebugMethod("relaystore", "Init", "relay %s does not advertise NIP-45", q)
		}()
	}

	logging.DebugMethod("relaystore", "Init", "query remotes: %v", r.queryUrls)
	logging.DebugMethod("relaystore", "Init", "countable query remotes (NIP-45): %v", r.countableQueryUrls)
	return nil
}

func (r *RelayStore) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rl := range r.relays {
		_ = rl.Close()
	}
	r.relays = map[string]*nostr.Relay{}
}

// helper to ensure a relay connection exists; best-effort.
func (r *RelayStore) ensureRelay(ctx context.Context, url string) (*nostr.Relay, error) {
	r.mu.RLock()
	rl, ok := r.relays[url]
	r.mu.RUnlock()
	if ok && rl.IsConnected() {
		return rl, nil
	}
	// try to connect synchronously
	logging.DebugMethod("relaystore", "ensureRelay", "connecting to %s", url)
	newrl, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		logging.DebugMethod("relaystore", "ensureRelay", "failed to connect to %s: %v", url, err)
		return nil, err
	}

	// attempt authentication if we have a relay secret key
	if r.relaySecKey != "" {
		logging.DebugMethod("relaystore", "ensureRelay", "attempting authentication to %s with key length: %d", url, len(r.relaySecKey))
		if len(r.relaySecKey) > 0 {
			prefixLen := 8
			if len(r.relaySecKey) < prefixLen {
				prefixLen = len(r.relaySecKey)
			}
			logging.DebugMethod("relaystore", "ensureRelay", "relaySecKey starts with: %s", r.relaySecKey[:prefixLen])
		}
		err = newrl.Auth(ctx, func(event *nostr.Event) error {
			// sign the AUTH event with our relay secret key
			return event.Sign(r.relaySecKey)
		})
		if err != nil {
			logging.DebugMethod("relaystore", "ensureRelay", "authentication to %s failed: %v", url, err)
			// continue without authentication - some relays don't require it
		} else {
			logging.DebugMethod("relaystore", "ensureRelay", "authenticated to %s", url)
		}
	}

	r.mu.Lock()
	r.relays[url] = newrl
	r.mu.Unlock()
	logging.DebugMethod("relaystore", "ensureRelay", "connected to %s", url)
	return newrl, nil
}

// QueryEvents returns an empty, closed channel because this store does not persist events.
func (r *RelayStore) QueryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
	// count total requests
	atomic.AddInt64(&r.queryRequests, 1)

	// If khatru explicitly marked this as an internal call, short-circuit.
	if khatru.IsInternalCall(ctx) || ctx.Value(1) == nil {
		atomic.AddInt64(&r.queryInternal, 1)
		logging.DebugMethod("relaystore", "QueryEvents", "internal query short-circuited (khatru internal call) filter=%+v", filter)
		ch := make(chan *nostr.Event)
		close(ch)
		return ch, nil
	}

	atomic.AddInt64(&r.queryExternal, 1)

	// if no pool available, return closed channel
	if r.pool == nil {
		logging.DebugMethod("relaystore", "QueryEvents", "QueryEvents called but no pool initialized (khatru_internal_call=%v) filter=%+v", khatru.IsInternalCall(ctx), filter)
		ch := make(chan *nostr.Event)
		close(ch)
		return ch, nil
	}

	// use FetchMany which ends when all relays return EOSE
	logging.DebugMethod("relaystore", "QueryEvents", "QueryEvents called (khatru_internal_call=%v) filter=%+v", khatru.IsInternalCall(ctx), filter)

	// before subscribing, try ensuring relays to detect quick failures and count them
	querySuccesses := 0
	for _, q := range r.queryUrls {
		if q == "" {
			continue
		}
		if _, err := r.pool.EnsureRelay(q); err != nil {
			// count query relay failure
			atomic.AddInt64(&r.queryFailures, 1)
			logging.DebugMethod("relaystore", "QueryEvents", "failed to ensure query relay %s: %v", q, err)
		} else {
			querySuccesses++
		}
	}

	// Track consecutive query failures for health checking
	// Require at least 1/4 of relays to be online (rounded up)
	totalRelays := len(r.queryUrls)
	threshold := (totalRelays + 3) / 4 // 1/4 rounded up

	if querySuccesses >= threshold {
		// Success: reset consecutive failure counter
		atomic.StoreInt64(&r.consecutiveQueryFailures, 0)
	} else {
		// Failure: increment consecutive failure counter
		atomic.AddInt64(&r.consecutiveQueryFailures, 1)
	}

	// Start timing measurement for the complete query operation
	startTime := time.Now()

	// QueryTimeoutDuration or cancel - timeout starts AFTER semaphore acquisition
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	evch := r.pool.FetchMany(timeoutCtx, r.queryUrls, filter)
	out := make(chan *nostr.Event)

	go func() {
		// Complete timing measurement for the complete query operation
		defer timeoutCancel()
		defer func() {
			duration := time.Since(startTime)
			atomic.AddInt64(&r.totalQueryDurationNs, duration.Nanoseconds())
			atomic.AddInt64(&r.queryCount, 1)
		}()
		defer close(out)

		maxEvents := 100
		if filter.Limit > 0 {
			maxEvents = int(filter.Limit)
		}
		numEvents := 0
		for {
			select {
			case <-timeoutCtx.Done():
				logging.Warn("query timed out after %v", QueryTimeoutDuration)
				return
			case ie, ok := <-evch:
				if !ok {
					logging.DebugMethod("relaystore", "QueryEvents", "query channel closed")
					return
				}
				atomic.AddInt64(&r.queryEventsReturned, 1)
				select {
				case out <- ie.Event:
					numEvents++ // Event sent successfully
					if numEvents >= maxEvents {
						logging.DebugMethod("relaystore", "QueryEvents", "query reached max events limit of %d", maxEvents)
						return
					}
				case <-timeoutCtx.Done():
					logging.Warn("query timed out after %v", QueryTimeoutDuration)
					return
				}
			}
		}
	}()

	return out, nil
}

// DeleteEvent is a no-op for relay forwarding store.
func (r *RelayStore) DeleteEvent(ctx context.Context, evt *nostr.Event) error {
	return nil
}

// SaveEvent forwards the event to all configured remotes. It returns nil if at least one remote accepted the event.
func (r *RelayStore) SaveEvent(ctx context.Context, evt *nostr.Event) error {
	// Start timing measurement
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		atomic.AddInt64(&r.totalPublishDurationNs, duration.Nanoseconds())
		atomic.AddInt64(&r.publishCount, 1)
	}()

	// publish to all remotes concurrently and collect errors
	var wg sync.WaitGroup
	errsMu := sync.Mutex{}
	var errs []error
	var prefixedErrs []PrefixedError

	// if no remotes configured, simply return nil (nothing to do)
	if len(r.urls) == 0 {
		logging.DebugMethod("relaystore", "SaveEvent", "no remotes configured, not forwarding event %s", evt.ID)
		return nil
	}

	for _, url := range r.urls {
		url := strings.TrimSpace(url)
		if url == "" {
			continue
		}
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			// create a child context with timeout for each publish
			cctx, cancel := context.WithTimeout(ctx, r.publishTimeout)
			defer cancel()

			logging.DebugMethod("relaystore", "SaveEvent", "publishing event %s to %s", evt.ID, u)

			// count attempt
			atomic.AddInt64(&r.publishAttempts, 1)

			rl, err := r.ensureRelay(cctx, u)
			if err != nil {
				errsMu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", u, err))
				errsMu.Unlock()
				logging.DebugMethod("relaystore", "SaveEvent", "publish to %s failed to get relay: %v", u, err)
				return
			}

			if err := rl.Publish(cctx, *evt); err != nil {
				// Check if this is an auth-required error and we have a relay key
				if prefix, _ := parseErrorPrefix(err); prefix == "auth-required" && r.relaySecKey != "" {
					logging.DebugMethod("relaystore", "SaveEvent", "auth-required from %s, attempting relay authentication", u)

					// Try to authenticate with the upstream relay
					// Derive our relay's public key for logging
					relayPubKey, _ := nostr.GetPublicKey(r.relaySecKey)
					logging.DebugMethod("relaystore", "SaveEvent", "authenticating with upstream relay using pubkey: %s", relayPubKey)
					authErr := rl.Auth(cctx, func(event *nostr.Event) error {
						return event.Sign(r.relaySecKey)
					})

					if authErr != nil {
						logging.DebugMethod("relaystore", "SaveEvent", "authentication to %s failed: %v", u, authErr)
						// Continue with normal error handling
					} else {
						logging.DebugMethod("relaystore", "SaveEvent", "authenticated to %s, retrying publish", u)

						// Retry the publish after authentication
						if retryErr := rl.Publish(cctx, *evt); retryErr != nil {
							r.handleError(&errsMu, &errs, &prefixedErrs, u, retryErr, "retry publish")
							return
						}

						// Success after authentication
						atomic.AddInt64(&r.publishSuccesses, 1)
						logging.DebugMethod("relaystore", "SaveEvent", "publish to %s succeeded after authentication for event %s", u, evt.ID)
						return
					}
				}

				r.handleError(&errsMu, &errs, &prefixedErrs, u, err, "publish")
				return
			}
			// count success
			atomic.AddInt64(&r.publishSuccesses, 1)
			logging.DebugMethod("relaystore", "SaveEvent", "publish to %s succeeded for event %s", u, evt.ID)
		}(url)
	}

	wg.Wait()

	// Track consecutive failures for health checking
	if len(errs) == 0 {
		// Success: reset consecutive failure counter
		atomic.StoreInt64(&r.consecutivePublishFailures, 0)
		return nil
	}

	// Failure: increment consecutive failure counter
	atomic.AddInt64(&r.consecutivePublishFailures, 1)

	// if all remotes failed, return the first prefixed error if available, otherwise aggregated error
	if len(prefixedErrs) > 0 {
		return prefixedErrs[0]
	}

	// if no prefixed errors, return aggregated error
	return errors.New(strings.Join(func() []string {
		ss := make([]string, len(errs))
		for i, e := range errs {
			ss[i] = e.Error()
		}
		return ss
	}(), "; "))
}

// ReplaceEvent just forwards the event (best-effort), similar to SaveEvent.
func (r *RelayStore) ReplaceEvent(ctx context.Context, evt *nostr.Event) error {
	return r.SaveEvent(ctx, evt)
}

// CountEvents forwards the filter to query remotes and returns the total number
// of matching events observed. It follows the same short-circuit rules as
// QueryEvents: internal khatru calls and the exact adding.go kind=5/#e
// short-circuit (when ctx.Value(1) == nil) are not forwarded.
func (r *RelayStore) CountEvents(ctx context.Context, filter nostr.Filter) (int64, error) {
	// Start timing measurement
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		atomic.AddInt64(&r.totalCountDurationNs, duration.Nanoseconds())
		atomic.AddInt64(&r.countCount, 1)
	}()

	// count total requests
	atomic.AddInt64(&r.countRequests, 1)

	// short-circuit khatru internal calls
	if khatru.IsInternalCall(ctx) {
		atomic.AddInt64(&r.countInternal, 1)
		logging.DebugMethod("relaystore", "CountEvents", "internal count short-circuited (khatru internal call) filter=%+v", filter)
		return 0, nil
	}

	atomic.AddInt64(&r.countExternal, 1)

	if r.pool == nil {
		logging.DebugMethod("relaystore", "CountEvents", "CountEvents called but no pool initialized (khatru_internal_call=%v) filter=%+v", khatru.IsInternalCall(ctx), filter)
		return 0, nil
	}

	logging.DebugMethod("relaystore", "CountEvents", "CountEvents called (khatru_internal_call=%v) filter=%+v", khatru.IsInternalCall(ctx), filter)

	// ensure relays and count failures (only for countable query remotes)
	if len(r.countableQueryUrls) == 0 {
		logging.DebugMethod("relaystore", "CountEvents", "no NIP-45-capable query remotes available; returning 0")
		return 0, nil
	}

	// before counting, try ensuring relays to detect quick failures and count them
	countSuccesses := 0
	for _, q := range r.countableQueryUrls {
		if q == "" {
			continue
		}
		if _, err := r.pool.EnsureRelay(q); err != nil {
			// count query relay failure
			atomic.AddInt64(&r.countFailures, 1)
			logging.DebugMethod("relaystore", "CountEvents", "failed to ensure query relay %s: %v", q, err)
		} else {
			countSuccesses++
		}
	}

	// Track consecutive count failures for health checking
	// Require at least 1/4 of relays to be online (rounded up)
	totalRelays := len(r.countableQueryUrls)
	threshold := (totalRelays + 3) / 4 // 1/4 rounded up

	if countSuccesses >= threshold {
		// Success: reset consecutive failure counter
		atomic.StoreInt64(&r.consecutiveQueryFailures, 0)
	} else {
		// Failure: increment consecutive failure counter
		atomic.AddInt64(&r.consecutiveQueryFailures, 1)
	}

	// use CountMany which aggregates counts across relays (NIP-45 HyperLogLog)
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer timeoutCancel()
	cnt := r.pool.CountMany(timeoutCtx, r.countableQueryUrls, filter, nil)
	if cnt > 0 {
		atomic.AddInt64(&r.countEventsReturned, int64(cnt))
	}
	return int64(cnt), nil
}

// Ensure RelayStore implements eventstore.Store and eventstore.Counter
var _ eventstore.Store = (*RelayStore)(nil)
var _ eventstore.Counter = (*RelayStore)(nil)
