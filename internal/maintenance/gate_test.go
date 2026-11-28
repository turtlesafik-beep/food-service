package maintenance

import (
	"testing"
	"time"
)

const gateTestTimeout = 2 * time.Second

func TestTryReadAllowsNormalRequests(t *testing.T) {
	gate := New()

	if !gate.TryRead() {
		t.Fatal("TryRead must succeed while maintenance is inactive")
	}
	gate.ReadUnlock()
}

func TestTryReadRejectsWhenMaintenanceHoldsWriteLock(t *testing.T) {
	gate := New()
	gate.Lock()
	defer gate.Unlock()

	result := make(chan bool, 1)
	go func() {
		result <- gate.TryRead()
	}()

	select {
	case ok := <-result:
		if ok {
			gate.ReadUnlock()
			t.Fatal("TryRead must fail while maintenance holds the write lock")
		}
	case <-time.After(gateTestTimeout):
		t.Fatal("TryRead blocked while maintenance held the write lock")
	}
}

func TestTryReadRejectsWhenMaintenanceWaitsForWriteLock(t *testing.T) {
	gate := New()
	if !gate.TryRead() {
		t.Fatal("TryRead must succeed while maintenance is inactive")
	}

	locked := make(chan struct{})
	go func() {
		gate.Lock()
		close(locked)
	}()

	deadline := time.Now().Add(gateTestTimeout)
	for !gate.Active() {
		if time.Now().After(deadline) {
			gate.ReadUnlock()
			t.Fatal("maintenance did not start waiting for the write lock")
		}
		time.Sleep(time.Millisecond)
	}

	result := make(chan bool, 1)
	go func() {
		result <- gate.TryRead()
	}()

	select {
	case ok := <-result:
		if ok {
			gate.ReadUnlock()
			t.Fatal("TryRead must fail while maintenance waits for the write lock")
		}
	case <-time.After(gateTestTimeout):
		gate.ReadUnlock()
		t.Fatal("TryRead blocked while maintenance waited for the write lock")
	}

	gate.ReadUnlock()
	<-locked
	gate.Unlock()
}
