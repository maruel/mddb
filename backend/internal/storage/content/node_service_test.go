package content

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

func TestNewNodeService(t *testing.T) {
	tempDir := t.TempDir()

	fileStore, _ := infra.NewFileStore(tempDir)
	service := NewNodeService(fileStore, nil, nil)

	if service.fileStore != fileStore {
		t.Error("fileStore not properly assigned")
	}
}

func TestNodeService_GetNode(t *testing.T) {
	tempDir := t.TempDir()

	fileStore, _ := infra.NewFileStore(tempDir)
	service := NewNodeService(fileStore, nil, nil)

	ctx := t.Context()
	orgID := jsonldb.ID(999)

	// Test with empty ID
	var emptyID jsonldb.ID
	_, err := service.GetNode(ctx, orgID, emptyID)
	if err == nil {
		t.Error("Expected error for empty node ID")
	}

	// Test with invalid ID (contains invalid character @)
	_, err = service.GetNode(ctx, orgID, jsonldb.ID(0))
	if err == nil {
		t.Error("Expected error for invalid node ID")
	}
}

func TestNodeService_CreateNode(t *testing.T) {
	tempDir := t.TempDir()

	ctx, orgID, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := infra.NewFileStore(tempDir)
	service := NewNodeService(fileStore, nil, orgService)

	// Test creating a document node
	var emptyParentID jsonldb.ID
	node, err := service.CreateNode(ctx, orgID, "Test Document", entity.NodeTypeDocument, emptyParentID)
	if err != nil {
		t.Fatalf("CreateNode (document) failed: %v", err)
	}

	if node.Title != "Test Document" {
		t.Errorf("Title = %q, want %q", node.Title, "Test Document")
	}
	if node.Type != entity.NodeTypeDocument {
		t.Errorf("Type = %v, want %v", node.Type, entity.NodeTypeDocument)
	}
	if node.ID.IsZero() {
		t.Error("Expected non-zero node ID")
	}
	if node.Created.IsZero() {
		t.Error("Expected non-zero Created time")
	}

	// Test creating a database node
	dbNode, err := service.CreateNode(ctx, orgID, "Test Database", entity.NodeTypeDatabase, emptyParentID)
	if err != nil {
		t.Fatalf("CreateNode (database) failed: %v", err)
	}

	if dbNode.Type != entity.NodeTypeDatabase {
		t.Errorf("Type = %v, want %v", dbNode.Type, entity.NodeTypeDatabase)
	}

	// Test creating a hybrid node
	hybridNode, err := service.CreateNode(ctx, orgID, "Test Hybrid", entity.NodeTypeHybrid, emptyParentID)
	if err != nil {
		t.Fatalf("CreateNode (hybrid) failed: %v", err)
	}

	if hybridNode.Type != entity.NodeTypeHybrid {
		t.Errorf("Type = %v, want %v", hybridNode.Type, entity.NodeTypeHybrid)
	}

	// Test creating a child node with parentID
	childNode, err := service.CreateNode(ctx, orgID, "Child Node", entity.NodeTypeDocument, node.ID)
	if err != nil {
		t.Fatalf("CreateNode (child) failed: %v", err)
	}

	if childNode.ParentID != node.ID {
		t.Errorf("ParentID = %v, want %v", childNode.ParentID, node.ID)
	}
}

func TestNodeService_ListNodes(t *testing.T) {
	tempDir := t.TempDir()

	ctx, orgID, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := infra.NewFileStore(tempDir)
	service := NewNodeService(fileStore, nil, orgService)

	// List initial nodes (may include welcome page from org creation)
	initialNodes, err := service.ListNodes(ctx, orgID)
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	initialCount := len(initialNodes)

	// Create some nodes
	var emptyParentID jsonldb.ID
	_, _ = service.CreateNode(ctx, orgID, "Node 1", entity.NodeTypeDocument, emptyParentID)
	_, _ = service.CreateNode(ctx, orgID, "Node 2", entity.NodeTypeDocument, emptyParentID)

	nodes, err := service.ListNodes(ctx, orgID)
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) != initialCount+2 {
		t.Errorf("Expected %d nodes, got %d", initialCount+2, len(nodes))
	}
}
