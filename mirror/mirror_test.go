package mirror

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestEnqueueMirrorEventDropsWhenFull(t *testing.T) {
	ch := make(chan *nostr.Event, 2)
	if !enqueueMirrorEvent(ch, &nostr.Event{ID: "a"}) || !enqueueMirrorEvent(ch, &nostr.Event{ID: "b"}) {
		t.Fatal("expected first two enqueues to succeed")
	}
	if enqueueMirrorEvent(ch, &nostr.Event{ID: "c"}) {
		t.Fatal("expected full queue to drop")
	}
	if got := <-ch; got.ID != "a" {
		t.Fatalf("got %s want a", got.ID)
	}
}

func TestNoteDropRateLimited(t *testing.T) {
	m := NewMirrorManager([]string{"wss://x.example"})
	atomic.StoreInt64(&m.lastDropLog, time.Now().Unix())
	for i := 0; i < 50; i++ {
		m.noteDrop()
	}
	if got := atomic.LoadInt64(&m.droppedEvents); got != 50 {
		t.Fatalf("dropped=%d want 50", got)
	}
}

func TestStatsIncludesDroppedEvents(t *testing.T) {
	m := NewMirrorManager([]string{"wss://x.example"})
	atomic.StoreInt64(&m.droppedEvents, 7)
	s := m.Stats()
	if s.DroppedEvents != 7 {
		t.Fatalf("DroppedEvents=%d want 7", s.DroppedEvents)
	}
}
