package broadcaster

import "time"

// RelayProvider provides relay URLs for broadcasting
type RelayProvider interface {
	GetBroadcastRelays() []string
}

// PublishResultTracker tracks results of publish operations
type PublishResultTracker interface {
	TrackPublishResult(url string, success bool, responseTime time.Duration, err error)
}
