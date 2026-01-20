package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// NodeHandler handles hierarchical node requests.
type NodeHandler struct {
	nodeService *storage.NodeService
	fileStore   *storage.FileStore
	gitService  *storage.GitService
	cache       *storage.Cache
	orgService  *storage.OrganizationService
}

// NewNodeHandler creates a new node handler.
func NewNodeHandler(fileStore *storage.FileStore, gitService *storage.GitService, cache *storage.Cache, orgService *storage.OrganizationService) *NodeHandler {
	return &NodeHandler{
		nodeService: storage.NewNodeService(fileStore, gitService, cache, orgService),
		fileStore:   fileStore,
		gitService:  gitService,
		cache:       cache,
		orgService:  orgService,
	}
}

// ListNodes returns the hierarchical node tree.
func (h *NodeHandler) ListNodes(ctx context.Context, req dto.ListNodesRequest) (*dto.ListNodesResponse, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	nodes, err := h.nodeService.ListNodes(ctx, orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to read node tree", err)
	}

	// Convert to response types
	responses := make([]dto.NodeResponse, 0, len(nodes))
	for _, n := range nodes {
		responses = append(responses, *nodeToResponse(n))
	}

	return &dto.ListNodesResponse{Nodes: responses}, nil
}

// GetNode retrieves a single node's metadata.
func (h *NodeHandler) GetNode(ctx context.Context, req dto.GetNodeRequest) (*dto.NodeResponse, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	id, err := jsonldb.DecodeID(req.ID)
	if err != nil {
		return nil, dto.BadRequest("invalid_node_id")
	}

	node, err := h.nodeService.GetNode(ctx, orgID, id)
	if err != nil {
		return nil, dto.NotFound("node")
	}

	return nodeToResponse(node), nil
}

// CreateNode creates a new node (page, database, or hybrid).
func (h *NodeHandler) CreateNode(ctx context.Context, req dto.CreateNodeRequest) (*dto.NodeResponse, error) {
	if req.Title == "" || req.Type == "" {
		return nil, dto.MissingField("title or type")
	}

	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}

	var nodeType entity.NodeType
	switch req.Type {
	case dto.NodeTypeDocument:
		nodeType = entity.NodeTypeDocument
	case dto.NodeTypeDatabase:
		nodeType = entity.NodeTypeDatabase
	case dto.NodeTypeHybrid:
		nodeType = entity.NodeTypeHybrid
	default:
		return nil, dto.BadRequest("Invalid node type")
	}

	node, err := h.nodeService.CreateNode(ctx, orgID, req.Title, nodeType, 0)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create node", err)
	}

	// Commit if git is enabled
	if h.gitService != nil {
		_ = h.gitService.CommitChange(ctx, orgID, "create", string(req.Type), node.ID.String(), req.Title)
	}

	return nodeToResponse(node), nil
}

