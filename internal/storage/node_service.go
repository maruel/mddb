package storage

import (
	"fmt"

	"github.com/maruel/mddb/internal/models"
)

// NodeService handles unified node business logic.
type NodeService struct {
	fileStore  *FileStore
	gitService *GitService
	cache      *Cache
}

// NewNodeService creates a new node service.
func NewNodeService(fileStore *FileStore, gitService *GitService, cache *Cache) *NodeService {
	return &NodeService{
		fileStore:  fileStore,
		gitService: gitService,
		cache:      cache,
	}
}

// GetNode retrieves a node by ID.
func (s *NodeService) GetNode(id string) (*models.Node, error) {
	if id == "" {
		return nil, fmt.Errorf("node id cannot be empty")
	}

	// For GetNode, we don't currently cache individual nodes but we could
	// If we have a cached node tree, we could search in it
	if tree := s.cache.GetNodeTree(); tree != nil {
		if node := findNodeInTree(tree, id); node != nil {
			return node, nil
		}
	}

	return s.fileStore.ReadNode(id)
}

// ListNodes returns all nodes as a hierarchical tree.
func (s *NodeService) ListNodes() ([]*models.Node, error) {
	if nodes := s.cache.GetNodeTree(); nodes != nil {
		return nodes, nil
	}

	nodes, err := s.fileStore.ReadNodeTree()
	if err != nil {
		return nil, err
	}

	s.cache.SetNodeTree(nodes)
	return nodes, nil
}

// CreateNode creates a new unified node.
func (s *NodeService) CreateNode(title string, nodeType models.NodeType) (*models.Node, error) {
	id := s.fileStore.NextID()

	var err error

	if nodeType == models.NodeTypeDatabase {
		db := &models.Database{
			ID:      id,
			Title:   title,
			Columns: []models.Column{{ID: "1", Name: "Name", Type: "text"}},
		}
		err = s.fileStore.WriteDatabase(db)
	} else {
		// Default to document
		_, err = s.fileStore.WritePage(id, title, "")
	}

	if err != nil {
		return nil, err
	}

	// Invalidate cache
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange("create", "node", id, title); err != nil {
			// Log error but don't fail node creation
			fmt.Printf("Warning: failed to commit change: %v\n", err)
		}
	}

	return s.fileStore.ReadNode(id)
}

func findNodeInTree(nodes []*models.Node, id string) *models.Node {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
		if child := findNodeInTree(node.Children, id); child != nil {
			return child
		}
	}
	return nil
}
