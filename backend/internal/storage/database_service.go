package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/models"
)

// DatabaseService handles database business logic.
type DatabaseService struct {
	fileStore  *FileStore
	gitService *GitService
	cache      *Cache
	orgService *OrganizationService
}

// NewDatabaseService creates a new database service.
func NewDatabaseService(fileStore *FileStore, gitService *GitService, cache *Cache, orgService *OrganizationService) *DatabaseService {
	return &DatabaseService{
		fileStore:  fileStore,
		gitService: gitService,
		cache:      cache,
		orgService: orgService,
	}
}

// GetDatabase retrieves a database by ID.
func (s *DatabaseService) GetDatabase(ctx context.Context, id string) (*models.Database, error) {
	if id == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	orgID := models.GetOrgID(ctx)
	return s.fileStore.ReadDatabase(orgID, id)
}

// CreateDatabase creates a new database with a generated numeric ID.
func (s *DatabaseService) CreateDatabase(ctx context.Context, title string, columns []models.Column) (*models.Database, error) {
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("at least one column is required")
	}

	orgID := models.GetOrgID(ctx)

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

	// Generate numeric ID (monotonically increasing)
	id := s.fileStore.NextID(orgID)

	// Ensure each column has an ID
	for i := range columns {
		if columns[i].ID == "" {
			colID, err := generateID()
			if err != nil {
				return nil, fmt.Errorf("failed to generate column id: %w", err)
			}
			columns[i].ID = colID
		}
	}

	now := time.Now()
	db := &models.Database{
		ID:       id,
		Title:    title,
		Columns:  columns,
		Created:  now,
		Modified: now,
		Version:  "1.0",
	}

	if err := s.fileStore.WriteDatabase(orgID, db); err != nil {
		return nil, err
	}

	// Invalidate cache
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "create", "database", id, title); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return db, nil
}

// UpdateDatabase updates an existing database's schema.
func (s *DatabaseService) UpdateDatabase(ctx context.Context, id, title string, columns []models.Column) (*models.Database, error) {
	if id == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("at least one column is required")
	}

	orgID := models.GetOrgID(ctx)
	db, err := s.fileStore.ReadDatabase(orgID, id)
	if err != nil {
		return nil, err
	}

	db.Title = title
	db.Columns = columns
	db.Modified = time.Now()

	if err := s.fileStore.WriteDatabase(orgID, db); err != nil {
		return nil, err
	}

	// Invalidate cache
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "update", "database", id, "Updated schema"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return db, nil
}

// DeleteDatabase deletes a database and all its records.
func (s *DatabaseService) DeleteDatabase(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("database id cannot be empty")
	}
	orgID := models.GetOrgID(ctx)
	if err := s.fileStore.DeleteDatabase(orgID, id); err != nil {
		return err
	}

	// Invalidate cache
	s.cache.InvalidateRecords(id)
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "delete", "database", id, "Deleted database"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListDatabases returns all databases.
func (s *DatabaseService) ListDatabases(ctx context.Context) ([]*models.Database, error) {
	orgID := models.GetOrgID(ctx)
	return s.fileStore.ListDatabases(orgID)
}

// CreateRecord creates a new record in a database.
func (s *DatabaseService) CreateRecord(ctx context.Context, databaseID string, data map[string]any) (*models.DataRecord, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}

	orgID := models.GetOrgID(ctx)
	// Verify database exists
	if !s.fileStore.DatabaseExists(orgID, databaseID) {
		return nil, fmt.Errorf("database not found")
	}

	// Generate record ID
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate record id: %w", err)
	}

	now := time.Now()
	record := &models.DataRecord{
		ID:       id,
		Data:     data,
		Created:  now,
		Modified: now,
	}

	if err := s.fileStore.AppendRecord(orgID, databaseID, record); err != nil {
		return nil, err
	}

	// Invalidate records cache
	s.cache.InvalidateRecords(databaseID)

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "create", "record", id, fmt.Sprintf("in database %s", databaseID)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return record, nil
}

// GetRecords retrieves all records from a database.
func (s *DatabaseService) GetRecords(ctx context.Context, databaseID string) ([]*models.DataRecord, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}

	if records, ok := s.cache.GetRecords(databaseID); ok {
		return records, nil
	}

	orgID := models.GetOrgID(ctx)
	// Verify database exists
	if !s.fileStore.DatabaseExists(orgID, databaseID) {
		return nil, fmt.Errorf("database not found")
	}

	records, err := s.fileStore.ReadRecords(orgID, databaseID)
	if err != nil {
		return nil, err
	}

	s.cache.SetRecords(databaseID, records)
	return records, nil
}

// GetRecordsPage retrieves a subset of records from a database.
func (s *DatabaseService) GetRecordsPage(ctx context.Context, databaseID string, offset, limit int) ([]*models.DataRecord, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}

	orgID := models.GetOrgID(ctx)
	// Verify database exists
	if !s.fileStore.DatabaseExists(orgID, databaseID) {
		return nil, fmt.Errorf("database not found")
	}

	return s.fileStore.ReadRecordsPage(orgID, databaseID, offset, limit)
}

// GetRecord retrieves a specific record by ID.
func (s *DatabaseService) GetRecord(ctx context.Context, databaseID, recordID string) (*models.DataRecord, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	if recordID == "" {
		return nil, fmt.Errorf("record id cannot be empty")
	}

	orgID := models.GetOrgID(ctx)
	records, err := s.fileStore.ReadRecords(orgID, databaseID)
	if err != nil {
		return nil, err
	}

	for _, record := range records {
		if record.ID == recordID {
			return record, nil
		}
	}

	return nil, fmt.Errorf("record not found")
}

// UpdateRecord updates an existing record in a database.
func (s *DatabaseService) UpdateRecord(ctx context.Context, databaseID, recordID string, data map[string]any) (*models.DataRecord, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	if recordID == "" {
		return nil, fmt.Errorf("record id cannot be empty")
	}

	orgID := models.GetOrgID(ctx)

	// Retrieve existing record to preserve Created time and ensure it exists
	existing, err := s.GetRecord(ctx, databaseID, recordID)
	if err != nil {
		return nil, err
	}

	record := &models.DataRecord{
		ID:       recordID,
		Data:     data,
		Created:  existing.Created,
		Modified: time.Now(),
	}

	if err := s.fileStore.UpdateRecord(orgID, databaseID, record); err != nil {
		return nil, err
	}

	// Invalidate records cache
	s.cache.InvalidateRecords(databaseID)

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "update", "record", recordID, fmt.Sprintf("in database %s", databaseID)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return record, nil
}

// DeleteRecord deletes a record from a database.
func (s *DatabaseService) DeleteRecord(ctx context.Context, databaseID, recordID string) error {
	if databaseID == "" {
		return fmt.Errorf("database id cannot be empty")
	}
	if recordID == "" {
		return fmt.Errorf("record id cannot be empty")
	}

	orgID := models.GetOrgID(ctx)

	if err := s.fileStore.DeleteRecord(orgID, databaseID, recordID); err != nil {
		return err
	}

	// Invalidate records cache
	s.cache.InvalidateRecords(databaseID)

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "delete", "record", recordID, fmt.Sprintf("from database %s", databaseID)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}
