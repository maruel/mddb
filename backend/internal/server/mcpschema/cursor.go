// Defines versioned, scope-bound MCP discovery and content continuation payloads.

package mcpschema

import "github.com/maruel/ksid"

// Cursor binds a continuation to its original query and canonical content.
type Cursor struct {
	Version   int     `json:"v"`
	Workspace ksid.ID `json:"workspace"`
	Kind      string  `json:"kind"`
	Parent    ksid.ID `json:"parent,omitempty"`
	Last      ksid.ID `json:"last,omitempty"`
	Limit     int     `json:"limit"`
	Node      ksid.ID `json:"node,omitempty"`
	Revision  string  `json:"revision,omitempty"`
	Offset    int64   `json:"offset,omitempty"`
}
