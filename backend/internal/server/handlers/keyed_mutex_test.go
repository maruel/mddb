// Tests that resource creation locks isolate keys and release queued callers.

package handlers

import (
	"sync"
	"testing"
	"time"

	"github.com/maruel/ksid"
)

func TestKeyedMutex(t *testing.T) {
	var locks keyedMutex
	firstID := ksid.NewID()
	otherID := ksid.NewID()
	releaseFirst := sync.OnceFunc(locks.lock(firstID))
	t.Cleanup(releaseFirst)

	var wg sync.WaitGroup
	otherAcquired := make(chan struct{})
	wg.Go(func() {
		release := locks.lock(otherID)
		release()
		close(otherAcquired)
	})
	select {
	case <-otherAcquired:
	case <-time.After(time.Second):
		t.Fatal("an unrelated key was blocked")
	}

	sameAcquired := make(chan struct{})
	wg.Go(func() {
		release := locks.lock(firstID)
		release()
		close(sameAcquired)
	})
	select {
	case <-sameAcquired:
		t.Fatal("the same key acquired the lock twice")
	case <-time.After(20 * time.Millisecond):
	}

	releaseFirst()
	select {
	case <-sameAcquired:
		wg.Wait()
	case <-time.After(time.Second):
		t.Fatal("queued caller did not acquire the released lock")
	}
}
