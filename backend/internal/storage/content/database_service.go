package content

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/git"
)

var (
	errDatabaseIDEmpty = errors.New("database id cannot be empty")
	errTitleEmpty      = errors.New("title cannot be empty")
	errColumnRequired  = errors.New("at least one column is required")
	errRecordIDEmpty   = errors.New("record id cannot be empty")
)

// DatabaseService handles database business logic.
type DatabaseService struct {
	FileStore   *FileStore
	gitService  *git.Client
	quotaGetter QuotaGetter
}

// NewDatabaseService creates a new database service.
func NewDatabaseService(fileStore *FileStore, gitService *git.Client, quotaGetter QuotaGetter) *DatabaseService {
	return &DatabaseService{
		FileStore:   fileStore,
		gitService:  gitService,
		quotaGetter: quotaGetter,
	}
}

// Get retrieves a database by ID and returns it as a Node.
func (s *DatabaseService) Get(ctx context.Context, orgID, id jsonldb.ID) (*Node, error) {
	if id.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	return s.FileStore.ReadDatabase(orgID, id)
}

// Create creates a new database with a generated numeric ID and returns it as a Node.
func (s *DatabaseService) Create(ctx context.Context, orgID jsonldb.ID, title string, columns []Property) (*Node, error) {
	if title == "" {
		return nil, errTitleEmpty
	}
	if len(columns) == 0 {
		return nil, errColumnRequired
	}

	// Check Quota
	quota, err := s.quotaGetter.GetQuota(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if quota.MaxPages > 0 {
		count, _, err := s.FileStore.GetOrganizationUsage(orgID)
		if err != nil {
			return nil, err
		}
		if count >= quota.MaxPages {
			return nil, fmt.Errorf("page quota exceeded (%d/%d)", count, quota.MaxPages)
		}
	}

	id := jsonldb.NewID()
	now := time.Now()
	node := &Node{
		ID:         id,
		Title:      title,
		Properties: columns,
		Created:    now,
		Modified:   now,
		Type:       NodeTypeDatabase,
	}

	if err := s.FileStore.WriteDatabase(orgID, node); err != nil {
		return nil, err
	}

	if s.gitService != nil {
		msg := fmt.Sprintf("create: database %s - %s", id.String(), title)
		files := []string{"pages/" + id.String() + "/metadata.json"}
		if err := s.gitService.Commit(ctx, orgID.String(), "", "", msg, files); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return node, nil
}

// Update updates an existing database's schema and returns it as a Node.
func (s *DatabaseService) Update(ctx context.Context, orgID, id jsonldb.ID, title string, columns []Property) (*Node, error) {
	if id.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	if title == "" {
		return nil, errTitleEmpty
	}
	if len(columns) == 0 {
		return nil, errColumnRequired
	}

	node, err := s.FileStore.ReadDatabase(orgID, id)
	if err != nil {
		return nil, err
	}

	node.Title = title
	node.Properties = columns
	node.Modified = time.Now()

	if err := s.FileStore.WriteDatabase(orgID, node); err != nil {
		return nil, err
	}

	if s.gitService != nil {
		msg := "update: database " + id.String()
		files := []string{"pages/" + id.String() + "/metadata.json"}
		if err := s.gitService.Commit(ctx, orgID.String(), "", "", msg, files); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return node, nil
}

// Delete deletes a database and all its records.
func (s *DatabaseService) Delete(ctx context.Context, orgID, id jsonldb.ID) error {
	if id.IsZero() {
		return errDatabaseIDEmpty
	}
	if err := s.FileStore.DeleteDatabase(orgID, id); err != nil {
		return err
	}

	if s.gitService != nil {
		msg := "delete: database " + id.String()
		files := []string{"pages/" + id.String()}
		if err := s.gitService.Commit(ctx, orgID.String(), "", "", msg, files); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// List returns all databases as Nodes.
func (s *DatabaseService) List(ctx context.Context, orgID jsonldb.ID) ([]*Node, error) {
	it, err := s.FileStore.IterDatabases(orgID)
	if err != nil {
		return nil, err
	}
	return slices.Collect(it), nil
}

// CreateRecord creates a new record in a database.
// Data values are coerced to SQLite-compatible types based on column schema.
func (s *DatabaseService) CreateRecord(ctx context.Context, orgID, databaseID jsonldb.ID, data map[string]any) (*DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}

	// Read database to get columns for type coercion
	node, err := s.FileStore.ReadDatabase(orgID, databaseID)
	if err != nil {
		return nil, errDatabaseNotFound
	}

	// Coerce data types based on property schema
	coercedData := coerceRecordData(data, node.Properties)

	// Generate record ID
	id := jsonldb.NewID()

	now := time.Now()
	record := &DataRecord{
		ID:       id,
		Data:     coercedData,
		Created:  now,
		Modified: now,
	}

	if err := s.FileStore.AppendRecord(orgID, databaseID, record); err != nil {
		return nil, err
	}

	if s.gitService != nil {
		msg := fmt.Sprintf("create: record %s in database %s", id.String(), databaseID.String())
		files := []string{"pages/" + databaseID.String() + "/data.jsonl"}
		if err := s.gitService.Commit(ctx, orgID.String(), "", "", msg, files); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return record, nil
}

// GetRecords retrieves all records from a database.
func (s *DatabaseService) GetRecords(ctx context.Context, orgID, databaseID jsonldb.ID) ([]*DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}

	// Verify database exists
	if !s.FileStore.DatabaseExists(orgID, databaseID) {
		return nil, errDatabaseNotFound
	}

	it, err := s.FileStore.IterRecords(orgID, databaseID)
	if err != nil {
		return nil, err
	}
	return slices.Collect(it), nil
}

// GetRecordsPage retrieves a subset of records from a database.
func (s *DatabaseService) GetRecordsPage(ctx context.Context, orgID, databaseID jsonldb.ID, offset, limit int) ([]*DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	// Verify database exists
	if !s.FileStore.DatabaseExists(orgID, databaseID) {
		return nil, errDatabaseNotFound
	}

	return s.FileStore.ReadRecordsPage(orgID, databaseID, offset, limit)
}

// GetRecord retrieves a specific record by ID.
func (s *DatabaseService) GetRecord(ctx context.Context, orgID, databaseID, recordID jsonldb.ID) (*DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	if recordID.IsZero() {
		return nil, errRecordIDEmpty
	}

	it, err := s.FileStore.IterRecords(orgID, databaseID)
	if err != nil {
		return nil, err
	}

	for record := range it {
		if record.ID == recordID {
			return record, nil
		}
	}

	return nil, errRecordNotFound
}

// UpdateRecord updates an existing record in a database.
// Data values are coerced to SQLite-compatible types based on column schema.
func (s *DatabaseService) UpdateRecord(ctx context.Context, orgID, databaseID, recordID jsonldb.ID, data map[string]any) (*DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	if recordID.IsZero() {
		return nil, errRecordIDEmpty
	}

	// Read database to get columns for type coercion
	node, err := s.FileStore.ReadDatabase(orgID, databaseID)
	if err != nil {
		return nil, errDatabaseNotFound
	}

	// Retrieve existing record to preserve Created time and ensure it exists
	existing, err := s.GetRecord(ctx, orgID, databaseID, recordID)
	if err != nil {
		return nil, err
	}

	// Coerce data types based on property schema
	coercedData := coerceRecordData(data, node.Properties)

	record := &DataRecord{
		ID:       recordID,
		Data:     coercedData,
		Created:  existing.Created,
		Modified: time.Now(),
	}

	if err := s.FileStore.UpdateRecord(orgID, databaseID, record); err != nil {
		return nil, err
	}

	if s.gitService != nil {
		msg := fmt.Sprintf("update: record %s in database %s", recordID.String(), databaseID.String())
		files := []string{"pages/" + databaseID.String() + "/data.jsonl"}
		if err := s.gitService.Commit(ctx, orgID.String(), "", "", msg, files); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return record, nil
}

// DeleteRecord deletes a record from a database.
func (s *DatabaseService) DeleteRecord(ctx context.Context, orgID, databaseID, recordID jsonldb.ID) error {
	if databaseID.IsZero() {
		return errDatabaseIDEmpty
	}
	if recordID.IsZero() {
		return errRecordIDEmpty
	}

	if err := s.FileStore.DeleteRecord(orgID, databaseID, recordID); err != nil {
		return err
	}

	if s.gitService != nil {
		msg := fmt.Sprintf("delete: record %s from database %s", recordID.String(), databaseID.String())
		files := []string{"pages/" + databaseID.String() + "/data.jsonl"}
		if err := s.gitService.Commit(ctx, orgID.String(), "", "", msg, files); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}
