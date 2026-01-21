package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

var errNodeIDEmpty = errors.New("node id cannot be empty")

// NodeService handles unified node business logic.
type NodeService struct {
	FileStore  *FileStore
	gitService *git.Client
	orgService *identity.OrganizationService
}

// NewNodeService creates a new node service.
func NewNodeService(fileStore *FileStore, gitService *git.Client, orgService *identity.OrganizationService) *NodeService {
	return &NodeService{
		FileStore:  fileStore,
		gitService: gitService,
		orgService: orgService,
	}
}

// GetNode retrieves a unified node by ID.
func (s *NodeService) GetNode(ctx context.Context, orgID, id jsonldb.ID) (*Node, error) {
	if id.IsZero() {
		return nil, errNodeIDEmpty
	}

	return s.FileStore.ReadNode(orgID, id)
}

// ListNodes returns the full hierarchical tree of nodes.
func (s *NodeService) ListNodes(ctx context.Context, orgID jsonldb.ID) ([]*Node, error) {
	return s.FileStore.ReadNodeTree(orgID)
}

// CreateNode creates a new node (can be document, database, or hybrid).
func (s *NodeService) CreateNode(ctx context.Context, orgID jsonldb.ID, title string, nodeType NodeType, parentID jsonldb.ID) (*Node, error) {
	// Check Quota
	if s.orgService != nil {
		org, err := s.orgService.Get(orgID)
		if err == nil && org.Quotas.MaxPages > 0 {
			count, _, err := s.FileStore.GetOrganizationUsage(orgID)
			if err == nil && count >= org.Quotas.MaxPages {
				return nil, fmt.Errorf("page quota exceeded (%d/%d)", count, org.Quotas.MaxPages)
			}
		}
	}

	id := jsonldb.NewID()
	now := time.Now()

	node := &Node{
		ID:       id,
		ParentID: parentID,
		Title:    title,
		Type:     nodeType,
		Created:  now,
		Modified: now,
	}

	if nodeType == NodeTypeDocument || nodeType == NodeTypeHybrid {
		_, err := s.FileStore.WritePage(orgID, id, title, "")
		if err != nil {
			return nil, err
		}
	}

	if nodeType == NodeTypeDatabase || nodeType == NodeTypeHybrid {
		dbNode := &Node{
			ID:       id,
			Title:    title,
			Created:  now,
			Modified: now,
			Type:     NodeTypeDatabase,
		}
		err := s.FileStore.WriteDatabase(orgID, dbNode)
		if err != nil {
			return nil, err
		}
	}

	return node, nil
}
