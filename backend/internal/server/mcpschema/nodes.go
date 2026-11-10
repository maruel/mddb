// Defines MCP node discovery, content, and mutation wire contracts.

package mcpschema

import "github.com/maruel/mddb/backend/internal/server/dto"

// ListNodesInput defines the list nodes input wire contract.
type ListNodesInput struct {
	Cursor   string `json:"cursor,omitempty" jsonschema:"description=Continuation cursor returned by nodes_list"`
	ParentID string `json:"parentId,omitempty" jsonschema:"description=Parent node ID; omit for workspace root"`
}

// ReadNodeInput defines the read node input wire contract.
type ReadNodeInput struct {
	ContentCursor string `json:"contentCursor,omitempty" jsonschema:"description=Continuation cursor returned by node_read for a long document"`
	NodeID        string `json:"nodeId" jsonschema:"description=Node ID to read"`
}

// NodeSummary defines the node summary wire contract.
type NodeSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	HasChildren bool   `json:"hasChildren"`
}

// ListNodesOutput defines the list nodes output wire contract.
type ListNodesOutput struct {
	URI        string        `json:"uri"`
	Revision   string        `json:"revision"`
	NextURI    string        `json:"nextURI,omitempty"`
	NextCursor string        `json:"nextCursor,omitempty"`
	Nodes      []NodeSummary `json:"nodes"`
}

// ReadNodeOutput defines the read node output wire contract.
type ReadNodeOutput struct {
	Node NodeDetail `json:"node"`
}

// NodeDetail is the canonical bounded snapshot shared by node tools, resources,
// and mutation acknowledgements. Revision covers title/body CAS, while metadata
// can change independently. A continuation is another window, not a deletion.
type NodeDetail struct {
	Revision          string     `json:"revision,omitempty"`
	ContentBytes      int64      `json:"contentBytes,omitempty"`
	ContentTruncated  bool       `json:"contentTruncated,omitempty"`
	NextContentCursor string     `json:"nextContentCursor,omitempty"`
	NextContentURI    string     `json:"nextContentURI,omitempty"`
	ID                string     `json:"id"`
	ParentID          string     `json:"parentId,omitempty"`
	Title             string     `json:"title"`
	Type              string     `json:"type"`
	Content           string     `json:"content,omitempty"`
	Properties        []Property `json:"properties,omitempty"`
	Views             []View     `json:"views,omitempty"`
	Created           float64    `json:"created"`
	Modified          float64    `json:"modified"`
	Tags              []string   `json:"tags,omitempty"`
	FaviconURL        string     `json:"faviconUrl,omitempty"`
	Icon              string     `json:"icon,omitempty"`
	Cover             string     `json:"cover,omitempty"`
}

// UpdateNodeInput defines the update node input wire contract.
type UpdateNodeInput struct {
	ExpectedRevision string `json:"expectedRevision" jsonschema:"description=Revision returned by node_read; required to reject stale edits"`
	NodeID           string `json:"nodeId" jsonschema:"description=Page node ID to update"`
	Title            string `json:"title" jsonschema:"description=New page title"`
	Content          string `json:"content" jsonschema:"description=Full replacement markdown body"`
}

// CreatePageInput defines the create page input wire contract.
type CreatePageInput struct {
	ParentID string `json:"parentId,omitempty" jsonschema:"description=Parent node ID; omit for workspace root"`
	Title    string `json:"title" jsonschema:"description=Page title"`
	Content  string `json:"content,omitempty" jsonschema:"description=Initial markdown body"`
}

// CreateTableInput defines the create table input wire contract.
type CreateTableInput struct {
	ParentID   string         `json:"parentId,omitempty" jsonschema:"description=Parent node ID; omit for workspace root"`
	Title      string         `json:"title" jsonschema:"description=Table title"`
	Properties []dto.Property `json:"properties" jsonschema:"description=Table column schema"`
}

// AppendNodeInput defines the append node input wire contract.
type AppendNodeInput struct {
	ExpectedRevision string `json:"expectedRevision" jsonschema:"description=Revision returned by node_read; required to reject stale appends"`
	NodeID           string `json:"nodeId" jsonschema:"description=Page node ID to append to"`
	Content          string `json:"content" jsonschema:"description=Markdown appended after the existing body"`
}

// WriteNodeOutput defines the write node output wire contract.
type WriteNodeOutput struct {
	Node NodeDetail `json:"node"`
}

// MutationError is the machine-facing com.fghbuild.mddb/mutationError entry in
// a failed tool result's _meta. CurrentRevision was observed when the mutation
// was rejected; recovery "read_current" requires inspecting the latest content
// before deciding whether to write. A conflict cannot prove whether a previous
// attempt succeeded, so clients must not automatically rebase and repeat append.
type MutationError struct {
	Error           string        `json:"error"`
	Code            dto.ErrorCode `json:"code"`
	CurrentRevision string        `json:"currentRevision,omitempty"`
	Recovery        string        `json:"recovery,omitempty"`
}
