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
func NewDatabaseHandler(fileStore *storage.FileStore, gitService *storage.GitService, cache *storage.Cache) *DatabaseHandler {
	return &DatabaseHandler{
		databaseService: storage.NewDatabaseService(fileStore, gitService, cache),
	}
}

// ListDatabasesRequest is a request to list all databases.
type ListDatabasesRequest struct {
	OrgID string `path:"orgID"`
}

// ListDatabasesResponse is a response containing a list of databases.
type ListDatabasesResponse struct {
	Databases []any `json:"databases"`
}

// GetDatabaseRequest is a request to get a database.
type GetDatabaseRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// GetDatabaseResponse is a response containing a database.
type GetDatabaseResponse struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Columns  []models.Column `json:"columns"`
	Created  string          `json:"created"`
	Modified string          `json:"modified"`
}

// CreateDatabaseRequest is a request to create a database.
type CreateDatabaseRequest struct {
	OrgID   string          `path:"orgID"`
	Title   string          `json:"title"`
	Columns []models.Column `json:"columns"`
}

// CreateDatabaseResponse is a response from creating a database.
type CreateDatabaseResponse struct {
	ID string `json:"id"`
}

// UpdateDatabaseRequest is a request to update a database.
type UpdateDatabaseRequest struct {
	OrgID   string          `path:"orgID"`
	ID      string          `path:"id"`
	Title   string          `json:"title"`
	Columns []models.Column `json:"columns"`
}

// UpdateDatabaseResponse is a response from updating a database.
type UpdateDatabaseResponse struct {
	ID string `json:"id"`
}

// DeleteDatabaseRequest is a request to delete a database.
type DeleteDatabaseRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// DeleteDatabaseResponse is a response from deleting a database.
type DeleteDatabaseResponse struct{}

// ListRecordsRequest is a request to list records in a database.
type ListRecordsRequest struct {
	OrgID  string `path:"orgID"`
	ID     string `path:"id"`
	Offset int    `query:"offset"`
	Limit  int    `query:"limit"`
}

// ListRecordsResponse is a response containing a list of records.
type ListRecordsResponse struct {
	Records []map[string]any `json:"records"`
}

// CreateRecordRequest is a request to create a record.
type CreateRecordRequest struct {
	OrgID string         `path:"orgID"`
	ID    string         `path:"id"`
	Data  map[string]any `json:"data"`
}

// CreateRecordResponse is a response from creating a record.
type CreateRecordResponse struct {
	ID string `json:"id"`
}

// UpdateRecordRequest is a request to update a record.
type UpdateRecordRequest struct {
	OrgID string         `path:"orgID"`
	ID    string         `path:"id"`
	RID   string         `path:"rid"`
	Data  map[string]any `json:"data"`
}

// UpdateRecordResponse is a response from updating a record.
type UpdateRecordResponse struct {
	ID string `json:"id"`
}

// GetRecordRequest is a request to get a record.
type GetRecordRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	RID   string `path:"rid"`
}

// GetRecordResponse is a response containing a record.
type GetRecordResponse struct {
	ID       string         `json:"id"`
	Data     map[string]any `json:"data"`
	Created  string         `json:"created"`
	Modified string         `json:"modified"`
}

// DeleteRecordRequest is a request to delete a record.
type DeleteRecordRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	RID   string `path:"rid"`
}

// DeleteRecordResponse is a response from deleting a record.
type DeleteRecordResponse struct{}

// ListDatabases returns a list of all databases
func (h *DatabaseHandler) ListDatabases(ctx context.Context, req ListDatabasesRequest) (*ListDatabasesResponse, error) {
	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	databases, err := h.databaseService.ListDatabases(orgID)
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

	return &ListDatabasesResponse{Databases: dbList}, nil
}

