package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NodeHandler handles hierarchical node requests.
type NodeHandler struct {
	nodeService *content.NodeService
	gitService  *git.Client
}

// NewNodeHandler creates a new node handler.
func NewNodeHandler(fileStore *content.FileStore, gitService *git.Client, orgService *identity.OrganizationService) *NodeHandler {
	return &NodeHandler{
		nodeService: content.NewNodeService(fileStore, gitService, orgService),
		gitService:  gitService,
	}
}

// ListNodes returns the hierarchical node tree.
func (h *NodeHandler) ListNodes(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.ListNodesRequest) (*dto.ListNodesResponse, error) {
	nodes, err := h.nodeService.ListNodes(ctx, orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to read node tree", err)
	}
	responses := make([]dto.NodeResponse, 0, len(nodes))
	for _, n := range nodes {
		responses = append(responses, *nodeToResponse(n))
	}
	return &dto.ListNodesResponse{Nodes: responses}, nil
}

// GetNode retrieves a single node's metadata.
func (h *NodeHandler) GetNode(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.GetNodeRequest) (*dto.NodeResponse, error) {
	id, err := decodeID(req.ID, "node_id")
	if err != nil {
		return nil, err
	}
	node, err := h.nodeService.GetNode(ctx, orgID, id)
	if err != nil {
		return nil, dto.NotFound("node")
	}
	return nodeToResponse(node), nil
}

// CreateNode creates a new node (page, database, or hybrid).
func (h *NodeHandler) CreateNode(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.CreateNodeRequest) (*dto.NodeResponse, error) {
	if req.Title == "" || req.Type == "" {
		return nil, dto.MissingField("title or type")
	}

	var nodeType content.NodeType
	switch req.Type {
	case dto.NodeTypeDocument:
		nodeType = content.NodeTypeDocument
	case dto.NodeTypeDatabase:
		nodeType = content.NodeTypeDatabase
	case dto.NodeTypeHybrid:
		nodeType = content.NodeTypeHybrid
	default:
		return nil, dto.BadRequest("Invalid node type")
	}

	node, err := h.nodeService.CreateNode(ctx, orgID, req.Title, nodeType, 0)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create node", err)
	}

	if h.gitService != nil {
		_ = h.gitService.CommitChange(ctx, orgID.String(), "create", string(req.Type), node.ID.String(), req.Title)
	}

	return nodeToResponse(node), nil
}
