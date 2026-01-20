package storage

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

func TestNewNodeService(t *testing.T) {
	tempDir := t.TempDir()

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
	tempDir := t.TempDir()

	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewNodeService(fileStore, nil, cache, nil)

	ctx := newTestContext(t, "")
	orgID := testID(999)

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
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewNodeService(fileStore, nil, cache, orgService)

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
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewNodeService(fileStore, nil, cache, orgService)

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

	// Need to clear cache to see new nodes from file system
	cache.InvalidateNodeTree()

	nodes, err := service.ListNodes(ctx, orgID)
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) != initialCount+2 {
		t.Errorf("Expected %d nodes, got %d", initialCount+2, len(nodes))
	}

	// Verify caching - second call should return cached results
	nodes2, _ := service.ListNodes(ctx, orgID)
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

	t.Run("find root", func(t *testing.T) {
		result := findNodeInTree(tree, testID(1))
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result.Title != "Parent" {
			t.Errorf("Title = %q, want %q", result.Title, "Parent")
		}
	})

	t.Run("find child", func(t *testing.T) {
		result := findNodeInTree(tree, testID(2))
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result.Title != "Child 1" {
			t.Errorf("Title = %q, want %q", result.Title, "Child 1")
		}
	})

	t.Run("find another child", func(t *testing.T) {
		result := findNodeInTree(tree, testID(3))
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result.Title != "Child 2" {
			t.Errorf("Title = %q, want %q", result.Title, "Child 2")
		}
	})

	t.Run("find grandchild", func(t *testing.T) {
		result := findNodeInTree(tree, testID(4))
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result.Title != "Grandchild" {
			t.Errorf("Title = %q, want %q", result.Title, "Grandchild")
		}
	})

	t.Run("not found", func(t *testing.T) {
		result := findNodeInTree(tree, testID(999))
		if result != nil {
			t.Error("Expected nil result")
		}
	})

	t.Run("empty tree", func(t *testing.T) {
		result := findNodeInTree([]*entity.Node{}, testID(1))
		if result != nil {
			t.Error("Expected nil result for empty tree")
		}
	})

	t.Run("nil children", func(t *testing.T) {
		nodeNoChildren := &entity.Node{ID: testID(10), Children: nil}
		result := findNodeInTree([]*entity.Node{nodeNoChildren}, testID(999))
		if result != nil {
			t.Error("Expected nil when searching in tree with no matching nodes")
		}
	})
}
