package storage

import (
	"fmt"
	"time"

	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/utils"
)

// DatabaseService handles database business logic.
type DatabaseService struct {
	fileStore *FileStore
}

// NewDatabaseService creates a new database service.
func NewDatabaseService(fileStore *FileStore) *DatabaseService {
	return &DatabaseService{fileStore: fileStore}
}

// GetDatabase retrieves a database by ID.
func (s *DatabaseService) GetDatabase(id string) (*models.Database, error) {
	if id == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	return s.fileStore.ReadDatabase(id)
}

// CreateDatabase creates a new database with a generated ID.
func (s *DatabaseService) CreateDatabase(title string, columns []models.Column) (*models.Database, error) {
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("at least one column is required")
	}

	// Generate ID using UUID
	id, err := utils.GenerateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate database id: %w", err)
	}

	// Ensure each column has an ID
	for i := range columns {
		if columns[i].ID == "" {
			colID, err := utils.GenerateID()
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
		Path:     id + ".db.json",
	}

	if err := s.fileStore.WriteDatabase(db); err != nil {
		return nil, err
	}

	return db, nil
}

// UpdateDatabase updates an existing database's schema.
func (s *DatabaseService) UpdateDatabase(id, title string, columns []models.Column) (*models.Database, error) {
	if id == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("at least one column is required")
	}

	db, err := s.fileStore.ReadDatabase(id)
	if err != nil {
		return nil, err
	}

	db.Title = title
	db.Columns = columns
	db.Modified = time.Now()

	if err := s.fileStore.WriteDatabase(db); err != nil {
		return nil, err
	}

	return db, nil
}

// DeleteDatabase deletes a database and all its records.
func (s *DatabaseService) DeleteDatabase(id string) error {
	if id == "" {
		return fmt.Errorf("database id cannot be empty")
	}
	return s.fileStore.DeleteDatabase(id)
}

// ListDatabases returns all databases.
func (s *DatabaseService) ListDatabases() ([]*models.Database, error) {
	return s.fileStore.ListDatabases()
}

// CreateRecord creates a new record in a database.
func (s *DatabaseService) CreateRecord(databaseID string, data map[string]interface{}) (*models.Record, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}

	// Verify database exists
	if !s.fileStore.DatabaseExists(databaseID) {
		return nil, fmt.Errorf("database not found")
	}

	// Generate record ID
	id, err := utils.GenerateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate record id: %w", err)
	}

	now := time.Now()
	record := &models.Record{
		ID:       id,
		Data:     data,
		Created:  now,
		Modified: now,
	}

	if err := s.fileStore.AppendRecord(databaseID, record); err != nil {
		return nil, err
	}

	return record, nil
}

// GetRecords retrieves all records from a database.
func (s *DatabaseService) GetRecords(databaseID string) ([]*models.Record, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}

	// Verify database exists
	if !s.fileStore.DatabaseExists(databaseID) {
		return nil, fmt.Errorf("database not found")
	}

	return s.fileStore.ReadRecords(databaseID)
}

// GetRecord retrieves a specific record by ID.
func (s *DatabaseService) GetRecord(databaseID, recordID string) (*models.Record, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database id cannot be empty")
	}
	if recordID == "" {
		return nil, fmt.Errorf("record id cannot be empty")
	}

	records, err := s.fileStore.ReadRecords(databaseID)
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
