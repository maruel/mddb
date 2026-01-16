package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// NodeHandler handles unified node HTTP requests
type NodeHandler struct {
	nodeService *storage.NodeService
}

// NewNodeHandler creates a new node handler
func NewNodeHandler(fileStore *storage.FileStore, gitService *storage.GitService, cache *storage.Cache) *NodeHandler {
	return &NodeHandler{
		nodeService: storage.NewNodeService(fileStore, gitService, cache),
	}
}

// ListNodesRequest is a request to list all nodes.
type ListNodesRequest struct {
	OrgID string `path:"orgID"`
}

// ListNodesResponse is a response containing a list of nodes.
type ListNodesResponse struct {
	Nodes []*models.Node `json:"nodes"`
}

// GetNodeRequest is a request to get a node.
type GetNodeRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// CreateNodeRequest is a request to create a node.
type CreateNodeRequest struct {
	OrgID string          `path:"orgID"`
	Title string          `json:"title"`
	Type  models.NodeType `json:"type"`
}

// ListNodes returns a list of all nodes
func (h *NodeHandler) ListNodes(ctx context.Context, req ListNodesRequest) (*ListNodesResponse, error) {
	nodes, err := h.nodeService.ListNodes(ctx)
	if err != nil {
		return nil, errors.InternalWithError("Failed to list nodes", err)
	}
	return &ListNodesResponse{Nodes: nodes}, nil
}

// GetNode returns a specific node by ID
func (h *NodeHandler) GetNode(ctx context.Context, req GetNodeRequest) (*models.Node, error) {
	node, err := h.nodeService.GetNode(ctx, req.ID)
	if err != nil {
		return nil, errors.NotFound("node")
	}
	return node, nil
}

// CreateNode creates a new node
func (h *NodeHandler) CreateNode(ctx context.Context, req CreateNodeRequest) (*models.Node, error) {
	if req.Title == "" {
		return nil, errors.MissingField("title")
	}
	// CreateNode now takes parentID as 4th arg, defaulting to empty for now
	node, err := h.nodeService.CreateNode(ctx, req.Title, req.Type, "")
	if err != nil {
		return nil, errors.InternalWithError("Failed to create node", err)
	}
	return node, nil
}
