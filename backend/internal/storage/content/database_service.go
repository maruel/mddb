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

var (
	errDatabaseIDEmpty  = errors.New("database id cannot be empty")
	errTitleEmpty       = errors.New("title cannot be empty")
	errColumnRequired   = errors.New("at least one column is required")
	errDatabaseNotFound = errors.New("database not found")
	errRecordIDEmpty    = errors.New("record id cannot be empty")
	errRecordNotFound   = errors.New("record not found")
)

// DatabaseService handles database business logic.
type DatabaseService struct {
	fileStore  *infra.FileStore
	gitService *infra.Git
	orgService *identity.OrganizationService
}

// NewDatabaseService creates a new database service.
func NewDatabaseService(fileStore *infra.FileStore, gitService *infra.Git, orgService *identity.OrganizationService) *DatabaseService {
	return &DatabaseService{
		fileStore:  fileStore,
		gitService: gitService,
		orgService: orgService,
	}
}

// GetDatabase retrieves a database by ID and returns it as a Node.
func (s *DatabaseService) GetDatabase(ctx context.Context, orgID, id jsonldb.ID) (*entity.Node, error) {
	if id.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	return s.fileStore.ReadDatabase(orgID, id)
}

// CreateDatabase creates a new database with a generated numeric ID and returns it as a Node.
func (s *DatabaseService) CreateDatabase(ctx context.Context, orgID jsonldb.ID, title string, columns []entity.Property) (*entity.Node, error) {
	if title == "" {
		return nil, errTitleEmpty
	}
	if len(columns) == 0 {
		return nil, errColumnRequired
	}

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
	now := time.Now()
	node := &entity.Node{
		ID:         id,
		Title:      title,
		Properties: columns,
		Created:    now,
		Modified:   now,
		Type:       entity.NodeTypeDatabase,
	}

	if err := s.fileStore.WriteDatabase(orgID, node); err != nil {
		return nil, err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "create", "database", id.String(), title); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return node, nil
}

// UpdateDatabase updates an existing database's schema and returns it as a Node.
func (s *DatabaseService) UpdateDatabase(ctx context.Context, orgID, id jsonldb.ID, title string, columns []entity.Property) (*entity.Node, error) {
	if id.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	if title == "" {
		return nil, errTitleEmpty
	}
	if len(columns) == 0 {
		return nil, errColumnRequired
	}

	node, err := s.fileStore.ReadDatabase(orgID, id)
	if err != nil {
		return nil, err
	}

	node.Title = title
	node.Properties = columns
	node.Modified = time.Now()

	if err := s.fileStore.WriteDatabase(orgID, node); err != nil {
		return nil, err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "update", "database", id.String(), "Updated schema"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return node, nil
}

// DeleteDatabase deletes a database and all its records.
func (s *DatabaseService) DeleteDatabase(ctx context.Context, orgID, id jsonldb.ID) error {
	if id.IsZero() {
		return errDatabaseIDEmpty
	}
	if err := s.fileStore.DeleteDatabase(orgID, id); err != nil {
		return err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "delete", "database", id.String(), "Deleted database"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListDatabases returns all databases as Nodes.
func (s *DatabaseService) ListDatabases(ctx context.Context, orgID jsonldb.ID) ([]*entity.Node, error) {
	return s.fileStore.ListDatabases(orgID)
}

// CreateRecord creates a new record in a database.
// Data values are coerced to SQLite-compatible types based on column schema.
func (s *DatabaseService) CreateRecord(ctx context.Context, orgID, databaseID jsonldb.ID, data map[string]any) (*entity.DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}

	// Read database to get columns for type coercion
	node, err := s.fileStore.ReadDatabase(orgID, databaseID)
	if err != nil {
		return nil, errDatabaseNotFound
	}

	// Coerce data types based on property schema
	coercedData := coerceRecordData(data, node.Properties)

	// Generate record ID
	id := jsonldb.NewID()

	now := time.Now()
	record := &entity.DataRecord{
		ID:       id,
		Data:     coercedData,
		Created:  now,
		Modified: now,
	}

	if err := s.fileStore.AppendRecord(orgID, databaseID, record); err != nil {
		return nil, err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "create", "record", id.String(), "in database "+databaseID.String()); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return record, nil
}

// GetRecords retrieves all records from a database.
func (s *DatabaseService) GetRecords(ctx context.Context, orgID, databaseID jsonldb.ID) ([]*entity.DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}

	// Verify database exists
	if !s.fileStore.DatabaseExists(orgID, databaseID) {
		return nil, errDatabaseNotFound
	}

	return s.fileStore.ReadRecords(orgID, databaseID)
}

// GetRecordsPage retrieves a subset of records from a database.
func (s *DatabaseService) GetRecordsPage(ctx context.Context, orgID, databaseID jsonldb.ID, offset, limit int) ([]*entity.DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	// Verify database exists
	if !s.fileStore.DatabaseExists(orgID, databaseID) {
		return nil, errDatabaseNotFound
	}

	return s.fileStore.ReadRecordsPage(orgID, databaseID, offset, limit)
}

// GetRecord retrieves a specific record by ID.
func (s *DatabaseService) GetRecord(ctx context.Context, orgID, databaseID, recordID jsonldb.ID) (*entity.DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	if recordID.IsZero() {
		return nil, errRecordIDEmpty
	}

	records, err := s.fileStore.ReadRecords(orgID, databaseID)
	if err != nil {
		return nil, err
	}

	for _, record := range records {
		if record.ID == recordID {
			return record, nil
		}
	}

	return nil, errRecordNotFound
}

// UpdateRecord updates an existing record in a database.
// Data values are coerced to SQLite-compatible types based on column schema.
func (s *DatabaseService) UpdateRecord(ctx context.Context, orgID, databaseID, recordID jsonldb.ID, data map[string]any) (*entity.DataRecord, error) {
	if databaseID.IsZero() {
		return nil, errDatabaseIDEmpty
	}
	if recordID.IsZero() {
		return nil, errRecordIDEmpty
	}

	// Read database to get columns for type coercion
	node, err := s.fileStore.ReadDatabase(orgID, databaseID)
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

	record := &entity.DataRecord{
		ID:       recordID,
		Data:     coercedData,
		Created:  existing.Created,
		Modified: time.Now(),
	}

	if err := s.fileStore.UpdateRecord(orgID, databaseID, record); err != nil {
		return nil, err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "update", "record", recordID.String(), "in database "+databaseID.String()); err != nil {
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

	if err := s.fileStore.DeleteRecord(orgID, databaseID, recordID); err != nil {
		return err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "delete", "record", recordID.String(), "from database "+databaseID.String()); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}
