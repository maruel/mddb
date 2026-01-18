package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// DatabaseHandler handles database-related HTTP requests
type DatabaseHandler struct {
	databaseService *storage.DatabaseService
}

// NewDatabaseHandler creates a new database handler
func NewDatabaseHandler(fileStore *storage.FileStore, gitService *storage.GitService, cache *storage.Cache, orgService *storage.OrganizationService) *DatabaseHandler {
	return &DatabaseHandler{
		databaseService: storage.NewDatabaseService(fileStore, gitService, cache, orgService),
	}
}

// ListDatabases returns a list of all databases
func (h *DatabaseHandler) ListDatabases(ctx context.Context, req models.ListDatabasesRequest) (*models.ListDatabasesResponse, error) {
	databases, err := h.databaseService.ListDatabases(ctx)
	if err != nil {
		return nil, errors.InternalWithError("Failed to list databases", err)
	}

	dbList := make([]any, len(databases))
	for i, db := range databases {
		dbList[i] = map[string]any{
			"id":       db.ID,
			"title":    db.Title,
			"created":  db.Created,
			"modified": db.Modified,
		}
	}

	return &models.ListDatabasesResponse{Databases: dbList}, nil
}

// GetDatabase returns a specific database by ID
func (h *DatabaseHandler) GetDatabase(ctx context.Context, req models.GetDatabaseRequest) (*models.GetDatabaseResponse, error) {
	db, err := h.databaseService.GetDatabase(ctx, req.ID)
	if err != nil {
		return nil, errors.NotFound("database")
	}

	return &models.GetDatabaseResponse{
		ID:       db.ID,
		Title:    db.Title,
		Columns:  db.Columns,
		Created:  db.Created.Format("2006-01-02T15:04:05Z07:00"),
		Modified: db.Modified.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// CreateDatabase creates a new database.
func (h *DatabaseHandler) CreateDatabase(ctx context.Context, req models.CreateDatabaseRequest) (*models.CreateDatabaseResponse, error) {
	if req.Title == "" {
		return nil, errors.MissingField("title")
	}

	db, err := h.databaseService.CreateDatabase(ctx, req.Title, req.Columns)
	if err != nil {
		return nil, errors.InternalWithError("Failed to create database", err)
	}

	return &models.CreateDatabaseResponse{ID: db.ID}, nil
}

// UpdateDatabase updates a database schema.
func (h *DatabaseHandler) UpdateDatabase(ctx context.Context, req models.UpdateDatabaseRequest) (*models.UpdateDatabaseResponse, error) {
	db, err := h.databaseService.UpdateDatabase(ctx, req.ID, req.Title, req.Columns)
	if err != nil {
		return nil, errors.NotFound("database")
	}

	return &models.UpdateDatabaseResponse{ID: db.ID}, nil
}

// DeleteDatabase deletes a database.
func (h *DatabaseHandler) DeleteDatabase(ctx context.Context, req models.DeleteDatabaseRequest) (*models.DeleteDatabaseResponse, error) {
	err := h.databaseService.DeleteDatabase(ctx, req.ID)
	if err != nil {
		return nil, errors.NotFound("database")
	}

	return &models.DeleteDatabaseResponse{}, nil
}

// ListRecords returns all records in a database.
func (h *DatabaseHandler) ListRecords(ctx context.Context, req models.ListRecordsRequest) (*models.ListRecordsResponse, error) {
	records, err := h.databaseService.GetRecordsPage(ctx, req.ID, req.Offset, req.Limit)
	if err != nil {
		return nil, errors.InternalWithError("Failed to list records", err)
	}

	recordList := make([]map[string]any, len(records))
	for i, record := range records {
		recordList[i] = record.Data
		recordList[i]["_id"] = record.ID
	}

	return &models.ListRecordsResponse{Records: recordList}, nil
}

// CreateRecord creates a new record in a database.
func (h *DatabaseHandler) CreateRecord(ctx context.Context, req models.CreateRecordRequest) (*models.CreateRecordResponse, error) {
	record, err := h.databaseService.CreateRecord(ctx, req.ID, req.Data)
	if err != nil {
		return nil, errors.InternalWithError("Failed to create record", err)
	}

	return &models.CreateRecordResponse{ID: record.ID}, nil
}

// UpdateRecord updates an existing record in a database.
func (h *DatabaseHandler) UpdateRecord(ctx context.Context, req models.UpdateRecordRequest) (*models.UpdateRecordResponse, error) {
	record, err := h.databaseService.UpdateRecord(ctx, req.ID, req.RID, req.Data)
	if err != nil {
		return nil, errors.NotFound("record")
	}

	return &models.UpdateRecordResponse{ID: record.ID}, nil
}

// GetRecord retrieves a single record from a database.
func (h *DatabaseHandler) GetRecord(ctx context.Context, req models.GetRecordRequest) (*models.GetRecordResponse, error) {
	record, err := h.databaseService.GetRecord(ctx, req.ID, req.RID)
	if err != nil {
		return nil, errors.NotFound("record")
	}

	return &models.GetRecordResponse{
		ID:       record.ID,
		Data:     record.Data,
		Created:  record.Created.Format("2006-01-02T15:04:05Z07:00"),
		Modified: record.Modified.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// DeleteRecord deletes a record from a database.
func (h *DatabaseHandler) DeleteRecord(ctx context.Context, req models.DeleteRecordRequest) (*models.DeleteRecordResponse, error) {
	err := h.databaseService.DeleteRecord(ctx, req.ID, req.RID)
	if err != nil {
		return nil, errors.NotFound("record")
	}

	return &models.DeleteRecordResponse{}, nil
}
