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
	fileStore  *storage.FileStore
	gitService *storage.GitService
	cache      *storage.Cache
	orgService *storage.OrganizationService
}

// NewNodeHandler creates a new node handler.
func NewNodeHandler(fileStore *storage.FileStore, gitService *storage.GitService, cache *storage.Cache, orgService *storage.OrganizationService) *NodeHandler {
	return &NodeHandler{
		fileStore:  fileStore,
		gitService: gitService,
		cache:      cache,
		orgService: orgService,
	}
}

// ListNodes returns the hierarchical node tree.
func (h *NodeHandler) ListNodes(ctx context.Context, req dto.ListNodesRequest) (*dto.ListNodesResponse, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	nodes, err := h.fileStore.ReadNodeTree(orgID)
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

	nodes, err := h.fileStore.ReadNodeTree(orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to read node tree", err)
	}

	node := findNode(nodes, id)
	if node == nil {
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
	id := jsonldb.NewID()

	var node *entity.Node

	switch req.Type {
	case dto.NodeTypeDocument:
		var page *entity.Page
		page, err = h.fileStore.WritePage(orgID, id, req.Title, "")
		if err == nil {
			node = &entity.Node{
				ID:       page.ID,
				Title:    page.Title,
				Content:  page.Content,
				Type:     entity.NodeTypeDocument,
				Created:  page.Created,
				Modified: page.Modified,
			}
		}
	case dto.NodeTypeDatabase:
		// We use databaseService here for better encapsulation
		ds := storage.NewDatabaseService(h.fileStore, h.gitService, h.cache, h.orgService)
		var db *entity.Database
		db, err = ds.CreateDatabase(ctx, orgID, req.Title, []entity.Property{})
		if err == nil {
			node = &entity.Node{
				ID:         db.ID,
				Title:      db.Title,
				Properties: db.Properties,
				Type:       entity.NodeTypeDatabase,
				Created:    db.Created,
				Modified:   db.Modified,
			}
		}
	case dto.NodeTypeHybrid:
		return nil, dto.NotImplemented("hybrid nodes")
	default:
		return nil, dto.BadRequest("Invalid node type")
	}

	if err != nil {
		return nil, dto.InternalWithError("Failed to create node", err)
	}

	// Commit if git is enabled
	if h.gitService != nil {
		_ = h.gitService.CommitChange(ctx, orgID, "create", string(req.Type), id.String(), req.Title)
	}

	// Invalidate cache
	h.cache.InvalidateNodeTree()

	return nodeToResponse(node), nil
}

func findNode(nodes []*entity.Node, id jsonldb.ID) *entity.Node {
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
		if child := findNode(n.Children, id); child != nil {
			return child
		}
	}
	return nil
}
