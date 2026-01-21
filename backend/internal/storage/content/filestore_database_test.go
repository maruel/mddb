package content

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestDatabase_ReadWrite(t *testing.T) {
	fs := testFileStore(t)
	ctx := context.Background()
	author := Author{Name: "Test", Email: "test@test.com"}

	orgID := jsonldb.ID(100)

	// Create org directory and initialize git repo
	if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("failed to create org dir: %v", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		t.Fatalf("failed to init org git repo: %v", err)
	}

	tests := []struct {
		name string
		node *Node
	}{
		{
			name: "simple database",
			node: &Node{
				ID:    jsonldb.ID(1),
				Title: "Test Database",
				Type:  NodeTypeDatabase,
				Properties: []Property{
					{Name: "title", Type: "text"},
					{Name: "status", Type: PropertyTypeText},
				},
				Created:  time.Now(),
				Modified: time.Now(),
			},
		},
		{
			name: "database with all column types",
			node: &Node{
				ID:    jsonldb.ID(2),
				Title: "Complex Database",
				Type:  NodeTypeDatabase,
				Properties: []Property{
					{Name: "text_field", Type: "text", Required: true},
					{Name: "number_field", Type: "number"},
					{Name: "select_field", Type: PropertyTypeText},
					{Name: "multi_select", Type: PropertyTypeText},
					{Name: "checkbox_field", Type: "checkbox"},
					{Name: "date_field", Type: "date"},
				},
				Created:  time.Now(),
				Modified: time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write database
			err := fs.WriteDatabase(ctx, orgID, tt.node, true, author)
			if err != nil {
				t.Fatalf("Failed to write database: %v", err)
			}

			// Read database
			got, err := fs.ReadDatabase(orgID, tt.node.ID)
			if err != nil {
				t.Fatalf("Failed to read database: %v", err)
			}

			// Verify basic fields
			if got.ID != tt.node.ID {
				t.Errorf("ID mismatch: got %v, want %v", got.ID, tt.node.ID)
			}
			if got.Title != tt.node.Title {
				t.Errorf("Title mismatch: got %q, want %q", got.Title, tt.node.Title)
			}
			if len(got.Properties) != len(tt.node.Properties) {
				t.Errorf("Column count mismatch: got %d, want %d", len(got.Properties), len(tt.node.Properties))
			}

			// Verify columns
			for i, col := range got.Properties {
				expCol := tt.node.Properties[i]
				if col.Name != expCol.Name {
					t.Errorf("Column[%d] Name mismatch: got %q, want %q", i, col.Name, expCol.Name)
				}
				if col.Type != expCol.Type {
					t.Errorf("Column[%d] Type mismatch: got %q, want %q", i, col.Type, expCol.Type)
				}
			}

			// Verify metadata file exists
			filePath := fs.databaseMetadataFile(orgID, tt.node.ID)
			if _, err := os.Stat(filePath); err != nil {
				t.Errorf("Database metadata file not found: %s", filePath)
			}
		})
	}
}

