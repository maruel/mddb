package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

// DatabaseHandler handles database-related HTTP requests
type DatabaseHandler struct {
	databaseService *content.DatabaseService
}

// NewDatabaseHandler creates a new database handler
func NewDatabaseHandler(fileStore *infra.FileStore, gitService *infra.GitService, cache *infra.Cache, orgService *identity.OrganizationService) *DatabaseHandler {
	return &DatabaseHandler{
		databaseService: content.NewDatabaseService(fileStore, gitService, cache, orgService),
	}
}

// ListDatabases returns a list of all databases
func (h *DatabaseHandler) ListDatabases(ctx context.Context, req dto.ListDatabasesRequest) (*dto.ListDatabasesResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	databases, err := h.databaseService.ListDatabases(ctx, orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list databases", err)
	}
	return &dto.ListDatabasesResponse{Databases: databasesToSummaries(databases)}, nil
}

// GetDatabase returns a specific database by ID
func (h *DatabaseHandler) GetDatabase(ctx context.Context, req dto.GetDatabaseRequest) (*dto.GetDatabaseResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	id, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	db, err := h.databaseService.GetDatabase(ctx, orgID, id)
	if err != nil {
		return nil, dto.NotFound("database")
	}
	return &dto.GetDatabaseResponse{
		ID:         db.ID.String(),
		Title:      db.Title,
		Properties: propertiesToDTO(db.Properties),
		Created:    formatTime(db.Created),
		Modified:   formatTime(db.Modified),
	}, nil
}

// CreateDatabase creates a new database.
func (h *DatabaseHandler) CreateDatabase(ctx context.Context, req dto.CreateDatabaseRequest) (*dto.CreateDatabaseResponse, error) {
	if req.Title == "" {
		return nil, dto.MissingField("title")
	}
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	db, err := h.databaseService.CreateDatabase(ctx, orgID, req.Title, propertiesToEntity(req.Properties))
	if err != nil {
		return nil, dto.InternalWithError("Failed to create database", err)
	}
	return &dto.CreateDatabaseResponse{ID: db.ID.String()}, nil
}

// UpdateDatabase updates a database schema.
func (h *DatabaseHandler) UpdateDatabase(ctx context.Context, req dto.UpdateDatabaseRequest) (*dto.UpdateDatabaseResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	id, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	db, err := h.databaseService.UpdateDatabase(ctx, orgID, id, req.Title, propertiesToEntity(req.Properties))
	if err != nil {
		return nil, dto.NotFound("database")
	}
	return &dto.UpdateDatabaseResponse{ID: db.ID.String()}, nil
}

// DeleteDatabase deletes a database.
func (h *DatabaseHandler) DeleteDatabase(ctx context.Context, req dto.DeleteDatabaseRequest) (*dto.DeleteDatabaseResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	id, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	if err := h.databaseService.DeleteDatabase(ctx, orgID, id); err != nil {
		return nil, dto.NotFound("database")
	}
	return &dto.DeleteDatabaseResponse{Ok: true}, nil
}

// ListRecords returns all records in a database.
func (h *DatabaseHandler) ListRecords(ctx context.Context, req dto.ListRecordsRequest) (*dto.ListRecordsResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	records, err := h.databaseService.GetRecordsPage(ctx, orgID, dbID, req.Offset, req.Limit)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list records", err)
	}
	recordList := make([]dto.DataRecordResponse, len(records))
	for i, record := range records {
		recordList[i] = *dataRecordToResponse(record)
	}
	return &dto.ListRecordsResponse{Records: recordList}, nil
}

// CreateRecord creates a new record in a database.
func (h *DatabaseHandler) CreateRecord(ctx context.Context, req dto.CreateRecordRequest) (*dto.CreateRecordResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	record, err := h.databaseService.CreateRecord(ctx, orgID, dbID, req.Data)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create record", err)
	}
	return &dto.CreateRecordResponse{ID: record.ID.String()}, nil
}

// UpdateRecord updates an existing record in a database.
func (h *DatabaseHandler) UpdateRecord(ctx context.Context, req dto.UpdateRecordRequest) (*dto.UpdateRecordResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}
	record, err := h.databaseService.UpdateRecord(ctx, orgID, dbID, recordID, req.Data)
	if err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.UpdateRecordResponse{ID: record.ID.String()}, nil
}

// GetRecord retrieves a single record from a database.
func (h *DatabaseHandler) GetRecord(ctx context.Context, req dto.GetRecordRequest) (*dto.GetRecordResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}
	record, err := h.databaseService.GetRecord(ctx, orgID, dbID, recordID)
	if err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.GetRecordResponse{
		ID:       record.ID.String(),
		Data:     record.Data,
		Created:  formatTime(record.Created),
		Modified: formatTime(record.Modified),
	}, nil
}

// DeleteRecord deletes a record from a database.
func (h *DatabaseHandler) DeleteRecord(ctx context.Context, req dto.DeleteRecordRequest) (*dto.DeleteRecordResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}
	if err := h.databaseService.DeleteRecord(ctx, orgID, dbID, recordID); err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.DeleteRecordResponse{Ok: true}, nil
}
