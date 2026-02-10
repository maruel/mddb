// Handles hierarchical node operations (documents, tables, hybrid, records).

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NodeHandler handles hierarchical node requests.
type NodeHandler struct {
	Svc *Services
	Cfg *Config
}

// GetNode retrieves a single node's metadata.
func (h *NodeHandler) GetNode(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.GetNodeRequest) (*dto.NodeResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	node, err := ws.ReadNode(req.ID)
	if err != nil {
		return nil, dto.NotFound("node")
	}

	resp := nodeToResponse(node)

	// Add signed asset URLs if node has content
	if node.Content != "" {
		it, err := ws.IterAssets(req.ID)
		if err == nil {
			resp.AssetURLs = make(map[string]string)
			for a := range it {
				resp.AssetURLs[a.Name] = h.Cfg.GenerateSignedAssetURL(wsID, req.ID, a.Name)
			}
		}
	}

	// Add backlinks (pages that link to this page)
	backlinks, err := ws.GetBacklinks(req.ID)
	if err == nil && len(backlinks) > 0 {
		resp.Backlinks = make([]dto.BacklinkInfo, len(backlinks))
		for i, bl := range backlinks {
			resp.Backlinks[i] = dto.BacklinkInfo{
				NodeID: bl.NodeID,
				Title:  bl.Title,
			}
		}
	}

	return resp, nil
}

// ListNodeChildren returns the children of a node.
// Use id=0 to list top-level nodes in the workspace.
func (h *NodeHandler) ListNodeChildren(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.ListNodeChildrenRequest) (*dto.ListNodeChildrenResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	children, err := ws.ListChildren(req.ParentID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list children", err)
	}

	responses := make([]dto.NodeResponse, 0, len(children))
	for _, n := range children {
		responses = append(responses, *nodeToResponse(n))
	}
	return &dto.ListNodeChildrenResponse{Nodes: responses}, nil
}

// GetNodeTitles returns a map of node IDs to their titles.
// The IDs are passed as a comma-separated query parameter.
func (h *NodeHandler) GetNodeTitles(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.GetNodeTitlesRequest) (*dto.GetNodeTitlesResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	titles, err := ws.GetNodeTitles(req.IDs)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get node titles", err)
	}

	return &dto.GetNodeTitlesResponse{Titles: titles}, nil
}

// MoveNode moves a node to a new parent.
func (h *NodeHandler) MoveNode(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.MoveNodeRequest) (*dto.MoveNodeResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := GitAuthor(user)
	if err := ws.MoveNode(ctx, req.ID, req.NewParentID, author); err != nil {
		return nil, dto.InternalWithError("Failed to move node", err)
	}
	return &dto.MoveNodeResponse{Ok: true}, nil
}

// DeleteNode deletes a node.
func (h *NodeHandler) DeleteNode(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.DeleteNodeRequest) (*dto.DeleteNodeResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := GitAuthor(user)
	if err := ws.DeletePage(ctx, req.ID, author); err != nil {
		return nil, dto.NotFound("node")
	}

	return &dto.DeleteNodeResponse{Ok: true}, nil
}

// ListNodeVersions returns the version history of a node.
func (h *NodeHandler) ListNodeVersions(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.ListNodeVersionsRequest) (*dto.ListNodeVersionsResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	history, err := ws.GetHistory(ctx, req.ID, req.Limit)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get node history", err)
	}
	return &dto.ListNodeVersionsResponse{History: commitsToDTO(history)}, nil
}

// GetNodeVersion returns a specific version of a node's content.
func (h *NodeHandler) GetNodeVersion(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.GetNodeVersionRequest) (*dto.GetNodeVersionResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	if req.ID.IsZero() {
		return nil, dto.NotFound("node") // No node with ID 0 exists
	}
	pageContent, err := ws.GetPageContentAtCommit(ctx, req.Hash, req.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get node version", err)
	}
	return &dto.GetNodeVersionResponse{Content: pageContent}, nil
}

