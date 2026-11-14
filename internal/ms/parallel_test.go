package ms

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestParallelReturnsTheFirstErrorInArgumentOrder(t *testing.T) {
	first, second := errors.New("first"), errors.New("second")
	var ran atomic.Int32
	err := parallel(
		func() error { ran.Add(1); return nil },
		func() error { ran.Add(1); return first },
		func() error { ran.Add(1); return second },
	)
	if !errors.Is(err, first) || ran.Load() != 3 {
		t.Fatalf("err %v after %d calls, want first after 3", err, ran.Load())
	}
	if err := parallel(); err != nil {
		t.Fatal(err)
	}
}

func TestParallelBoundsTheCallsInFlight(t *testing.T) {
	var mu sync.Mutex
	now, peak := 0, 0
	release := make(chan struct{})
	fns := make([]func() error, 3*maxGitJobs)
	for i := range fns {
		fns[i] = func() error {
			mu.Lock()
			now++
			peak = max(peak, now)
			mu.Unlock()
			<-release
			mu.Lock()
			now--
			mu.Unlock()
			return nil
		}
	}
	done := make(chan error)
	go func() { done <- parallel(fns...) }()
	for range fns {
		release <- struct{}{}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak > maxGitJobs {
		t.Fatalf("%d calls in flight, want at most %d", peak, maxGitJobs)
	}
}
