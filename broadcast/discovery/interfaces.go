package discovery

// RelayRegistry manages relay information
type RelayRegistry interface {
	AddRelay(url string)
	GetAllRelays() []string
	GetRelayInfo(url string) interface{} // Returns nil if not found
}

// RelayHealthChecker performs health checks on relays
type RelayHealthChecker interface {
	CheckBatch(urls []string)
	CheckInitial(url string) bool
}