func TestDatabase_Exists(t *testing.T) {
	fs := testFileStore(t)
	ctx := context.Background()
	author := Author{Name: "Test", Email: "test@test.com"}

	orgID := jsonldb.ID(100)

	// Create org directory and initialize git repo
	if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("failed to create org dir: %v", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		t.Fatalf("failed to init org git repo: %v", err)
	}

	node := &Node{
		ID:    jsonldb.ID(1),
		Title: "Test",
		Type:  NodeTypeDatabase,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	// Should not exist initially
	if fs.DatabaseExists(orgID, node.ID) {
		t.Error("Database should not exist initially")
	}

	// Write database
	if err := fs.WriteDatabase(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to write database: %v", err)
	}

	// Should exist after write
	if !fs.DatabaseExists(orgID, node.ID) {
		t.Error("Database should exist after write")
	}
}

func TestDatabase_List(t *testing.T) {
	fs := testFileStore(t)
	ctx := context.Background()
	author := Author{Name: "Test", Email: "test@test.com"}

	orgID := jsonldb.ID(100)

	// Create org directory and initialize git repo
	if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("failed to create org dir: %v", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		t.Fatalf("failed to init org git repo: %v", err)
	}

	// Create multiple databases
	dbIDs := []jsonldb.ID{jsonldb.ID(1), jsonldb.ID(2), jsonldb.ID(3)}
	for _, id := range dbIDs {
		node := &Node{
			ID:    id,
			Title: "Database " + id.String(),
			Type:  NodeTypeDatabase,
			Properties: []Property{
				{Name: "name", Type: "text"},
			},
			Created:  time.Now(),
			Modified: time.Now(),
		}
		if err := fs.WriteDatabase(ctx, orgID, node, true, author); err != nil {
			t.Fatalf("Failed to write database %v: %v", id, err)
		}
	}

	// List databases
	it, err := fs.IterDatabases(orgID)
	if err != nil {
		t.Fatalf("Failed to list databases: %v", err)
	}
	databases := slices.Collect(it)

	if len(databases) != len(dbIDs) {
		t.Errorf("Database count mismatch: got %d, want %d", len(databases), len(dbIDs))
	}

	for _, db := range databases {
		found := false
		for _, id := range dbIDs {
			if db.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected database: %v", db.ID)
		}
	}
}

func TestDatabase_Delete(t *testing.T) {
	fs := testFileStore(t)
	ctx := context.Background()
	author := Author{Name: "Test", Email: "test@test.com"}

	orgID := jsonldb.ID(100)

	// Create org directory and initialize git repo
	if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("failed to create org dir: %v", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		t.Fatalf("failed to init org git repo: %v", err)
	}

	node := &Node{
		ID:    jsonldb.ID(1),
		Title: "Test",
		Type:  NodeTypeDatabase,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	// Write database
	if err := fs.WriteDatabase(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to write database: %v", err)
	}

	// Verify metadata file exists
	metadataPath := fs.databaseMetadataFile(orgID, node.ID)
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("Database metadata file not found: %v", err)
	}

	// Delete database
	err := fs.DeleteDatabase(ctx, orgID, node.ID, author)
	if err != nil {
		t.Fatalf("Failed to delete database: %v", err)
	}

	// Verify file is deleted
	if _, err := os.Stat(metadataPath); err == nil {
		t.Error("Database metadata file should be deleted")
	}
}

func TestRecord_AppendRead(t *testing.T) {
	fs := testFileStore(t)
	ctx := context.Background()
	author := Author{Name: "Test", Email: "test@test.com"}

	orgID := jsonldb.ID(100)
	dbID := jsonldb.ID(1)

	// Create org directory and initialize git repo
	if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("failed to create org dir: %v", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		t.Fatalf("failed to init org git repo: %v", err)
	}

	// Create database first
	node := &Node{
		ID:    dbID,
		Title: "Test",
		Type:  NodeTypeDatabase,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}
	if err := fs.WriteDatabase(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Append records
	records := []*DataRecord{
		{
			ID:       jsonldb.NewID(),
			Data:     map[string]any{"name": "Record 1"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
		{
			ID:       jsonldb.NewID(),
			Data:     map[string]any{"name": "Record 2"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
		{
			ID:       jsonldb.NewID(),
			Data:     map[string]any{"name": "Record 3"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
	}

	for _, rec := range records {
		err := fs.AppendRecord(ctx, orgID, dbID, rec, author)
		if err != nil {
			t.Fatalf("Failed to append record: %v", err)
		}
	}

	// Read records
	recIt, err := fs.IterRecords(orgID, dbID)
	if err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}
	got := slices.Collect(recIt)

	if len(got) != len(records) {
		t.Errorf("Record count mismatch: got %d, want %d", len(got), len(records))
	}

	for i, rec := range got {
		if rec.ID != records[i].ID {
			t.Errorf("Record[%d] ID mismatch: got %v, want %v", i, rec.ID, records[i].ID)
		}
		if name, ok := rec.Data["name"]; ok {
			if name != records[i].Data["name"] {
				t.Errorf("Record[%d] name mismatch: got %q, want %q", i, name, records[i].Data["name"])
			}
		}
	}

	// Verify JSONL file exists
	recordsPath := fs.databaseRecordsFile(orgID, dbID)
	if _, err := os.Stat(recordsPath); err != nil {
		t.Errorf("Records file not found: %s", recordsPath)
	}
}

func TestRecord_EmptyDatabase(t *testing.T) {
	fs := testFileStore(t)
	ctx := context.Background()
	author := Author{Name: "Test", Email: "test@test.com"}

	orgID := jsonldb.ID(100)
	dbID := jsonldb.ID(1)

	// Create org directory and initialize git repo
	if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("failed to create org dir: %v", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		t.Fatalf("failed to init org git repo: %v", err)
	}

	// Create database
	node := &Node{
		ID:    dbID,
		Title: "Empty DB",
		Type:  NodeTypeDatabase,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}
	if err := fs.WriteDatabase(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Read records from empty database
	recIt, err := fs.IterRecords(orgID, dbID)
	if err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}
	records := slices.Collect(recIt)

	if len(records) != 0 {
		t.Errorf("Expected 0 records, got %d", len(records))
	}
}

func TestDatabase_NestedPath(t *testing.T) {
	fs := testFileStore(t)
	ctx := context.Background()
	author := Author{Name: "Test", Email: "test@test.com"}

	orgID := jsonldb.ID(100)

	// Create org directory and initialize git repo
	if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("failed to create org dir: %v", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		t.Fatalf("failed to init org git repo: %v", err)
	}

	// Create database with base64 encoded ID
	dbID := jsonldb.ID(42)
	node := &Node{
		ID:    dbID,
		Title: "Database 42",
		Type:  NodeTypeDatabase,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	if err := fs.WriteDatabase(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to write database: %v", err)
	}

	// Read back
	got, err := fs.ReadDatabase(orgID, dbID)
	if err != nil {
		t.Fatalf("Failed to read database: %v", err)
	}

	if got.ID != dbID {
		t.Errorf("ID mismatch: got %v, want %v", got.ID, dbID)
	}

	// Verify metadata file exists at correct path
	expectedPath := fs.databaseMetadataFile(orgID, dbID)
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("Database metadata file not found at expected path: %s", expectedPath)
	}
}
