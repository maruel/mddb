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
type ListNodesRequest struct{}

// ListNodesResponse is a response containing a list of nodes.
type ListNodesResponse struct {
	Nodes []*models.Node `json:"nodes"`
}

// GetNodeRequest is a request to get a node.
type GetNodeRequest struct {
	ID string `path:"id"`
}

// CreateNodeRequest is a request to create a node.
type CreateNodeRequest struct {
	Title string          `json:"title"`
	Type  models.NodeType `json:"type"`
}

// ListNodes returns a list of all nodes
func (h *NodeHandler) ListNodes(ctx context.Context, req ListNodesRequest) (*ListNodesResponse, error) {
	orgID := models.GetOrgID(ctx)
	nodes, err := h.nodeService.ListNodes(orgID)
	if err != nil {
		return nil, errors.InternalWithError("Failed to list nodes", err)
	}
	return &ListNodesResponse{Nodes: nodes}, nil
}

// GetNode returns a specific node by ID
func (h *NodeHandler) GetNode(ctx context.Context, req GetNodeRequest) (*models.Node, error) {
	orgID := models.GetOrgID(ctx)
	node, err := h.nodeService.GetNode(orgID, req.ID)
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
	orgID := models.GetOrgID(ctx)
	// CreateNode now takes parentID as 4th arg, defaulting to empty for now
	node, err := h.nodeService.CreateNode(orgID, req.Title, req.Type, "")
	if err != nil {
		return nil, errors.InternalWithError("Failed to create node", err)
	}
	return node, nil
}
