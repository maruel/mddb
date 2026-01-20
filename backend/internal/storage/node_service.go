package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/entity"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// NodeService handles unified node business logic.
type NodeService struct {
	fileStore  *FileStore
	gitService *GitService
	cache      *Cache
	orgService *OrganizationService
}

// NewNodeService creates a new node service.
func NewNodeService(fileStore *FileStore, gitService *GitService, cache *Cache, orgService *OrganizationService) *NodeService {
	return &NodeService{
		fileStore:  fileStore,
		gitService: gitService,
		cache:      cache,
		orgService: orgService,
	}
}

// GetNode retrieves a unified node by ID.
func (s *NodeService) GetNode(ctx context.Context, idStr string) (*entity.Node, error) {
	if idStr == "" {
		return nil, fmt.Errorf("node id cannot be empty")
	}

	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid node id: %w", err)
	}

	orgID := entity.GetOrgID(ctx)

	// For GetNode, we don't currently cache individual nodes but we could
	// If we have a cached node tree, we could search in it
	if tree := s.cache.GetNodeTree(); tree != nil {
		if node := findNodeInTree(tree, id); node != nil {
			return node, nil
		}
	}

	return s.fileStore.ReadNode(orgID, id)
}

// ListNodes returns the full hierarchical tree of nodes.
func (s *NodeService) ListNodes(ctx context.Context) ([]*entity.Node, error) {
	orgID := entity.GetOrgID(ctx)
	if nodes := s.cache.GetNodeTree(); nodes != nil {
		return nodes, nil
	}

	nodes, err := s.fileStore.ReadNodeTree(orgID)
	if err != nil {
		return nil, err
	}

	s.cache.SetNodeTree(nodes)
	return nodes, nil
}

// CreateNode creates a new node (can be document, database, or hybrid)
func (s *NodeService) CreateNode(ctx context.Context, title string, nodeType entity.NodeType, parentIDStr string) (*entity.Node, error) {
	orgID := entity.GetOrgID(ctx)

	// Check Quota
	if s.orgService != nil {
		org, err := s.orgService.GetOrganization(orgID)
		if err == nil && org.Quotas.MaxPages > 0 {
			count, _, err := s.fileStore.GetOrganizationUsage(orgID)
			if err == nil && count >= org.Quotas.MaxPages {
				return nil, fmt.Errorf("page quota exceeded (%d/%d)", count, org.Quotas.MaxPages)
			}
		}
	}

	id := jsonldb.NewID()
	parentID, _ := jsonldb.DecodeID(parentIDStr) // Empty string decodes to zero ID
	now := time.Now()

	node := &entity.Node{
		ID:       id,
		ParentID: parentID,
		Title:    title,
		Type:     nodeType,
		Created:  now,
		Modified: now,
	}

	// Create physical directory (FileStore handles this through WritePage/WriteDatabase)
	// But we need to support ParentID structure.
	// Currently FileStore uses flat directory for IDs.
	// If ParentID is used, we might want to store it in metadata.

	if nodeType == entity.NodeTypeDocument || nodeType == entity.NodeTypeHybrid {
		_, err := s.fileStore.WritePage(orgID, id, title, "")
		if err != nil {
			return nil, err
		}
	}

	if nodeType == entity.NodeTypeDatabase || nodeType == entity.NodeTypeHybrid {
		db := &entity.Database{
			ID:       id,
			Title:    title,
			Created:  now,
			Modified: now,
		}
		err := s.fileStore.WriteDatabase(orgID, db)
		if err != nil {
			return nil, err
		}
	}

	// Invalidate cache
	s.cache.InvalidateNodeTree()

	return node, nil
}

func findNodeInTree(nodes []*entity.Node, id jsonldb.ID) *entity.Node {
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
