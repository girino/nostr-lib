// Copyright (c) 2026 Girino Vey.
package fanout

import "time"

const (
	defaultWriteQueue     = 64
	defaultMaxConnections = 256
	defaultSlowWrite      = 200 * time.Millisecond
	defaultStuckWrite     = 3 * time.Second
)

// Option configures a Hub.
type Option func(*Hub)

// WithMaxConnections rejects new websockets above n. 0 disables the cap.
func WithMaxConnections(n int) Option {
	return func(h *Hub) { h.maxConns = n }
}

// WithWriteQueue sets the per-socket outbound buffer. Full queue drops only
// that client's events.
func WithWriteQueue(n int) Option {
	return func(h *Hub) {
		if n > 0 {
			h.writeQueue = n
		}
	}
}

// WithSlowWrite logs writes slower than d.
func WithSlowWrite(d time.Duration) Option {
	return func(h *Hub) { h.slowWrite = d }
}

// WithStuckWrite disconnects a socket whose WriteJSON has been blocked for d.
func WithStuckWrite(d time.Duration) Option {
	return func(h *Hub) { h.stuckWrite = d }
}
