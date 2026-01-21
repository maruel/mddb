package content

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestNewNodeService(t *testing.T) {
	tempDir := t.TempDir()

	fileStore, _ := NewFileStore(tempDir)
	service := NewNodeService(fileStore, nil, nil)

	if service.FileStore != fileStore {
		t.Error("fileStore not properly assigned")
	}
}

func TestNodeService_GetNode(t *testing.T) {
	tempDir := t.TempDir()

	fileStore, _ := NewFileStore(tempDir)
	service := NewNodeService(fileStore, nil, nil)

	ctx := t.Context()
	orgID := jsonldb.ID(999)

	// Test with empty ID
	var emptyID jsonldb.ID
	_, err := service.Get(ctx, orgID, emptyID)
	if err == nil {
		t.Error("Expected error for empty node ID")
	}

	// Test with invalid ID (contains invalid character @)
	_, err = service.Get(ctx, orgID, jsonldb.ID(0))
	if err == nil {
		t.Error("Expected error for invalid node ID")
	}
}

func TestNodeService_CreateNode(t *testing.T) {
	tempDir := t.TempDir()

	ctx, orgID, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	service := NewNodeService(fileStore, nil, orgService)

	// Test creating a document node
	var emptyParentID jsonldb.ID
	node, err := service.Create(ctx, orgID, "Test Document", NodeTypeDocument, emptyParentID)
	if err != nil {
		t.Fatalf("CreateNode (document) failed: %v", err)
	}

	if node.Title != "Test Document" {
		t.Errorf("Title = %q, want %q", node.Title, "Test Document")
	}
	if node.Type != NodeTypeDocument {
		t.Errorf("Type = %v, want %v", node.Type, NodeTypeDocument)
	}
	if node.ID.IsZero() {
		t.Error("Expected non-zero node ID")
	}
	if node.Created.IsZero() {
		t.Error("Expected non-zero Created time")
	}

	// Test creating a database node
	dbNode, err := service.Create(ctx, orgID, "Test Database", NodeTypeDatabase, emptyParentID)
	if err != nil {
		t.Fatalf("CreateNode (database) failed: %v", err)
	}

	if dbNode.Type != NodeTypeDatabase {
		t.Errorf("Type = %v, want %v", dbNode.Type, NodeTypeDatabase)
	}

	// Test creating a hybrid node
	hybridNode, err := service.Create(ctx, orgID, "Test Hybrid", NodeTypeHybrid, emptyParentID)
	if err != nil {
		t.Fatalf("CreateNode (hybrid) failed: %v", err)
	}

	if hybridNode.Type != NodeTypeHybrid {
		t.Errorf("Type = %v, want %v", hybridNode.Type, NodeTypeHybrid)
	}

	// Test creating a child node with parentID
	childNode, err := service.Create(ctx, orgID, "Child Node", NodeTypeDocument, node.ID)
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
	fileStore, _ := NewFileStore(tempDir)
	service := NewNodeService(fileStore, nil, orgService)

	// List initial nodes (may include welcome page from org creation)
	initialNodes, err := service.List(ctx, orgID)
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	initialCount := len(initialNodes)

	// Create some nodes
	var emptyParentID jsonldb.ID
	_, _ = service.Create(ctx, orgID, "Node 1", NodeTypeDocument, emptyParentID)
	_, _ = service.Create(ctx, orgID, "Node 2", NodeTypeDocument, emptyParentID)

	nodes, err := service.List(ctx, orgID)
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) != initialCount+2 {
		t.Errorf("Expected %d nodes, got %d", initialCount+2, len(nodes))
	}
}
