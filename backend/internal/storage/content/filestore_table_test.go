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

func TestTable_ReadWrite(t *testing.T) {
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
			name: "simple table",
			node: &Node{
				ID:    jsonldb.ID(1),
				Title: "Test Table",
				Type:  NodeTypeTable,
				Properties: []Property{
					{Name: "title", Type: "text"},
					{Name: "status", Type: PropertyTypeText},
				},
				Created:  time.Now(),
				Modified: time.Now(),
			},
		},
		{
			name: "table with all column types",
			node: &Node{
				ID:    jsonldb.ID(2),
				Title: "Complex Table",
				Type:  NodeTypeTable,
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
			// Write table
			err := fs.WriteTable(ctx, orgID, tt.node, true, author)
			if err != nil {
				t.Fatalf("Failed to write table: %v", err)
			}

			// Read table
			got, err := fs.ReadTable(orgID, tt.node.ID)
			if err != nil {
				t.Fatalf("Failed to read table: %v", err)
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
				t.Errorf("Table metadata file not found: %s", filePath)
			}
		})
	}
}

func TestTable_Exists(t *testing.T) {
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
		Type:  NodeTypeTable,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	// Should not exist initially
	if fs.TableExists(orgID, node.ID) {
		t.Error("Table should not exist initially")
	}

	// Write table
	if err := fs.WriteTable(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to write table: %v", err)
	}

	// Should exist after write
	if !fs.TableExists(orgID, node.ID) {
		t.Error("Table should exist after write")
	}
}

func TestTable_List(t *testing.T) {
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

	// Create multiple tables
	tableIDs := []jsonldb.ID{jsonldb.ID(1), jsonldb.ID(2), jsonldb.ID(3)}
	for _, id := range tableIDs {
		node := &Node{
			ID:    id,
			Title: "Table " + id.String(),
			Type:  NodeTypeTable,
			Properties: []Property{
				{Name: "name", Type: "text"},
			},
			Created:  time.Now(),
			Modified: time.Now(),
		}
		if err := fs.WriteTable(ctx, orgID, node, true, author); err != nil {
			t.Fatalf("Failed to write table %v: %v", id, err)
		}
	}

	// List tables
	it, err := fs.IterTables(orgID)
	if err != nil {
		t.Fatalf("Failed to list tables: %v", err)
	}
	tables := slices.Collect(it)

	if len(tables) != len(tableIDs) {
		t.Errorf("Table count mismatch: got %d, want %d", len(tables), len(tableIDs))
	}

	for _, table := range tables {
		found := false
		for _, id := range tableIDs {
			if table.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected table: %v", table.ID)
		}
	}
}

func TestTable_Delete(t *testing.T) {
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
		Type:  NodeTypeTable,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	// Write table
	if err := fs.WriteTable(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to write table: %v", err)
	}

	// Verify metadata file exists
	metadataPath := fs.databaseMetadataFile(orgID, node.ID)
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("Table metadata file not found: %v", err)
	}

	// Delete table
	err := fs.DeleteTable(ctx, orgID, node.ID, author)
	if err != nil {
		t.Fatalf("Failed to delete table: %v", err)
	}

	// Verify file is deleted
	if _, err := os.Stat(metadataPath); err == nil {
		t.Error("Table metadata file should be deleted")
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

	// Create table first
	node := &Node{
		ID:    dbID,
		Title: "Test",
		Type:  NodeTypeTable,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}
	if err := fs.WriteTable(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to create table: %v", err)
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

func TestRecord_EmptyTable(t *testing.T) {
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

	// Create table
	node := &Node{
		ID:    dbID,
		Title: "Empty Table",
		Type:  NodeTypeTable,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}
	if err := fs.WriteTable(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Read records from empty table
	recIt, err := fs.IterRecords(orgID, dbID)
	if err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}
	records := slices.Collect(recIt)

	if len(records) != 0 {
		t.Errorf("Expected 0 records, got %d", len(records))
	}
}

func TestTable_NestedPath(t *testing.T) {
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

	// Create table with base64 encoded ID
	dbID := jsonldb.ID(42)
	node := &Node{
		ID:    dbID,
		Title: "Table 42",
		Type:  NodeTypeTable,
		Properties: []Property{
			{Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	if err := fs.WriteTable(ctx, orgID, node, true, author); err != nil {
		t.Fatalf("Failed to write table: %v", err)
	}

	// Read back
	got, err := fs.ReadTable(orgID, dbID)
	if err != nil {
		t.Fatalf("Failed to read table: %v", err)
	}

	if got.ID != dbID {
		t.Errorf("ID mismatch: got %v, want %v", got.ID, dbID)
	}

	// Verify metadata file exists at correct path
	expectedPath := fs.databaseMetadataFile(orgID, dbID)
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("Table metadata file not found at expected path: %s", expectedPath)
	}
}
