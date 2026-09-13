package manager

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestGetStatsNoDeadlockWithConcurrentWriters(t *testing.T) {
	m := NewManager(10, 0.9)
	for i := 0; i < 30; i++ {
		url := fmt.Sprintf("wss://r%d.example", i)
		m.AddRelay(url)
		m.UpdateHealth(url, true, time.Millisecond)
	}

	const nWriters = 4
	const nReaders = 8
	var writers, readers sync.WaitGroup
	stop := make(chan struct{})

	writers.Add(nWriters)
	for w := 0; w < nWriters; w++ {
		go func(id int) {
			defer writers.Done()
			url := fmt.Sprintf("wss://r%d.example", id)
			for {
				select {
				case <-stop:
					return
				default:
					m.UpdateHealth(url, true, time.Millisecond)
				}
			}
		}(w)
	}

	readers.Add(nReaders)
	for r := 0; r < nReaders; r++ {
		go func() {
			defer readers.Done()
			for i := 0; i < 200; i++ {
				_ = m.GetStats()
				_ = m.GetTopRelays()
				_ = m.GetMandatoryRelays()
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		readers.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(stop)
		writers.Wait()
	case <-time.After(3 * time.Second):
		t.Fatal("GetStats deadlocked with concurrent UpdateHealth (nested RWMutex RLock)")
	}
}

func TestGetStatsIncludesTopAndMandatory(t *testing.T) {
	m := NewManager(2, 0.9)
	m.AddMandatoryRelay("wss://must.example")
	m.UpdateHealth("wss://must.example", true, time.Millisecond)
	m.AddRelay("wss://a.example")
	m.AddRelay("wss://b.example")
	m.UpdateHealth("wss://a.example", true, time.Millisecond)
	m.UpdateHealth("wss://b.example", true, 50*time.Millisecond)

	obj := m.GetStats()
	if obj == nil {
		t.Fatal("nil stats")
	}
	if got := m.GetRelayCount(); got != 3 {
		t.Fatalf("relay count=%d want 3", got)
	}
	top := m.GetTopRelays()
	if len(top) != 2 {
		t.Fatalf("topN=%d want 2", len(top))
	}
	mand := m.GetMandatoryRelays()
	if len(mand) != 1 || mand[0].URL != "wss://must.example" {
		t.Fatalf("mandatory=%v", mand)
	}
}
