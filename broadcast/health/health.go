package health

import (
	"context"
	"sync"
	"time"

	"github.com/girino/nostr-lib/broadcast/manager"
	"github.com/girino/nostr-lib/logging"
	"github.com/nbd-wtf/go-nostr"
)

type Checker struct {
	manager        *manager.Manager
	initialTimeout time.Duration
}

func NewChecker(mgr *manager.Manager, initialTimeout time.Duration) *Checker {
	logging.DebugMethod("health", "NewChecker", "Initializing health checker with timeout=%v", initialTimeout)
	return &Checker{
		manager:        mgr,
		initialTimeout: initialTimeout,
	}
}

// CheckInitial performs initial timeout-based health check on a relay
func (c *Checker) CheckInitial(url string) bool {
	logging.DebugMethod("health", "CheckInitial", "Testing relay: %s", url)

	ctx, cancel := context.WithTimeout(context.Background(), c.initialTimeout)
	defer cancel()

	start := time.Now()
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		elapsed := time.Since(start)
		logging.DebugMethod("health", "CheckInitial", "Failed to connect to %s | error=%v | time=%.2fms", url, err, elapsed.Seconds()*1000)
		c.manager.UpdateHealth(url, false, 0)
		return false
	}
	defer relay.Close()

	elapsed := time.Since(start)

	// Consider it successful if we connected
	c.manager.UpdateHealth(url, true, elapsed)
	logging.DebugMethod("health", "CheckInitial", "Connected successfully to %s | time=%.2fms", url, elapsed.Seconds()*1000)
	return true
}

// CheckBatch performs initial checks on multiple relays concurrently
func (c *Checker) CheckBatch(urls []string) {
	logging.DebugMethod("health", "CheckBatch", "Starting batch health check of %d relays (max 20 concurrent)", len(urls))

	sem := make(chan struct{}, 20) // Limit concurrent checks
	var wg sync.WaitGroup
	successCount := 0
	failCount := 0
	var mu sync.Mutex

	start := time.Now()

	for _, url := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			success := c.CheckInitial(u)
			mu.Lock()
			if success {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()
		}(url)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Always log the summary
	logging.Info("Health: Batch check complete - %d success, %d failed out of %d total (%.2fs)",
		successCount, failCount, len(urls), elapsed.Seconds())
}

// PublishResult tracks the result of a publish attempt
type PublishResult struct {
	URL          string
	Success      bool
	ResponseTime time.Duration
	Error        error
}

// TrackPublishResult updates relay health based on publish results
func (c *Checker) TrackPublishResult(result PublishResult) {
	c.manager.UpdateHealth(result.URL, result.Success, result.ResponseTime)

	if !result.Success && result.Error != nil {
		logging.DebugMethod("health", "TrackPublishResult", "Publish to %s failed: %v", result.URL, result.Error)
	}
}
