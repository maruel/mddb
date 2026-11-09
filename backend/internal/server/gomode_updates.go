// Coalesces repeated workspace resource invalidations without delaying a target's first update.

package server

import (
	"slices"
	"time"

	"github.com/maruel/gomode/mcp"
)

// resourceInvalidations belongs to one authorized workspace stream. Its keys
// are bounded by the admitted resource filter plus one catalog target. Only
// repeated state invalidations wait; this owner never carries alert occurrences.
type resourceInvalidations struct {
	last    map[string]time.Time
	pending map[string]time.Time
	timer   *time.Timer
	timerC  <-chan time.Time
}

func newResourceInvalidations() *resourceInvalidations {
	return &resourceInvalidations{last: make(map[string]time.Time), pending: make(map[string]time.Time)}
}

func (p *resourceInvalidations) stop() {
	if p.timer != nil {
		p.timer.Stop()
	}
}

func (p *resourceInvalidations) add(update mcp.ResourceUpdate, resync bool) mcp.ResourceUpdate {
	now := time.Now()
	if resync {
		clear(p.pending)
	}
	targets := update.ResourceURIs
	if update.ResourcesListChanged {
		targets = append(slices.Clone(targets), "")
	}
	ready := make([]string, 0, len(targets))
	for _, target := range targets {
		deadline := p.last[target].Add(8 * time.Millisecond)
		if resync || !now.Before(deadline) {
			ready = append(ready, target)
			p.last[target] = now
			delete(p.pending, target)
		} else {
			p.pending[target] = deadline
		}
	}
	p.arm()
	return resourceInvalidationUpdate(ready)
}

func (p *resourceInvalidations) flush() mcp.ResourceUpdate {
	now := time.Now()
	var ready []string
	for target, deadline := range p.pending {
		if !now.Before(deadline) {
			ready = append(ready, target)
			p.last[target] = now
			delete(p.pending, target)
		}
	}
	p.arm()
	return resourceInvalidationUpdate(ready)
}

func (p *resourceInvalidations) arm() {
	p.stop()
	p.timerC = nil
	var next time.Time
	for _, deadline := range p.pending {
		if next.IsZero() || deadline.Before(next) {
			next = deadline
		}
	}
	if next.IsZero() {
		return
	}
	if p.timer == nil {
		p.timer = time.NewTimer(time.Until(next))
	} else {
		p.timer.Reset(time.Until(next))
	}
	p.timerC = p.timer.C
}

func resourceInvalidationUpdate(targets []string) mcp.ResourceUpdate {
	update := mcp.ResourceUpdate{}
	for _, target := range targets {
		if target == "" {
			update.ResourcesListChanged = true
		} else {
			update.ResourceURIs = append(update.ResourceURIs, target)
		}
	}
	slices.Sort(update.ResourceURIs)
	return update
}
