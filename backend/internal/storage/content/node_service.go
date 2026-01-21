package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

var errNodeIDEmpty = errors.New("node id cannot be empty")

// NodeService handles unified node business logic.
type NodeService struct {
	fileStore  *infra.FileStore
	gitService *infra.Git
	orgService *identity.OrganizationService
}

// NewNodeService creates a new node service.
func NewNodeService(fileStore *infra.FileStore, gitService *infra.Git, orgService *identity.OrganizationService) *NodeService {
	return &NodeService{
		fileStore:  fileStore,
		gitService: gitService,
		orgService: orgService,
	}
}

// GetNode retrieves a unified node by ID.
func (s *NodeService) GetNode(ctx context.Context, orgID, id jsonldb.ID) (*entity.Node, error) {
	if id.IsZero() {
		return nil, errNodeIDEmpty
	}

	return s.fileStore.ReadNode(orgID, id)
}

// ListNodes returns the full hierarchical tree of nodes.
func (s *NodeService) ListNodes(ctx context.Context, orgID jsonldb.ID) ([]*entity.Node, error) {
	return s.fileStore.ReadNodeTree(orgID)
}

// CreateNode creates a new node (can be document, database, or hybrid).
func (s *NodeService) CreateNode(ctx context.Context, orgID jsonldb.ID, title string, nodeType entity.NodeType, parentID jsonldb.ID) (*entity.Node, error) {
	// Check Quota
	if s.orgService != nil {
		org, err := s.orgService.Get(orgID)
		if err == nil && org.Quotas.MaxPages > 0 {
			count, _, err := s.fileStore.GetOrganizationUsage(orgID)
			if err == nil && count >= org.Quotas.MaxPages {
				return nil, fmt.Errorf("page quota exceeded (%d/%d)", count, org.Quotas.MaxPages)
			}
		}
	}

	id := jsonldb.NewID()
	now := time.Now()

	node := &entity.Node{
		ID:       id,
		ParentID: parentID,
		Title:    title,
		Type:     nodeType,
		Created:  now,
		Modified: now,
	}

	if nodeType == entity.NodeTypeDocument || nodeType == entity.NodeTypeHybrid {
		_, err := s.fileStore.WritePage(orgID, id, title, "")
		if err != nil {
			return nil, err
		}
	}

	if nodeType == entity.NodeTypeDatabase || nodeType == entity.NodeTypeHybrid {
		dbNode := &entity.Node{
			ID:       id,
			Title:    title,
			Created:  now,
			Modified: now,
			Type:     entity.NodeTypeDatabase,
		}
		err := s.fileStore.WriteDatabase(orgID, dbNode)
		if err != nil {
			return nil, err
		}
	}

	return node, nil
}
