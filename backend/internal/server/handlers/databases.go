package handlers

import (
	"context"
	"slices"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
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

// ListTables returns a list of all tables.
func (h *TableHandler) ListTables(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.ListTablesRequest) (*dto.ListTablesResponse, error) {
	it, err := h.fs.IterTables(orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list tables", err)
	}
	return &dto.ListTablesResponse{Tables: tablesToSummaries(slices.Collect(it))}, nil
}

// GetTable returns a specific table by ID.
func (h *TableHandler) GetTable(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.GetTableRequest) (*dto.GetTableResponse, error) {
	id, err := decodeID(req.ID, "table_id")
	if err != nil {
		return nil, err
	}
	table, err := h.fs.ReadTable(orgID, id)
	if err != nil {
		return nil, dto.NotFound("table")
	}
	return &dto.GetTableResponse{
		ID:         table.ID.String(),
		Title:      table.Title,
		Properties: propertiesToDTO(table.Properties),
		Created:    formatTime(table.Created),
		Modified:   formatTime(table.Modified),
	}, nil
}

// CreateTable creates a new table.
func (h *TableHandler) CreateTable(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.CreateTableRequest) (*dto.CreateTableResponse, error) {
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

	author := git.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.WriteTable(ctx, orgID, node, true, author); err != nil {
		return nil, dto.InternalWithError("Failed to create table", err)
	}
	return &dto.CreateTableResponse{ID: id.String()}, nil
}

// UpdateTable updates a table schema.
func (h *TableHandler) UpdateTable(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.UpdateTableRequest) (*dto.UpdateTableResponse, error) {
	id, err := decodeID(req.ID, "table_id")
	if err != nil {
		return nil, err
	}

	node, err := h.fs.ReadTable(orgID, id)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	node.Title = req.Title
	node.Properties = propertiesToEntity(req.Properties)
	node.Modified = time.Now()

	author := git.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.WriteTable(ctx, orgID, node, false, author); err != nil {
		return nil, dto.NotFound("table")
	}
	return &dto.UpdateTableResponse{ID: id.String()}, nil
}

// DeleteTable deletes a table.
func (h *TableHandler) DeleteTable(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.DeleteTableRequest) (*dto.DeleteTableResponse, error) {
	id, err := decodeID(req.ID, "table_id")
	if err != nil {
		return nil, err
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.DeleteTable(ctx, orgID, id, author); err != nil {
		return nil, dto.NotFound("table")
	}
	return &dto.DeleteTableResponse{Ok: true}, nil
}

// ListRecords returns all records in a table.
func (h *TableHandler) ListRecords(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.ListRecordsRequest) (*dto.ListRecordsResponse, error) {
	tableID, err := decodeID(req.ID, "table_id")
	if err != nil {
		return nil, err
	}
	records, err := h.fs.ReadRecordsPage(orgID, tableID, req.Offset, req.Limit)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list records", err)
	}
	recordList := make([]dto.DataRecordResponse, len(records))
	for i, record := range records {
		recordList[i] = *dataRecordToResponse(record)
	}
	return &dto.ListRecordsResponse{Records: recordList}, nil
}

// CreateRecord creates a new record in a table.
func (h *TableHandler) CreateRecord(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.CreateRecordRequest) (*dto.CreateRecordResponse, error) {
	tableID, err := decodeID(req.ID, "table_id")
	if err != nil {
		return nil, err
	}

	// Read table to get columns for type coercion
	node, err := h.fs.ReadTable(orgID, tableID)
	if err != nil {
		return nil, dto.NotFound("table")
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

	author := git.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.AppendRecord(ctx, orgID, tableID, record, author); err != nil {
		return nil, dto.InternalWithError("Failed to create record", err)
	}
	return &dto.CreateRecordResponse{ID: id.String()}, nil
}

// UpdateRecord updates an existing record in a table.
func (h *TableHandler) UpdateRecord(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.UpdateRecordRequest) (*dto.UpdateRecordResponse, error) {
	tableID, err := decodeID(req.ID, "table_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}

	// Read table to get columns for type coercion
	node, err := h.fs.ReadTable(orgID, tableID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	// Find existing record to preserve Created time
	it, err := h.fs.IterRecords(orgID, tableID)
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

	author := git.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.UpdateRecord(ctx, orgID, tableID, record, author); err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.UpdateRecordResponse{ID: recordID.String()}, nil
}

// GetRecord retrieves a single record from a table.
func (h *TableHandler) GetRecord(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.GetRecordRequest) (*dto.GetRecordResponse, error) {
	tableID, err := decodeID(req.ID, "table_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}

	it, err := h.fs.IterRecords(orgID, tableID)
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

// DeleteRecord deletes a record from a table.
func (h *TableHandler) DeleteRecord(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.DeleteRecordRequest) (*dto.DeleteRecordResponse, error) {
	tableID, err := decodeID(req.ID, "table_id")
	if err != nil {
		return nil, err
	}
	recordID, err := decodeID(req.RID, "record_id")
	if err != nil {
		return nil, err
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.DeleteRecord(ctx, orgID, tableID, recordID, author); err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.DeleteRecordResponse{Ok: true}, nil
}
