package handlers

import (
	"context"
	"slices"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// TableHandler handles table-related HTTP requests.
type TableHandler struct {
	fs *content.FileStore
}

// NewTableHandler creates a new table handler.
func NewTableHandler(fs *content.FileStore) *TableHandler {
	return &TableHandler{fs: fs}
}

// ListDatabases returns a list of all databases.
func (h *TableHandler) ListDatabases(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.ListDatabasesRequest) (*dto.ListDatabasesResponse, error) {
	it, err := h.fs.IterTables(orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list databases", err)
	}
	return &dto.ListDatabasesResponse{Databases: tablesToSummaries(slices.Collect(it))}, nil
}

// GetDatabase returns a specific database by ID.
func (h *TableHandler) GetDatabase(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.GetDatabaseRequest) (*dto.GetDatabaseResponse, error) {
	id, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	db, err := h.fs.ReadTable(orgID, id)
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
func (h *TableHandler) CreateDatabase(ctx context.Context, orgID jsonldb.ID, user *identity.User, req dto.CreateDatabaseRequest) (*dto.CreateDatabaseResponse, error) {
	if req.Title == "" {
		return nil, dto.MissingField("title")
	}

	id := jsonldb.NewID()
	now := time.Now()
	node := &content.Node{
		ID:         id,
		Title:      req.Title,
		Properties: propertiesToEntity(req.Properties),
		Created:    now,
		Modified:   now,
		Type:       content.NodeTypeTable,
	}

	author := content.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.WriteTable(ctx, orgID, node, true, author); err != nil {
		return nil, dto.InternalWithError("Failed to create database", err)
	}
	return &dto.CreateDatabaseResponse{ID: id.String()}, nil
}

// UpdateDatabase updates a database schema.
func (h *TableHandler) UpdateDatabase(ctx context.Context, orgID jsonldb.ID, user *identity.User, req dto.UpdateDatabaseRequest) (*dto.UpdateDatabaseResponse, error) {
	id, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}

	node, err := h.fs.ReadTable(orgID, id)
	if err != nil {
		return nil, dto.NotFound("database")
	}

	node.Title = req.Title
	node.Properties = propertiesToEntity(req.Properties)
	node.Modified = time.Now()

	author := content.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.WriteTable(ctx, orgID, node, false, author); err != nil {
		return nil, dto.NotFound("database")
	}
	return &dto.UpdateDatabaseResponse{ID: id.String()}, nil
}

// DeleteDatabase deletes a database.
func (h *TableHandler) DeleteDatabase(ctx context.Context, orgID jsonldb.ID, user *identity.User, req dto.DeleteDatabaseRequest) (*dto.DeleteDatabaseResponse, error) {
	id, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	author := content.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.DeleteTable(ctx, orgID, id, author); err != nil {
		return nil, dto.NotFound("database")
	}
	return &dto.DeleteDatabaseResponse{Ok: true}, nil
}

// ListRecords returns all records in a database.
func (h *TableHandler) ListRecords(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.ListRecordsRequest) (*dto.ListRecordsResponse, error) {
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	records, err := h.fs.ReadRecordsPage(orgID, dbID, req.Offset, req.Limit)
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
func (h *TableHandler) CreateRecord(ctx context.Context, orgID jsonldb.ID, user *identity.User, req dto.CreateRecordRequest) (*dto.CreateRecordResponse, error) {
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}

	// Read table to get columns for type coercion
	node, err := h.fs.ReadTable(orgID, dbID)
	if err != nil {
		return nil, dto.NotFound("database")
	}

	// Coerce data types based on property schema
	coercedData := content.CoerceRecordData(req.Data, node.Properties)

	id := jsonldb.NewID()
	now := time.Now()
	record := &content.DataRecord{
		ID:       id,
		Data:     coercedData,
		Created:  now,
		Modified: now,
	}

	author := content.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.AppendRecord(ctx, orgID, dbID, record, author); err != nil {
		return nil, dto.InternalWithError("Failed to create record", err)
	}
	return &dto.CreateRecordResponse{ID: id.String()}, nil
}

// UpdateRecord updates an existing record in a database.
func (h *TableHandler) UpdateRecord(ctx context.Context, orgID jsonldb.ID, user *identity.User, req dto.UpdateRecordRequest) (*dto.UpdateRecordResponse, error) {
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}

	// Read table to get columns for type coercion
	node, err := h.fs.ReadTable(orgID, dbID)
	if err != nil {
		return nil, dto.NotFound("database")
	}

	// Find existing record to preserve Created time
	it, err := h.fs.IterRecords(orgID, dbID)
	if err != nil {
		return nil, dto.NotFound("record")
	}
	var existing *content.DataRecord
	for r := range it {
		if r.ID == recordID {
			existing = r
			break
		}
	}
	if existing == nil {
		return nil, dto.NotFound("record")
	}

	// Coerce data types based on property schema
	coercedData := content.CoerceRecordData(req.Data, node.Properties)

	record := &content.DataRecord{
		ID:       recordID,
		Data:     coercedData,
		Created:  existing.Created,
		Modified: time.Now(),
	}

	author := content.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.UpdateRecord(ctx, orgID, dbID, record, author); err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.UpdateRecordResponse{ID: recordID.String()}, nil
}

// GetRecord retrieves a single record from a database.
func (h *TableHandler) GetRecord(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.GetRecordRequest) (*dto.GetRecordResponse, error) {
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}

	it, err := h.fs.IterRecords(orgID, dbID)
	if err != nil {
		return nil, dto.NotFound("record")
	}
	for record := range it {
		if record.ID == recordID {
			return &dto.GetRecordResponse{
				ID:       record.ID.String(),
				Data:     record.Data,
				Created:  formatTime(record.Created),
				Modified: formatTime(record.Modified),
			}, nil
		}
	}
	return nil, dto.NotFound("record")
}

// DeleteRecord deletes a record from a database.
func (h *TableHandler) DeleteRecord(ctx context.Context, orgID jsonldb.ID, user *identity.User, req dto.DeleteRecordRequest) (*dto.DeleteRecordResponse, error) {
	dbID, err := decodeID(req.ID, "database_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}
	author := content.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.DeleteRecord(ctx, orgID, dbID, recordID, author); err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.DeleteRecordResponse{Ok: true}, nil
}
