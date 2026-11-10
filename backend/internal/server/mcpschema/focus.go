// Package mcpschema defines MCP workspace, node, and voice context wire contracts.

package mcpschema

// WorkspaceSummary identifies an accessible workspace without choosing a shared default.
type WorkspaceSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Role  string `json:"role"`
}

// WorkspaceListInput requests the accessible workspace catalog.
type WorkspaceListInput struct{}

// WorkspaceListOutput describes the workspaces the current principal can access.
type WorkspaceListOutput struct {
	Workspaces []WorkspaceSummary `json:"workspaces"`
}

// WorkspaceSelectInput selects a workspace for one MCP conversation.
type WorkspaceSelectInput struct {
	WorkspaceID string `json:"workspaceId" jsonschema:"description=Workspace ID from workspaces_list"`
}

// WorkspaceSelectOutput confirms a conversation workspace selection.
type WorkspaceSelectOutput struct {
	Workspace WorkspaceSummary `json:"workspace"`
}

// ServiceItem is a generic bounded node summary used by browser and native voice.
type ServiceItem struct {
	ID        string `json:"id"`
	Reference string `json:"reference"`
	Title     string `json:"title"`
	State     string `json:"state"`
}

// ServiceItemsOutput carries a bounded baseline with an explicit omitted count.
type ServiceItemsOutput struct {
	Items         []ServiceItem `json:"items"`
	MoreItemsHint string        `json:"moreItemsHint,omitempty"`
	OmittedCount  int           `json:"omittedCount,omitempty"`
}