// ListNodeAssets returns a list of assets associated with a node.
func (h *NodeHandler) ListNodeAssets(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.ListNodeAssetsRequest) (*dto.ListNodeAssetsResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	it, err := ws.IterAssets(req.NodeID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list assets", err)
	}
	assets := slices.Collect(it)
	return &dto.ListNodeAssetsResponse{Assets: h.assetsToSummaries(assets, wsID, req.NodeID)}, nil
}

// assetsToSummaries converts assets to DTOs with signed URLs.
func (h *NodeHandler) assetsToSummaries(assets []*content.Asset, wsID, nodeID ksid.ID) []dto.AssetSummary {
	result := make([]dto.AssetSummary, len(assets))
	for i, a := range assets {
		result[i] = dto.AssetSummary{
			ID:       a.ID,
			Name:     a.Name,
			Size:     a.Size,
			MimeType: a.MimeType,
			Created:  a.Created,
			URL:      h.Cfg.GenerateSignedAssetURL(wsID, nodeID, a.Name),
		}
	}
	return result
}

// DeleteNodeAsset deletes an asset from a node.
func (h *NodeHandler) DeleteNodeAsset(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.DeleteNodeAssetRequest) (*dto.DeleteNodeAssetResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := GitAuthor(user)
	if err := ws.DeleteAsset(ctx, req.NodeID, req.AssetName, author); err != nil {
		return nil, dto.NotFound("asset")
	}
	return &dto.DeleteNodeAssetResponse{Ok: true}, nil
}

// --- Page-specific handlers ---

// CreatePage creates a new page under a parent node.
// The parent ID is in req.ParentID; use 0 for root.
func (h *NodeHandler) CreatePage(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.CreatePageRequest) (*dto.CreatePageResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	author := GitAuthor(user)
	node, err := ws.CreatePageUnderParent(ctx, req.ParentID, req.Title, req.Content, author)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create page", err)
	}

	return &dto.CreatePageResponse{ID: node.ID}, nil
}

// GetPage retrieves a page's content.
func (h *NodeHandler) GetPage(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.GetPageRequest) (*dto.GetPageResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	node, err := ws.ReadNode(req.ID)
	if err != nil {
		return nil, dto.NotFound("page")
	}

	return &dto.GetPageResponse{
		ID:       node.ID,
		Title:    node.Title,
		Content:  node.Content,
		Created:  node.Created,
		Modified: node.Modified,
	}, nil
}

// UpdatePage updates a page's title and content.
func (h *NodeHandler) UpdatePage(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.UpdatePageRequest) (*dto.UpdatePageResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := GitAuthor(user)
	node, err := ws.UpdatePage(ctx, req.ID, req.Title, req.Content, author)
	if err != nil {
		return nil, dto.NotFound("page")
	}

	return &dto.UpdatePageResponse{ID: node.ID}, nil
}

// DeletePage removes the page content from a node.
// The node directory is kept if table content exists.
func (h *NodeHandler) DeletePage(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.DeletePageRequest) (*dto.DeletePageResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := GitAuthor(user)
	if err := ws.DeletePageFromNode(ctx, req.ID, author); err != nil {
		return nil, dto.NotFound("page")
	}

	return &dto.DeletePageResponse{Ok: true}, nil
}

// --- Table-specific handlers ---

// CreateTable creates a new table under a parent node.
// The parent ID is in req.ParentID; use 0 for root.
func (h *NodeHandler) CreateTable(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.CreateTableRequest) (*dto.CreateTableUnderParentResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	eq := ws.EffectiveQuotas()

	// Check column quota
	if eq.MaxColumnsPerTable > 0 && len(req.Properties) > eq.MaxColumnsPerTable {
		return nil, dto.QuotaExceeded("columns per table", eq.MaxColumnsPerTable)
	}

	// Check table quota for workspace
	if err := ws.CheckTableQuota(); err != nil {
		if errors.Is(err, content.ErrTableQuotaExceeded) {
			return nil, dto.QuotaExceeded("tables per workspace", eq.MaxTablesPerWorkspace)
		}
		return nil, dto.InternalWithError("Failed to check table quota", err)
	}

	author := GitAuthor(user)
	node, err := ws.CreateTableUnderParent(ctx, req.ParentID, req.Title, propertiesToEntity(req.Properties), author)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create table", err)
	}

	return &dto.CreateTableUnderParentResponse{ID: node.ID}, nil
}

