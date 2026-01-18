package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
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
func (h *NodeHandler) ListNodes(ctx context.Context, req models.ListNodesRequest) (*models.ListNodesResponse, error) {
	orgID := models.GetOrgID(ctx)
	nodes, err := h.fileStore.ReadNodeTree(orgID)
	if err != nil {
		return nil, models.InternalWithError("Failed to read node tree", err)
	}

	return &models.ListNodesResponse{Nodes: nodes}, nil
}

// GetNode retrieves a single node's metadata.
func (h *NodeHandler) GetNode(ctx context.Context, req models.GetNodeRequest) (*models.Node, error) {
	orgID := models.GetOrgID(ctx)
	nodes, err := h.fileStore.ReadNodeTree(orgID)
	if err != nil {
		return nil, models.InternalWithError("Failed to read node tree", err)
	}

	node := findNode(nodes, req.ID)
	if node == nil {
		return nil, models.NotFound("node")
	}

	return node, nil
}

// CreateNode creates a new node (page, database, or hybrid).
func (h *NodeHandler) CreateNode(ctx context.Context, req models.CreateNodeRequest) (*models.Node, error) {
	if req.Title == "" || req.Type == "" {
		return nil, models.MissingField("title or type")
	}

	orgID := models.GetOrgID(ctx)
	id := h.fileStore.NextID(orgID)

	var node *models.Node
	var err error

	switch req.Type {
	case models.NodeTypeDocument:
		var page *models.Page
		page, err = h.fileStore.WritePage(orgID, id, req.Title, "")
		if err == nil {
			node = &models.Node{
				ID:       page.ID,
				Title:    page.Title,
				Content:  page.Content,
				Type:     models.NodeTypeDocument,
				Created:  page.Created,
				Modified: page.Modified,
			}
		}
	case models.NodeTypeDatabase:
		// We use databaseService here for better encapsulation
		ds := storage.NewDatabaseService(h.fileStore, h.gitService, h.cache, h.orgService)
		var db *models.Database
		db, err = ds.CreateDatabase(ctx, req.Title, []models.Column{})
		if err == nil {
			node = &models.Node{
				ID:       db.ID,
				Title:    db.Title,
				Columns:  db.Columns,
				Type:     models.NodeTypeDatabase,
				Created:  db.Created,
				Modified: db.Modified,
			}
		}
	case models.NodeTypeHybrid:
		return nil, models.NotImplemented("hybrid nodes")
	default:
		return nil, models.BadRequest("Invalid node type")
	}

	if err != nil {
		return nil, models.InternalWithError("Failed to create node", err)
	}

	// Commit if git is enabled
	if h.gitService != nil {
		_ = h.gitService.CommitChange(ctx, "create", string(req.Type), id, req.Title)
	}

	// Invalidate cache
	h.cache.InvalidateNodeTree()

	return node, nil
}

func findNode(nodes []*models.Node, id string) *models.Node {
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
