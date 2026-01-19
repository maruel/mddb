package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
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
func (s *DatabaseService) GetDatabase(ctx context.Context, idStr string) (*models.Database, error) {
	if idStr == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid database id: %w", err)
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

	id := jsonldb.NewID()

	// Ensure each column has an ID
	for i := range columns {
		if columns[i].ID.IsZero() {
			columns[i].ID = jsonldb.NewID()
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
		if err := s.gitService.CommitChange(ctx, "create", "database", id.String(), title); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return db, nil
}

// UpdateDatabase updates an existing database's schema.
func (s *DatabaseService) UpdateDatabase(ctx context.Context, idStr, title string, columns []models.Column) (*models.Database, error) {
	if idStr == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("at least one column is required")
	}

	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid database id: %w", err)
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
		if err := s.gitService.CommitChange(ctx, "update", "database", idStr, "Updated schema"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return db, nil
}

// DeleteDatabase deletes a database and all its records.
func (s *DatabaseService) DeleteDatabase(ctx context.Context, idStr string) error {
	if idStr == "" {
		return fmt.Errorf("database id cannot be empty")
	}
	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return fmt.Errorf("invalid database id: %w", err)
	}
	orgID := models.GetOrgID(ctx)
	if err := s.fileStore.DeleteDatabase(orgID, id); err != nil {
		return err
	}

	// Invalidate cache
	s.cache.InvalidateRecords(id)
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "delete", "database", idStr, "Deleted database"); err != nil {
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
// Data values are coerced to SQLite-compatible types based on column schema.
func (s *DatabaseService) CreateRecord(ctx context.Context, databaseIDStr string, data map[string]any) (*models.DataRecord, error) {
	if databaseIDStr == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}

	databaseID, err := jsonldb.DecodeID(databaseIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid database id: %w", err)
	}

	orgID := models.GetOrgID(ctx)

	// Read database to get columns for type coercion
	db, err := s.fileStore.ReadDatabase(orgID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("database not found")
	}

	// Coerce data types based on column schema
	coercedData := jsonldb.CoerceDataWithTypes(data, columnTypeMap(db.Columns))

	// Generate record ID
	id := jsonldb.NewID()

	now := time.Now()
	record := &models.DataRecord{
		ID:       id,
		Data:     coercedData,
		Created:  now,
		Modified: now,
	}

	if err := s.fileStore.AppendRecord(orgID, databaseID, record); err != nil {
		return nil, err
	}

	// Invalidate records cache
	s.cache.InvalidateRecords(databaseID)

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "create", "record", id.String(), fmt.Sprintf("in database %s", databaseIDStr)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return record, nil
}

// GetRecords retrieves all records from a database.
func (s *DatabaseService) GetRecords(ctx context.Context, databaseIDStr string) ([]*models.DataRecord, error) {
	if databaseIDStr == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}

	databaseID, err := jsonldb.DecodeID(databaseIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid database id: %w", err)
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
func (s *DatabaseService) GetRecordsPage(ctx context.Context, databaseIDStr string, offset, limit int) ([]*models.DataRecord, error) {
	if databaseIDStr == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}

	databaseID, err := jsonldb.DecodeID(databaseIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid database id: %w", err)
	}

	orgID := models.GetOrgID(ctx)
	// Verify database exists
	if !s.fileStore.DatabaseExists(orgID, databaseID) {
		return nil, fmt.Errorf("database not found")
	}

	return s.fileStore.ReadRecordsPage(orgID, databaseID, offset, limit)
}

// GetRecord retrieves a specific record by ID.
func (s *DatabaseService) GetRecord(ctx context.Context, databaseIDStr, recordIDStr string) (*models.DataRecord, error) {
	if databaseIDStr == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	if recordIDStr == "" {
		return nil, fmt.Errorf("record id cannot be empty")
	}

	databaseID, err := jsonldb.DecodeID(databaseIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid database id: %w", err)
	}
	recordID, err := jsonldb.DecodeID(recordIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid record id: %w", err)
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
// Data values are coerced to SQLite-compatible types based on column schema.
func (s *DatabaseService) UpdateRecord(ctx context.Context, databaseIDStr, recordIDStr string, data map[string]any) (*models.DataRecord, error) {
	if databaseIDStr == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	if recordIDStr == "" {
		return nil, fmt.Errorf("record id cannot be empty")
	}

	databaseID, err := jsonldb.DecodeID(databaseIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid database id: %w", err)
	}
	recordID, err := jsonldb.DecodeID(recordIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid record id: %w", err)
	}

	orgID := models.GetOrgID(ctx)

	// Read database to get columns for type coercion
	db, err := s.fileStore.ReadDatabase(orgID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("database not found")
	}

	// Retrieve existing record to preserve Created time and ensure it exists
	existing, err := s.GetRecord(ctx, databaseIDStr, recordIDStr)
	if err != nil {
		return nil, err
	}

	// Coerce data types based on column schema
	coercedData := jsonldb.CoerceDataWithTypes(data, columnTypeMap(db.Columns))

	record := &models.DataRecord{
		ID:       recordID,
		Data:     coercedData,
		Created:  existing.Created,
		Modified: time.Now(),
	}

	if err := s.fileStore.UpdateRecord(orgID, databaseID, record); err != nil {
		return nil, err
	}

	// Invalidate records cache
	s.cache.InvalidateRecords(databaseID)

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "update", "record", recordIDStr, fmt.Sprintf("in database %s", databaseIDStr)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return record, nil
}

// DeleteRecord deletes a record from a database.
func (s *DatabaseService) DeleteRecord(ctx context.Context, databaseIDStr, recordIDStr string) error {
	if databaseIDStr == "" {
		return fmt.Errorf("database id cannot be empty")
	}
	if recordIDStr == "" {
		return fmt.Errorf("record id cannot be empty")
	}

	databaseID, err := jsonldb.DecodeID(databaseIDStr)
	if err != nil {
		return fmt.Errorf("invalid database id: %w", err)
	}
	recordID, err := jsonldb.DecodeID(recordIDStr)
	if err != nil {
		return fmt.Errorf("invalid record id: %w", err)
	}

	orgID := models.GetOrgID(ctx)

	if err := s.fileStore.DeleteRecord(orgID, databaseID, recordID); err != nil {
		return err
	}

	// Invalidate records cache
	s.cache.InvalidateRecords(databaseID)

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "delete", "record", recordIDStr, fmt.Sprintf("from database %s", databaseIDStr)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// columnTypeMap builds a map of column name to type from a slice of models.Column.
func columnTypeMap(columns []models.Column) map[string]string {
	m := make(map[string]string, len(columns))
	for _, col := range columns {
		m[col.Name] = col.Type
	}
	return m
}