// GetDatabase returns a specific database by ID
func (h *DatabaseHandler) GetDatabase(ctx context.Context, req GetDatabaseRequest) (*GetDatabaseResponse, error) {
	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	db, err := h.databaseService.GetDatabase(orgID, req.ID)
	if err != nil {
		return nil, errors.NotFound("database")
	}

	return &GetDatabaseResponse{
		ID:       db.ID,
		Title:    db.Title,
		Columns:  db.Columns,
		Created:  db.Created.Format("2006-01-02T15:04:05Z07:00"),
		Modified: db.Modified.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// CreateDatabase creates a new database.
func (h *DatabaseHandler) CreateDatabase(ctx context.Context,
	req CreateDatabaseRequest,
) (*CreateDatabaseResponse, error) {
	if req.Title == "" {
		return nil, errors.MissingField("title")
	}

	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	db, err := h.databaseService.CreateDatabase(orgID, req.Title, req.Columns)
	if err != nil {
		return nil, errors.InternalWithError("Failed to create database", err)
	}

	return &CreateDatabaseResponse{ID: db.ID}, nil
}

// UpdateDatabase updates a database schema.
func (h *DatabaseHandler) UpdateDatabase(ctx context.Context,
	req UpdateDatabaseRequest,
) (*UpdateDatabaseResponse, error) {
	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	db, err := h.databaseService.UpdateDatabase(orgID, req.ID, req.Title, req.Columns)
	if err != nil {
		return nil, errors.NotFound("database")
	}

	return &UpdateDatabaseResponse{ID: db.ID}, nil
}

// DeleteDatabase deletes a database.
func (h *DatabaseHandler) DeleteDatabase(ctx context.Context,
	req DeleteDatabaseRequest,
) (*DeleteDatabaseResponse, error) {
	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	err := h.databaseService.DeleteDatabase(orgID, req.ID)
	if err != nil {
		return nil, errors.NotFound("database")
	}

	return &DeleteDatabaseResponse{}, nil
}

// ListRecords returns records from a database
func (h *DatabaseHandler) ListRecords(ctx context.Context, req ListRecordsRequest) (*ListRecordsResponse, error) {
	// If limit is not provided, we could either return all or set a default.
	// For performance, let's set a default large limit if not specified, or just call GetRecordsPage.
	limit := req.Limit
	if limit == 0 {
		limit = 1000 // Default limit to prevent huge responses
	}

	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	records, err := h.databaseService.GetRecordsPage(orgID, req.ID, req.Offset, limit)
	if err != nil {
		return nil, errors.NotFound("database")
	}

	recordList := make([]map[string]any, len(records))
	for i, r := range records {
		recordList[i] = map[string]any{
			"id":       r.ID,
			"data":     r.Data,
			"created":  r.Created,
			"modified": r.Modified,
		}
	}

	return &ListRecordsResponse{Records: recordList}, nil
}

// GetRecord returns a specific record
func (h *DatabaseHandler) GetRecord(ctx context.Context, req GetRecordRequest) (*GetRecordResponse, error) {
	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	record, err := h.databaseService.GetRecord(orgID, req.ID, req.RID)
	if err != nil {
		return nil, errors.NotFound("record")
	}

	return &GetRecordResponse{
		ID:       record.ID,
		Data:     record.Data,
		Created:  record.Created.Format("2006-01-02T15:04:05Z07:00"),
		Modified: record.Modified.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// CreateRecord creates a new record in a database
func (h *DatabaseHandler) CreateRecord(ctx context.Context, req CreateRecordRequest) (*CreateRecordResponse, error) {
	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	record, err := h.databaseService.CreateRecord(orgID, req.ID, req.Data)
	if err != nil {
		return nil, errors.NotFound("database")
	}

	return &CreateRecordResponse{ID: record.ID}, nil
}

// UpdateRecord updates an existing record
func (h *DatabaseHandler) UpdateRecord(ctx context.Context, req UpdateRecordRequest) (*UpdateRecordResponse, error) {
	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	return nil, errors.NotImplemented("record update")
}

// DeleteRecord deletes a record
func (h *DatabaseHandler) DeleteRecord(ctx context.Context, req DeleteRecordRequest) (*DeleteRecordResponse, error) {
	orgID := models.GetOrgID(ctx)
	if req.OrgID != orgID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}
	return nil, errors.NotImplemented("record delete")
}
