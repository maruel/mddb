// Handles hierarchical node operations (documents, tables, hybrid).

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// NodeHandler handles hierarchical node requests.
type NodeHandler struct {
	fs *content.FileStoreService
}

// NewNodeHandler creates a new node handler.
func NewNodeHandler(fs *content.FileStoreService) *NodeHandler {
	return &NodeHandler{fs: fs}
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
func (h *NodeHandler) GetNode(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.GetNodeRequest) (*dto.NodeResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
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

// CreateNode creates a new node (page, table, or hybrid).
func (h *NodeHandler) CreateNode(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.CreateNodeRequest) (*dto.NodeResponse, error) {
	if req.Title == "" || req.Type == "" {
		return nil, dto.MissingField("title or type")
	}

	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	var nodeType content.NodeType
	switch req.Type {
	case dto.NodeTypeDocument:
		nodeType = content.NodeTypeDocument
	case dto.NodeTypeTable:
		nodeType = content.NodeTypeTable // Map table to table in storage layer
	case dto.NodeTypeHybrid:
		nodeType = content.NodeTypeHybrid
	default:
		return nil, dto.BadRequest("Invalid node type")
	}

	author := git.Author{Name: user.Name, Email: user.Email}
	node, err := ws.CreateNode(ctx, req.Title, nodeType, req.ParentID, author)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create node", err)
	}

	return nodeToResponse(node), nil
}
