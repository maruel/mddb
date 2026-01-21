// Package content provides services for git-backed content management.
//
// This package handles file-based storage with git versioning for:
//   - Pages (markdown documents)
//   - Databases (structured data with JSONL storage)
//   - Records (database rows)
//   - Assets (file attachments)
//   - Search (full-text search across content)
package content

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

var (
	errPageTitleEmpty     = errors.New("title cannot be empty")
	errGitServiceNotAvail = errors.New("git service not available")
)

// PageService handles page business logic.
type PageService struct {
	fileStore   *infra.FileStore
	gitService  *infra.Git
	quotaGetter QuotaGetter
}

// NewPageService creates a new page service.
func NewPageService(fileStore *infra.FileStore, gitService *infra.Git, quotaGetter QuotaGetter) *PageService {
	return &PageService{
		fileStore:   fileStore,
		gitService:  gitService,
		quotaGetter: quotaGetter,
	}
}

// GetPage retrieves a page by ID and returns it as a Node.
func (s *PageService) GetPage(ctx context.Context, orgID, id jsonldb.ID) (*entity.Node, error) {
	if id.IsZero() {
		return nil, errPageIDEmpty
	}

	return s.fileStore.ReadPage(orgID, id)
}

// CreatePage creates a new page with a generated numeric ID and returns it as a Node.
func (s *PageService) CreatePage(ctx context.Context, orgID jsonldb.ID, title, content string) (*entity.Node, error) {
	if title == "" {
		return nil, errPageTitleEmpty
	}

	// Check Quota
	quota, err := s.quotaGetter.GetQuota(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if quota.MaxPages > 0 {
		count, _, err := s.fileStore.GetOrganizationUsage(orgID)
		if err != nil {
			return nil, err
		}
		if count >= quota.MaxPages {
			return nil, fmt.Errorf("page quota exceeded (%d/%d)", count, quota.MaxPages)
		}
	}

	id := jsonldb.NewID()

	node, err := s.fileStore.WritePage(orgID, id, title, content)
	if err != nil {
		return nil, err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "create", "page", id.String(), title); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return node, nil
}

// UpdatePage updates an existing page and returns it as a Node.
func (s *PageService) UpdatePage(ctx context.Context, orgID, id jsonldb.ID, title, content string) (*entity.Node, error) {
	if id.IsZero() {
		return nil, errPageIDEmpty
	}
	if title == "" {
		return nil, errPageTitleEmpty
	}

	node, err := s.fileStore.UpdatePage(orgID, id, title, content)
	if err != nil {
		return nil, err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "update", "page", id.String(), "Updated content"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return node, nil
}

// DeletePage deletes a page.
func (s *PageService) DeletePage(ctx context.Context, orgID, id jsonldb.ID) error {
	if id.IsZero() {
		return errPageIDEmpty
	}
	if err := s.fileStore.DeletePage(orgID, id); err != nil {
		return err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "delete", "page", id.String(), "Deleted page"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListPages returns all pages as Nodes.
func (s *PageService) ListPages(ctx context.Context, orgID jsonldb.ID) ([]*entity.Node, error) {
	it, err := s.fileStore.IterPages(orgID)
	if err != nil {
		return nil, err
	}
	return slices.Collect(it), nil
}

// SearchPages performs a simple text search across pages.
func (s *PageService) SearchPages(ctx context.Context, orgID jsonldb.ID, query string) ([]*entity.Node, error) {
	if query == "" {
		return []*entity.Node{}, nil
	}

	it, err := s.fileStore.IterPages(orgID)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var results []*entity.Node

	for node := range it {
		if strings.Contains(strings.ToLower(node.Title), queryLower) ||
			strings.Contains(strings.ToLower(node.Content), queryLower) {
			results = append(results, node)
		}
	}

	return results, nil
}

// GetPageHistory returns the commit history for a page.
func (s *PageService) GetPageHistory(ctx context.Context, orgID, id jsonldb.ID) ([]*entity.Commit, error) {
	if s.gitService == nil {
		return []*entity.Commit{}, nil
	}
	return s.gitService.GetHistory(ctx, orgID, "page", id.String())
}

// GetPageVersion returns the content of a page at a specific commit.
func (s *PageService) GetPageVersion(ctx context.Context, orgID, id jsonldb.ID, commitHash string) (string, error) {
	if s.gitService == nil {
		return "", errGitServiceNotAvail
	}

	// New storage path: {orgID}/pages/{id}/index.md
	path := fmt.Sprintf("%s/pages/%s/index.md", orgID.String(), id.String())
	if orgID.IsZero() {
		path = fmt.Sprintf("pages/%s/index.md", id.String())
	}

	contentBytes, err := s.gitService.GetFileAtCommit(ctx, orgID, commitHash, path)
	if err != nil {
		return "", err
	}

	return string(contentBytes), nil
}
