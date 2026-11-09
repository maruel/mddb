// Tests immediate workspace invalidations, bounded repeated-target convergence, and gap ordering.

package server

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maruel/gomode/mcp"
)

func TestResourceInvalidations(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		p := newResourceInvalidations()
		t.Cleanup(p.stop)
		first := mcp.ResourceUpdate{ResourcesListChanged: true, ResourceURIs: []string{"items", "page"}}
		if got := p.add(first, false); !got.ResourcesListChanged || !slices.Equal(got.ResourceURIs, first.ResourceURIs) {
			t.Fatalf("first update delayed: %+v", got)
		}
		for range 20 {
			if got := p.add(first, false); got.ResourcesListChanged || len(got.ResourceURIs) > 0 {
				t.Fatalf("repeat bypassed coalescing: %+v", got)
			}
		}
		if got := p.add(mcp.ResourceUpdate{ResourceURIs: []string{"different-page"}}, false); !slices.Equal(got.ResourceURIs, []string{"different-page"}) {
			t.Fatalf("independent target delayed: %+v", got)
		}
		time.Sleep(8 * time.Millisecond)
		select {
		case <-p.timerC:
		default:
			t.Fatal("trailing invalidation exceeded bound")
		}
		if got := p.flush(); !got.ResourcesListChanged || !slices.Equal(got.ResourceURIs, first.ResourceURIs) {
			t.Fatalf("latest repeated state lost: %+v", got)
		}
		p.add(first, false)
		if got := p.add(first, true); !got.ResourcesListChanged || !slices.Equal(got.ResourceURIs, first.ResourceURIs) {
			t.Fatalf("gap recovery delayed: %+v", got)
		}
		time.Sleep(8 * time.Millisecond)
		if got := p.flush(); got.ResourcesListChanged || len(got.ResourceURIs) > 0 {
			t.Fatalf("stale pre-gap batch survived: %+v", got)
		}
	})
}
