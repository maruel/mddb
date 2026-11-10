// Defines mddb conversation selection and watched-resource freshness independently of client transport.

package mcpschema

import "github.com/maruel/gomode"

// ServiceFocus identifies a focused resource independently of a host's domain
// or the current bounded item catalog. Reference is a host-owned display label.
type ServiceFocus struct {
	ID        string `json:"id"`
	Reference string `json:"reference,omitempty"`
	URI       string `json:"uri,omitempty"`
}

// ResourceContextState describes whether a watched resource has been read or
// invalidated. Eviction from a bounded context is neither deletion nor revoked
// authorization. Only an authoritative host check can report unavailable.
type ResourceContextState string

const (
	// ResourceContextChanged means content must be read before answering from it.
	ResourceContextChanged ResourceContextState = "changed"
	// ResourceContextRead records a successfully authorized read or mutation result.
	ResourceContextRead ResourceContextState = "read"
	// ResourceContextUnavailable withdraws a resource after an authoritative check.
	ResourceContextUnavailable ResourceContextState = "unavailable"
)

// ResourceContext is an identity-only context update, never a content snapshot
// or an instruction to start a model turn. Revision is the host's snapshot
// revision; ChangeID is an opaque invalidation watermark and cannot establish
// snapshot equality. Aspects describe which host-owned projections changed.
type ResourceContext struct {
	URI          string                      `json:"uri"`
	Dependencies []gomode.ResourceDependency `json:"dependencies"`
	Revision     string                      `json:"revision,omitempty"`
	ChangeID     string                      `json:"changeId,omitempty"`
	Aspects      []string                    `json:"aspects,omitempty"`
	State        ResourceContextState        `json:"state"`
}

// ConversationContext is a bounded, authorized context projection for one
// conversation. Resources replace watch membership, not document existence.
// A reconnect or resync invalidates knowledge of freshness until reconciled.
type ConversationContext struct {
	// Epoch and Version order context projections only, never document content.
	// Clients accept a new epoch from an authoritative context resource read,
	// then reject older-epoch private tool results still in flight.
	Epoch        string            `json:"epoch"`
	Version      int64             `json:"version"`
	Focus        *ServiceFocus     `json:"focus,omitempty"`
	Resources    []ResourceContext `json:"resources"`
	OmittedCount int               `json:"omittedCount,omitempty"`
}

// ModelResource describes mddb's public freshness semantics, without transport routing.
type ModelResource struct {
	URI      string               `json:"uri"`
	Revision string               `json:"revision,omitempty"`
	Aspects  []string             `json:"aspects,omitempty"`
	State    ResourceContextState `json:"state"`
}

// ModelContext is mddb's public conversation context, interpreted by its MCP instructions.
type ModelContext struct {
	Focus        *ServiceFocus   `json:"focus"`
	Resources    []ModelResource `json:"resources"`
	OmittedCount int             `json:"omittedCount"`
}