// GetTable retrieves a table's schema.
func (h *NodeHandler) GetTable(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.GetTableRequest) (*dto.GetTableSchemaResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	node, err := ws.ReadNode(req.ID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	return &dto.GetTableSchemaResponse{
		ID:         node.ID,
		Title:      node.Title,
		Properties: propertiesToDTO(node.Properties),
		Created:    node.Created,
		Modified:   node.Modified,
	}, nil
}

// UpdateTable updates a table's schema.
func (h *NodeHandler) UpdateTable(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.UpdateTableRequest) (*dto.UpdateTableResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	// Check column quota
	eq := ws.EffectiveQuotas()
	if eq.MaxColumnsPerTable > 0 && len(req.Properties) > eq.MaxColumnsPerTable {
		return nil, dto.QuotaExceeded("columns per table", eq.MaxColumnsPerTable)
	}

	node, err := ws.ReadTable(req.ID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	node.Title = req.Title
	node.Properties = propertiesToEntity(req.Properties)
	node.Modified = storage.Now()

	author := GitAuthor(user)
	if err := ws.WriteTable(ctx, node, false, author); err != nil {
		return nil, dto.NotFound("table")
	}
	return &dto.UpdateTableResponse{ID: req.ID}, nil
}

// DeleteTable removes the table content from a node.
// The node directory is kept if page content exists.
func (h *NodeHandler) DeleteTable(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.DeleteTableRequest) (*dto.DeleteTableResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := GitAuthor(user)
	if err := ws.DeleteTableFromNode(ctx, req.ID, author); err != nil {
		return nil, dto.NotFound("table")
	}
	return &dto.DeleteTableResponse{Ok: true}, nil
}

// --- Record handlers (moved from databases.go) ---

// ListRecords returns all records in a table.
func (h *NodeHandler) ListRecords(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.ListRecordsRequest) (*dto.ListRecordsResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	// Fast path: No view, no filters, no sorts -> use optimized paging
	if req.ViewID.IsZero() && req.Filters == "" && req.Sorts == "" {
		records, err := ws.ReadRecordsPage(req.ID, req.Offset, req.Limit)
		if err != nil {
			return nil, dto.InternalWithError("Failed to list records", err)
		}
		recordList := make([]dto.DataRecordResponse, len(records))
		for i, record := range records {
			recordList[i] = *dataRecordToResponse(record)
		}
		return &dto.ListRecordsResponse{Records: recordList}, nil
	}

	// Slow path: Load all records, filter, sort, then page
	node, err := ws.ReadTable(req.ID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	var filters []content.Filter
	var sorts []content.Sort

	// Apply View configuration
	if !req.ViewID.IsZero() {
		found := false
		for i := range node.Views {
			if node.Views[i].ID == req.ViewID {
				filters = node.Views[i].Filters
				sorts = node.Views[i].Sorts
				found = true
				break
			}
		}
		if !found {
			return nil, dto.NotFound("view")
		}
	}

	// Apply ad-hoc overrides (DTO types -> Content types)
	if req.Filters != "" {
		var dtoFilters []dto.Filter
		if err := json.Unmarshal([]byte(req.Filters), &dtoFilters); err != nil {
			return nil, dto.InvalidField("filters", "invalid JSON: "+err.Error())
		}
		if err := validateFiltersWithSchema(dtoFilters, node.Properties); err != nil {
			return nil, err
		}
		filters = filtersToEntity(dtoFilters)
	}
	if req.Sorts != "" {
		var dtoSorts []dto.Sort
		if err := json.Unmarshal([]byte(req.Sorts), &dtoSorts); err != nil {
			return nil, dto.InvalidField("sorts", "invalid JSON: "+err.Error())
		}
		if err := validateSortsWithSchema(dtoSorts, node.Properties); err != nil {
			return nil, err
		}
		sorts = sortsToEntity(dtoSorts)
	}

	// Load all records
	it, err := ws.IterRecords(req.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to read records", err)
	}
	var records []*content.DataRecord
	for r := range it {
		records = append(records, r)
	}

	// Filter
	if len(filters) > 0 {
		records = content.FilterRecords(records, filters)
	}

	// Sort
	if len(sorts) > 0 {
		content.SortRecords(records, sorts)
	}

	// Page
	start := req.Offset
	if start > len(records) {
		start = len(records)
	}
	end := start + req.Limit
	if end > len(records) {
		end = len(records)
	}
	records = records[start:end]

	recordList := make([]dto.DataRecordResponse, len(records))
	for i, record := range records {
		recordList[i] = *dataRecordToResponse(record)
	}
	return &dto.ListRecordsResponse{Records: recordList}, nil
}

// CreateRecord creates a new record in a table.
func (h *NodeHandler) CreateRecord(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.CreateRecordRequest) (*dto.CreateRecordResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	// Read table to get columns for type coercion
	node, err := ws.ReadTable(req.ID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	// Coerce data types based on property schema
	coercedData := content.CoerceRecordData(req.Data, node.Properties)
	// Ensure Data is never nil - JSON marshaling treats nil and {} differently
	if coercedData == nil {
		coercedData = make(map[string]any)
	}

	id := ksid.NewID()
	now := storage.Now()
	record := &content.DataRecord{
		ID:       id,
		Data:     coercedData,
		Created:  now,
		Modified: now,
	}

	author := GitAuthor(user)
	if err := ws.AppendRecord(ctx, req.ID, record, author); err != nil {
		return nil, dto.InternalWithError("Failed to create record", err)
	}
	return &dto.CreateRecordResponse{ID: id}, nil
}

// UpdateRecord updates an existing record in a table.
func (h *NodeHandler) UpdateRecord(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.UpdateRecordRequest) (*dto.UpdateRecordResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	// Read table to get columns for type coercion
	node, err := ws.ReadTable(req.ID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	// Find existing record to preserve Created time
	it, err := ws.IterRecords(req.ID)
	if err != nil {
		return nil, dto.NotFound("record")
	}
	var existing *content.DataRecord
	for r := range it {
		if r.ID == req.RID {
			existing = r
			break
		}
	}
	if existing == nil {
		return nil, dto.NotFound("record")
	}

	// Coerce data types based on property schema
	coercedData := content.CoerceRecordData(req.Data, node.Properties)
	// Ensure Data is never nil - JSON marshaling treats nil and {} differently
	if coercedData == nil {
		coercedData = make(map[string]any)
	}

	record := &content.DataRecord{
		ID:       req.RID,
		Data:     coercedData,
		Created:  existing.Created,
		Modified: storage.Now(),
	}

	author := GitAuthor(user)
	if err := ws.UpdateRecord(ctx, req.ID, record, author); err != nil {
		return nil, dto.InternalWithError("Failed to update record", err)
	}
	return &dto.UpdateRecordResponse{ID: req.RID}, nil
}

// GetRecord retrieves a single record from a table.
func (h *NodeHandler) GetRecord(ctx context.Context, wsID ksid.ID, _ *identity.User, req *dto.GetRecordRequest) (*dto.GetRecordResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	it, err := ws.IterRecords(req.ID)
	if err != nil {
		return nil, dto.NotFound("record")
	}
	for record := range it {
		if record.ID == req.RID {
			return &dto.GetRecordResponse{
				ID:       record.ID,
				Data:     record.Data,
				Created:  record.Created,
				Modified: record.Modified,
			}, nil
		}
	}
	return nil, dto.NotFound("record")
}

// DeleteRecord deletes a record from a table.
func (h *NodeHandler) DeleteRecord(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.DeleteRecordRequest) (*dto.DeleteRecordResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := GitAuthor(user)
	if err := ws.DeleteRecord(ctx, req.ID, req.RID, author); err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.DeleteRecordResponse{Ok: true}, nil
}
