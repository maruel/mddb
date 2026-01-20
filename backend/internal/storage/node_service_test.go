package storage

import (
	"os"
	"testing"

	"github.com/maruel/mddb/backend/internal/storage/entity"
)

func TestNewNodeService(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-node-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewNodeService(fileStore, nil, cache, nil)

	if service == nil {
		t.Fatal("NewNodeService returned nil")
	}
	if service.fileStore != fileStore {
		t.Error("fileStore not properly assigned")
	}
	if service.cache != cache {
		t.Error("cache not properly assigned")
	}
}

func TestNodeService_GetNode(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-node-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewNodeService(fileStore, nil, cache, nil)

	ctx := newTestContext("")

	// Test with empty ID
	_, err = service.GetNode(ctx, "")
	if err == nil {
		t.Error("Expected error for empty node ID")
	}

	// Test with invalid ID (contains invalid character @)
	_, err = service.GetNode(ctx, "invalid@id")
	if err == nil {
		t.Error("Expected error for invalid node ID")
	}
}

func TestNodeService_CreateNode(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-node-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewNodeService(fileStore, nil, cache, orgService)

	// Test creating a document node
	node, err := service.CreateNode(ctx, "Test Document", entity.NodeTypeDocument, "")
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
	dbNode, err := service.CreateNode(ctx, "Test Database", entity.NodeTypeDatabase, "")
	if err != nil {
		t.Fatalf("CreateNode (database) failed: %v", err)
	}

	if dbNode.Type != entity.NodeTypeDatabase {
		t.Errorf("Type = %v, want %v", dbNode.Type, entity.NodeTypeDatabase)
	}

	// Test creating a hybrid node
	hybridNode, err := service.CreateNode(ctx, "Test Hybrid", entity.NodeTypeHybrid, "")
	if err != nil {
		t.Fatalf("CreateNode (hybrid) failed: %v", err)
	}

	if hybridNode.Type != entity.NodeTypeHybrid {
		t.Errorf("Type = %v, want %v", hybridNode.Type, entity.NodeTypeHybrid)
	}

	// Test creating a child node with parentID
	childNode, err := service.CreateNode(ctx, "Child Node", entity.NodeTypeDocument, node.ID.String())
	if err != nil {
		t.Fatalf("CreateNode (child) failed: %v", err)
	}

	if childNode.ParentID != node.ID {
		t.Errorf("ParentID = %v, want %v", childNode.ParentID, node.ID)
	}
}

func TestNodeService_ListNodes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-node-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewNodeService(fileStore, nil, cache, orgService)

	// List initial nodes (may include welcome page from org creation)
	initialNodes, err := service.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	initialCount := len(initialNodes)

	// Create some nodes
	_, _ = service.CreateNode(ctx, "Node 1", entity.NodeTypeDocument, "")
	_, _ = service.CreateNode(ctx, "Node 2", entity.NodeTypeDocument, "")

	// Need to clear cache to see new nodes from file system
	cache.InvalidateNodeTree()

	nodes, err := service.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) != initialCount+2 {
		t.Errorf("Expected %d nodes, got %d", initialCount+2, len(nodes))
	}

	// Verify caching - second call should return cached results
	nodes2, _ := service.ListNodes(ctx)
	if len(nodes2) != initialCount+2 {
		t.Errorf("Expected %d nodes from cache, got %d", initialCount+2, len(nodes2))
	}
}

func TestFindNodeInTree(t *testing.T) {
	child1 := &entity.Node{ID: testID(2), Title: "Child 1"}
	child2 := &entity.Node{ID: testID(3), Title: "Child 2"}
	grandchild := &entity.Node{ID: testID(4), Title: "Grandchild"}
	child2.Children = []*entity.Node{grandchild}

	parent := &entity.Node{
		ID:       testID(1),
		Title:    "Parent",
		Children: []*entity.Node{child1, child2},
	}

	tree := []*entity.Node{parent}

	tests := []struct {
		name      string
		searchID  uint64
		wantTitle string
		wantNil   bool
	}{
		{"find root", 1, "Parent", false},
		{"find child", 2, "Child 1", false},
		{"find another child", 3, "Child 2", false},
		{"find grandchild", 4, "Grandchild", false},
		{"not found", 999, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findNodeInTree(tree, testID(tt.searchID))
			if tt.wantNil {
				if result != nil {
					t.Error("Expected nil result")
				}
			} else {
				if result == nil {
					t.Fatal("Expected non-nil result")
				}
				if result.Title != tt.wantTitle {
					t.Errorf("Title = %q, want %q", result.Title, tt.wantTitle)
				}
			}
		})
	}

	// Test with empty tree
	result := findNodeInTree([]*entity.Node{}, testID(1))
	if result != nil {
		t.Error("Expected nil result for empty tree")
	}

	// Test with nil children
	nodeNoChildren := &entity.Node{ID: testID(10), Children: nil}
	result = findNodeInTree([]*entity.Node{nodeNoChildren}, testID(999))
	if result != nil {
		t.Error("Expected nil when searching in tree with no matching nodes")
	}
}
