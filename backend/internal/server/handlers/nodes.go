// Handles hierarchical node operations (documents, tables, hybrid, records).

package handlers

import (
	"context"
	"slices"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NodeHandler handles hierarchical node requests.
type NodeHandler struct {
	fs           *content.FileStoreService
	assetHandler *AssetHandler
}

// NewNodeHandler creates a new node handler.
func NewNodeHandler(fs *content.FileStoreService, assetHandler *AssetHandler) *NodeHandler {
	return &NodeHandler{fs: fs, assetHandler: assetHandler}
}

// ListNodes returns the hierarchical node tree.
func (h *NodeHandler) ListNodes(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.ListNodesRequest) (*dto.ListNodesResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	nodes, err := ws.ReadNodeTree()
	if err != nil {
		return nil, dto.InternalWithError("Failed to read node tree", err)
	}
	responses := make([]dto.NodeResponse, 0, len(nodes))
	for _, n := range nodes {
		responses = append(responses, *nodeToResponse(n))
	}
	return &dto.ListNodesResponse{Nodes: responses}, nil
}

// GetNode retrieves a single node's metadata.
// Supports id=0 for root node.
func (h *NodeHandler) GetNode(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.GetNodeRequest) (*dto.NodeResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	// Handle root node (id=0)
	if req.ID.IsZero() {
		node, err := ws.ReadRootNode()
		if err != nil {
			return nil, dto.InternalWithError("Failed to read root node", err)
		}
		if node == nil {
			// Return empty root node if it doesn't exist
			node = &content.Node{ID: 0, Title: "Root"}
		}
		return nodeToResponse(node), nil
	}

	// Read the full tree to find the node by ID and get its actual parent
	nodes, err := ws.ReadNodeTree()
	if err != nil {
		return nil, dto.InternalWithError("Failed to read node tree", err)
	}

	node := findNodeByID(nodes, req.ID)
	if node == nil {
		return nil, dto.NotFound("node")
	}
	return nodeToResponse(node), nil
}

// ListNodeChildren returns the children of a node.
// Use id=0 to list root-level nodes.
func (h *NodeHandler) ListNodeChildren(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.ListNodeChildrenRequest) (*dto.ListNodeChildrenResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
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

// findNodeByID recursively searches for a node by ID in the tree.
func findNodeByID(nodes []*content.Node, id jsonldb.ID) *content.Node {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
		if found := findNodeByID(node.Children, id); found != nil {
			return found
		}
	}
	return nil
}

// DeleteNode deletes a node.
func (h *NodeHandler) DeleteNode(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.DeleteNodeRequest) (*dto.DeleteNodeResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.DeletePage(ctx, req.ID, author); err != nil {
		return nil, dto.NotFound("node")
	}
	return &dto.DeleteNodeResponse{Ok: true}, nil
}

// ListNodeVersions returns the version history of a node.
func (h *NodeHandler) ListNodeVersions(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.ListNodeVersionsRequest) (*dto.ListNodeVersionsResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
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
// Supports id=0 for root node.
func (h *NodeHandler) GetNodeVersion(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.GetNodeVersionRequest) (*dto.GetNodeVersionResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	// For root node (id=0), the path is just index.md
	var path string
	if req.ID.IsZero() {
		path = "index.md"
	} else {
		path = req.ID.String() + "/index.md"
	}
	contentBytes, err := ws.GetFileAtCommit(ctx, req.Hash, path)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get node version", err)
	}
	return &dto.GetNodeVersionResponse{Content: string(contentBytes)}, nil
}

// ListNodeAssets returns a list of assets associated with a node.
func (h *NodeHandler) ListNodeAssets(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.ListNodeAssetsRequest) (*dto.ListNodeAssetsResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
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
func (h *NodeHandler) assetsToSummaries(assets []*content.Asset, wsID, nodeID jsonldb.ID) []dto.AssetSummary {
	result := make([]dto.AssetSummary, len(assets))
	for i, a := range assets {
		result[i] = dto.AssetSummary{
			ID:       a.ID,
			Name:     a.Name,
			Size:     a.Size,
			MimeType: a.MimeType,
			Created:  a.Created,
			URL:      h.assetHandler.GenerateSignedAssetURL(wsID, nodeID, a.Name),
		}
	}
	return result
}

// DeleteNodeAsset deletes an asset from a node.
func (h *NodeHandler) DeleteNodeAsset(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.DeleteNodeAssetRequest) (*dto.DeleteNodeAssetResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.DeleteAsset(ctx, req.NodeID, req.AssetName, author); err != nil {
		return nil, dto.NotFound("asset")
	}
	return &dto.DeleteNodeAssetResponse{Ok: true}, nil
}

// --- Page-specific handlers ---

// CreatePage creates a new page under a parent node.
// The parent ID is in req.ParentID; use 0 for root.
func (h *NodeHandler) CreatePage(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.CreatePageRequest) (*dto.CreatePageResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	author := git.Author{Name: user.Name, Email: user.Email}
	node, err := ws.CreatePageUnderParent(ctx, req.ParentID, req.Title, req.Content, author)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create page", err)
	}

	return &dto.CreatePageResponse{ID: node.ID}, nil
}

// GetPage retrieves a page's content.
// Supports id=0 for root page.
func (h *NodeHandler) GetPage(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.GetPageRequest) (*dto.GetPageResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	var node *content.Node
	if req.ID.IsZero() {
		node, err = ws.ReadRootNode()
	} else {
		node, err = ws.ReadNode(req.ID)
	}
	if err != nil || node == nil {
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
// Supports id=0 for root page.
func (h *NodeHandler) UpdatePage(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.UpdatePageRequest) (*dto.UpdatePageResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	node, err := ws.UpdatePage(ctx, req.ID, req.Title, req.Content, author)
	if err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.UpdatePageResponse{ID: node.ID}, nil
}

// DeletePage removes the page content from a node.
// The node directory is kept if table content exists.
func (h *NodeHandler) DeletePage(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.DeletePageRequest) (*dto.DeletePageResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.DeletePageFromNode(ctx, req.ID, author); err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.DeletePageResponse{Ok: true}, nil
}

// --- Table-specific handlers ---

// CreateTable creates a new table under a parent node.
// The parent ID is in req.ParentID; use 0 for root.
func (h *NodeHandler) CreateTable(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.CreateTableRequest) (*dto.CreateTableUnderParentResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	author := git.Author{Name: user.Name, Email: user.Email}
	node, err := ws.CreateTableUnderParent(ctx, req.ParentID, req.Title, propertiesToEntity(req.Properties), author)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create table", err)
	}

	return &dto.CreateTableUnderParentResponse{ID: node.ID}, nil
}

// GetTable retrieves a table's schema.
// Supports id=0 for root table.
func (h *NodeHandler) GetTable(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.GetTableRequest) (*dto.GetTableSchemaResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	var node *content.Node
	if req.ID.IsZero() {
		node, err = ws.ReadRootNode()
	} else {
		node, err = ws.ReadTable(req.ID)
	}
	if err != nil || node == nil {
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
// Supports id=0 for root table.
func (h *NodeHandler) UpdateTable(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.UpdateTableRequest) (*dto.UpdateTableResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	node, err := ws.ReadTable(req.ID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	node.Title = req.Title
	node.Properties = propertiesToEntity(req.Properties)
	node.Modified = storage.Now()

	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.WriteTable(ctx, node, false, author); err != nil {
		return nil, dto.NotFound("table")
	}
	return &dto.UpdateTableResponse{ID: req.ID}, nil
}

// DeleteTable removes the table content from a node.
// The node directory is kept if page content exists.
func (h *NodeHandler) DeleteTable(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.DeleteTableRequest) (*dto.DeleteTableResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.DeleteTableFromNode(ctx, req.ID, author); err != nil {
		return nil, dto.NotFound("table")
	}
	return &dto.DeleteTableResponse{Ok: true}, nil
}

// --- Record handlers (moved from databases.go) ---

// ListRecords returns all records in a table.
// Supports id=0 for root table.
func (h *NodeHandler) ListRecords(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.ListRecordsRequest) (*dto.ListRecordsResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
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

// CreateRecord creates a new record in a table.
// Supports id=0 for root table.
func (h *NodeHandler) CreateRecord(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.CreateRecordRequest) (*dto.CreateRecordResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
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

	id := jsonldb.NewID()
	now := storage.Now()
	record := &content.DataRecord{
		ID:       id,
		Data:     coercedData,
		Created:  now,
		Modified: now,
	}

	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.AppendRecord(ctx, req.ID, record, author); err != nil {
		return nil, dto.InternalWithError("Failed to create record", err)
	}
	return &dto.CreateRecordResponse{ID: id}, nil
}

// UpdateRecord updates an existing record in a table.
// Supports id=0 for root table.
func (h *NodeHandler) UpdateRecord(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.UpdateRecordRequest) (*dto.UpdateRecordResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
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

	record := &content.DataRecord{
		ID:       req.RID,
		Data:     coercedData,
		Created:  existing.Created,
		Modified: storage.Now(),
	}

	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.UpdateRecord(ctx, req.ID, record, author); err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.UpdateRecordResponse{ID: req.RID}, nil
}

// GetRecord retrieves a single record from a table.
// Supports id=0 for root table.
func (h *NodeHandler) GetRecord(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.GetRecordRequest) (*dto.GetRecordResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
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
// Supports id=0 for root table.
func (h *NodeHandler) DeleteRecord(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.DeleteRecordRequest) (*dto.DeleteRecordResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.DeleteRecord(ctx, req.ID, req.RID, author); err != nil {
		return nil, dto.NotFound("record")
	}
	return &dto.DeleteRecordResponse{Ok: true}, nil
}
