// Owns bounded conversation dependencies and reconciles typed invalidations with authorized read snapshots.

package server

import (
	"context"
	"slices"

	"github.com/maruel/mddb/backend/internal/server/mcpschema"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/sse"
)

const voiceContextURI = "mddb://conversation/context"
const maxVoiceContextResources = 32

// voiceContext belongs to the existing focus entry and its login-session lease.
// focus.mu protects its state. Lock order is focus.mu -> broker.mu; neither lock
// is held during content reads or authorization checks. Broker publishing never
// calls back into focus. There is one typed feed per live conversation, not read.
type voiceContext struct {
	feed      *sse.Feed
	cancel    context.CancelFunc
	resources []*watchedResource
	omitted   int
}

// updateInterestLocked runs under the focus owner. Before a child-list read
// reveals its bounded members, scoped ancestry interest closes the handoff;
// completion narrows it to the query and represented child-presence fields.
func (c *voiceContext) updateInterestLocked() (int64, error) {
	interest := sse.EventInterest{}
	for _, w := range c.resources {
		switch w.kind {
		case watchNode:
			interest.Nodes = append(interest.Nodes, w.node)
		case watchRecords:
			interest.Records = append(interest.Records, w.node)
		case watchChildren:
			interest.Children = append(interest.Children, w.node)
		}
	}
	return c.feed.UpdateInterest(&interest)
}

type watchedResourceKind uint8

const (
	watchNode watchedResourceKind = iota
	watchRecords
	watchChildren
)

type watchedResource struct {
	resource        mcpschema.ResourceContext
	node            ksid.ID
	record          ksid.ID
	kind            watchedResourceKind
	members         []ksid.ID
	readingChildren bool
	queuedChildren  []*sse.Change
	childrenGap     string
	readVersion     uint64
	readStart       int64
	readRevision    string
	request         string
	pending         map[dto.EventAspect]*sse.Change
}

type resourceRead struct {
	context *voiceContext
	watch   *watchedResource
	version uint64
}

func readIncludesChange(w *watchedResource, event *sse.Change) bool {
	// Only the server-generated identity of this accepted mutation can prove an
	// echo. An equal title/body CAS alone says nothing about metadata freshness.
	return w.request != "" && event.RequestID == w.request &&
		(event.Revision == "" || event.Revision == w.readRevision)
}

func watchedResourceRemoved(w *watchedResource, event *sse.Change) bool {
	return w.node != 0 && event.Type == dto.EventNodeDeleted && (event.NodeID == w.node || slices.Contains(event.AffectedIDs, w.node))
}

// applyWatchedResourceChange reconciles one authorized source effect under the
// context owner. Child-list bootstrap events wait for actual returned members.
func applyWatchedResourceChange(w *watchedResource, event *sse.Change) bool {
	if event.Sequence() <= w.readStart {
		return false
	}
	if w.kind == watchChildren && w.readingChildren {
		if sse.ChildrenResourceChanged(&event.WorkspaceEvent, w.node, nil) {
			if w.childrenGap != "" || len(w.queuedChildren) == 64 {
				w.childrenGap = event.ChangeID
				w.queuedChildren = nil
			} else {
				w.queuedChildren = append(w.queuedChildren, event)
			}
		}
		return false
	}
	matches := false
	switch w.kind {
	case watchNode:
		matches = sse.NodeResourceChanged(&event.WorkspaceEvent, w.node)
	case watchRecords:
		matches = sse.RecordResourceChanged(&event.WorkspaceEvent, w.node, w.record)
	case watchChildren:
		matches = sse.ChildrenResourceChanged(&event.WorkspaceEvent, w.node, w.members)
	}
	if !matches || readIncludesChange(w, event) {
		return false
	}
	for _, aspect := range event.Aspects {
		w.pending[aspect] = event
	}
	if event.Type == dto.EventResync || event.Type == dto.EventNodeDeleted || len(event.Aspects) == 0 {
		w.pending[""] = event
	}
	w.resource.State = mcpschema.ResourceContextChanged
	if watchedResourceRemoved(w, event) {
		w.resource.State = mcpschema.ResourceContextUnavailable
	}
	w.resource.ChangeID = event.ChangeID
	w.resource.Aspects = nil
	for _, aspect := range event.Aspects {
		w.resource.Aspects = append(w.resource.Aspects, string(aspect))
	}
	if event.Revision != "" && w.kind == watchNode {
		w.resource.Revision = event.Revision
	}
	return true
}
