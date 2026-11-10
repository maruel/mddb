// Defines bounded record/query snapshots shared by MCP tools and data resources.

package mcpschema

import "github.com/maruel/mddb/backend/internal/server/dto"

// ReadRecordInput selects actual table data rather than the node's schema.
type ReadRecordInput struct {
	NodeID   string `json:"nodeId"`
	RecordID string `json:"recordId"`
}

// QueryRecordsInput selects one bounded window, optionally using a saved view's
// filters and ordering. Windows are independently current, not a frozen table.
type QueryRecordsInput struct {
	NodeID string `json:"nodeId"`
	ViewID string `json:"viewId,omitempty"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// RecordsSnapshot is the canonical data projection for record/query tools and
// resources. Revision hashes this window, including ordering and continuation;
// it is independent of the table node's title/body CAS revision. NextURI means
// further rows exist; omission from a window never proves deletion.
type RecordsSnapshot struct {
	URI      string                   `json:"uri"`
	Revision string                   `json:"revision"`
	Records  []dto.DataRecordResponse `json:"records"`
	NextURI  string                   `json:"nextURI,omitempty"`
}
