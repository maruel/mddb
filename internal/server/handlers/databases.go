package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/storage"
)

// DatabaseHandler handles database-related HTTP requests
type DatabaseHandler struct {
	fileStore *storage.FileStore
}

// NewDatabaseHandler creates a new database handler
func NewDatabaseHandler(fileStore *storage.FileStore) *DatabaseHandler {
	return &DatabaseHandler{fileStore: fileStore}
}

// ListDatabasesRequest is a request to list all databases.
type ListDatabasesRequest struct{}

// ListDatabasesResponse is a response containing a list of databases.
type ListDatabasesResponse struct {
	Databases []any `json:"databases"`
}

// GetDatabaseRequest is a request to get a database.
type GetDatabaseRequest struct {
	ID string `path:"id"`
}

// GetDatabaseResponse is a response containing a database.
type GetDatabaseResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Columns []any  `json:"columns"`
}

// CreateDatabaseRequest is a request to create a database.
type CreateDatabaseRequest struct {
	Title   string `json:"title"`
	Columns []any  `json:"columns"`
}

// CreateDatabaseResponse is a response from creating a database.
type CreateDatabaseResponse struct {
	ID string `json:"id"`
}

// UpdateDatabaseRequest is a request to update a database.
type UpdateDatabaseRequest struct {
	ID      string `path:"id"`
	Title   string `json:"title"`
	Columns []any  `json:"columns"`
}

// UpdateDatabaseResponse is a response from updating a database.
type UpdateDatabaseResponse struct {
	ID string `json:"id"`
}

// DeleteDatabaseRequest is a request to delete a database.
type DeleteDatabaseRequest struct {
	ID string `path:"id"`
}

// DeleteDatabaseResponse is a response from deleting a database.
type DeleteDatabaseResponse struct{}

// ListRecordsRequest is a request to list records in a database.
type ListRecordsRequest struct {
	ID string `path:"id"`
}

// ListRecordsResponse is a response containing a list of records.
type ListRecordsResponse struct {
	Records []any `json:"records"`
}

// CreateRecordRequest is a request to create a record.
type CreateRecordRequest struct {
	ID   string         `path:"id"`
	Data map[string]any `json:"data"`
}

// CreateRecordResponse is a response from creating a record.
type CreateRecordResponse struct {
	ID string `json:"id"`
}

// UpdateRecordRequest is a request to update a record.
type UpdateRecordRequest struct {
	ID   string         `path:"id"`
	RID  string         `path:"rid"`
	Data map[string]any `json:"data"`
}

// UpdateRecordResponse is a response from updating a record.
type UpdateRecordResponse struct {
	ID string `json:"id"`
}

// DeleteRecordRequest is a request to delete a record.
type DeleteRecordRequest struct {
	ID  string `path:"id"`
	RID string `path:"rid"`
}

// DeleteRecordResponse is a response from deleting a record.
type DeleteRecordResponse struct{}

// ListDatabases returns a list of all databases
func (h *DatabaseHandler) ListDatabases(ctx context.Context, req ListDatabasesRequest) (*ListDatabasesResponse, error) {
	// TODO: Implement listing databases
	return &ListDatabasesResponse{Databases: []any{}}, nil
}

// GetDatabase returns a specific database by ID
func (h *DatabaseHandler) GetDatabase(ctx context.Context, req GetDatabaseRequest) (*GetDatabaseResponse, error) {
	// TODO: Implement getting a database (req.ID is populated from path parameter)
	return nil, errors.NewAPIError(404, "Database not found")
}

// CreateDatabase creates a new database.
func (h *DatabaseHandler) CreateDatabase(ctx context.Context,
	req CreateDatabaseRequest,
) (*CreateDatabaseResponse, error) {
	// TODO: Implement creating a database
	return &CreateDatabaseResponse{ID: "placeholder"}, nil
}

// UpdateDatabase updates a database schema.
func (h *DatabaseHandler) UpdateDatabase(ctx context.Context,
	req UpdateDatabaseRequest,
) (*UpdateDatabaseResponse, error) {
	// TODO: Implement updating a database (req.ID is populated from path parameter)
	return nil, errors.NewAPIError(404, "Database not found")
}

// DeleteDatabase deletes a database.
func (h *DatabaseHandler) DeleteDatabase(ctx context.Context,
	req DeleteDatabaseRequest,
) (*DeleteDatabaseResponse, error) {
	// TODO: Implement deleting a database (req.ID is populated from path parameter)
	return &DeleteDatabaseResponse{}, nil
}

// ListRecords returns records from a database
func (h *DatabaseHandler) ListRecords(ctx context.Context, req ListRecordsRequest) (*ListRecordsResponse, error) {
	// TODO: Implement listing records (req.ID is populated from path parameter)
	return &ListRecordsResponse{Records: []any{}}, nil
}

// CreateRecord creates a new record in a database
func (h *DatabaseHandler) CreateRecord(ctx context.Context, req CreateRecordRequest) (*CreateRecordResponse, error) {
	// TODO: Implement creating a record (req.ID is populated from path parameter)
	return &CreateRecordResponse{ID: "placeholder"}, nil
}

// UpdateRecord updates an existing record
func (h *DatabaseHandler) UpdateRecord(ctx context.Context, req UpdateRecordRequest) (*UpdateRecordResponse, error) {
	// TODO: Implement updating a record (req.ID and req.RID are populated from path parameters)
	return nil, errors.NewAPIError(404, "Record not found")
}

// DeleteRecord deletes a record
func (h *DatabaseHandler) DeleteRecord(ctx context.Context, req DeleteRecordRequest) (*DeleteRecordResponse, error) {
	// TODO: Implement deleting a record (req.ID and req.RID are populated from path parameters)
	return &DeleteRecordResponse{}, nil
}
