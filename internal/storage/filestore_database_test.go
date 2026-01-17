package storage

import (
	"os"
	"testing"
	"time"

	"github.com/maruel/mddb/internal/models"
)

func TestDatabase_ReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	tests := []struct {
		name     string
		database *models.Database
	}{
		{
			name: "simple database",
			database: &models.Database{
				ID:    "1",
				Title: "Test Database",
				Columns: []models.Column{
					{ID: "col_1", Name: "title", Type: "text"},
					{ID: "col_2", Name: "status", Type: "select", Options: []string{"todo", "done"}},
				},
				Created:  time.Now(),
				Modified: time.Now(),
				Path:     "metadata.json",
			},
		},
		{
			name: "database with all column types",
			database: &models.Database{
				ID:    "2",
				Title: "Complex Database",
				Columns: []models.Column{
					{ID: "col_1", Name: "text_field", Type: "text", Required: true},
					{ID: "col_2", Name: "number_field", Type: "number"},
					{ID: "col_3", Name: "select_field", Type: "select", Options: []string{"a", "b", "c"}},
					{ID: "col_4", Name: "multi_select", Type: "multi_select", Options: []string{"x", "y", "z"}},
					{ID: "col_5", Name: "checkbox_field", Type: "checkbox"},
					{ID: "col_6", Name: "date_field", Type: "date"},
				},
				Created:  time.Now(),
				Modified: time.Now(),
				Path:     "metadata.json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write database
			err := fs.WriteDatabase("", tt.database)
			if err != nil {
				t.Fatalf("Failed to write database: %v", err)
			}

			// Read database
			got, err := fs.ReadDatabase("", tt.database.ID)
			if err != nil {
				t.Fatalf("Failed to read database: %v", err)
			}

			// Verify basic fields
			if got.ID != tt.database.ID {
				t.Errorf("ID mismatch: got %q, want %q", got.ID, tt.database.ID)
			}
			if got.Title != tt.database.Title {
				t.Errorf("Title mismatch: got %q, want %q", got.Title, tt.database.Title)
			}
			if len(got.Columns) != len(tt.database.Columns) {
				t.Errorf("Column count mismatch: got %d, want %d", len(got.Columns), len(tt.database.Columns))
			}

			// Verify columns
			for i, col := range got.Columns {
				expCol := tt.database.Columns[i]
				if col.ID != expCol.ID {
					t.Errorf("Column[%d] ID mismatch: got %q, want %q", i, col.ID, expCol.ID)
				}
				if col.Name != expCol.Name {
					t.Errorf("Column[%d] Name mismatch: got %q, want %q", i, col.Name, expCol.Name)
				}
				if col.Type != expCol.Type {
					t.Errorf("Column[%d] Type mismatch: got %q, want %q", i, col.Type, expCol.Type)
				}
			}

			// Verify file exists
			filePath := fs.databaseSchemaFile("", tt.database.ID)
			if _, err := os.Stat(filePath); err != nil {
				t.Errorf("Database file not found: %s", filePath)
			}
		})
	}
}

func TestDatabase_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	db := &models.Database{
		ID:    "1",
		Title: "Test",
		Columns: []models.Column{
			{ID: "col_1", Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
		Path:     "metadata.json",
	}

	// Should not exist initially
	if fs.DatabaseExists("", db.ID) {
		t.Error("Database should not exist initially")
	}

	// Write database
	if err := fs.WriteDatabase("", db); err != nil {
		t.Fatalf("Failed to write database: %v", err)
	}

	// Should exist after write
	if !fs.DatabaseExists("", db.ID) {
		t.Error("Database should exist after write")
	}
}

func TestDatabase_List(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	// Create multiple databases
	dbIDs := []string{"1", "2", "3"}
	for _, id := range dbIDs {
		db := &models.Database{
			ID:    id,
			Title: "Database " + id,
			Columns: []models.Column{
				{ID: "col_1", Name: "name", Type: "text"},
			},
			Created:  time.Now(),
			Modified: time.Now(),
			Path:     "metadata.json",
		}
		if err := fs.WriteDatabase("", db); err != nil {
			t.Fatalf("Failed to write database %s: %v", id, err)
		}
	}

	// List databases
	databases, err := fs.ListDatabases("")
	if err != nil {
		t.Fatalf("Failed to list databases: %v", err)
	}

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
			t.Errorf("Unexpected database: %s", db.ID)
		}
	}
}

func TestDatabase_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	db := &models.Database{
		ID:    "1",
		Title: "Test",
		Columns: []models.Column{
			{ID: "col_1", Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
		Path:     "metadata.json",
	}

	// Write database
	if err := fs.WriteDatabase("", db); err != nil {
		t.Fatalf("Failed to write database: %v", err)
	}

	// Verify file exists
	schemaPath := fs.databaseSchemaFile("", db.ID)
	if _, err := os.Stat(schemaPath); err != nil {
		t.Fatalf("Database schema file not found: %v", err)
	}

	// Delete database
	err = fs.DeleteDatabase("", db.ID)
	if err != nil {
		t.Fatalf("Failed to delete database: %v", err)
	}

	// Verify file is deleted
	if _, err := os.Stat(schemaPath); err == nil {
		t.Error("Database schema file should be deleted")
	}
}

func TestRecord_AppendRead(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	dbID := "1"

	// Create database first
	db := &models.Database{
		ID:    dbID,
		Title: "Test",
		Columns: []models.Column{
			{ID: "col_1", Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
		Path:     "metadata.json",
	}
	if err := fs.WriteDatabase("", db); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Append records
	records := []*models.DataRecord{
		{
			ID:       "rec_1",
			Data:     map[string]interface{}{"name": "Record 1"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
		{
			ID:       "rec_2",
			Data:     map[string]interface{}{"name": "Record 2"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
		{
			ID:       "rec_3",
			Data:     map[string]interface{}{"name": "Record 3"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
	}

	for _, rec := range records {
		err := fs.AppendRecord("", dbID, rec)
		if err != nil {
			t.Fatalf("Failed to append record: %v", err)
		}
	}

	// Read records
	got, err := fs.ReadRecords("", dbID)
	if err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}

	if len(got) != len(records) {
		t.Errorf("Record count mismatch: got %d, want %d", len(got), len(records))
	}

	for i, rec := range got {
		if rec.ID != records[i].ID {
			t.Errorf("Record[%d] ID mismatch: got %q, want %q", i, rec.ID, records[i].ID)
		}
		if name, ok := rec.Data["name"]; ok {
			if name != records[i].Data["name"] {
				t.Errorf("Record[%d] name mismatch: got %q, want %q", i, name, records[i].Data["name"])
			}
		}
	}

	// Verify JSONL file exists
	recordsPath := fs.databaseRecordsFile("", dbID)
	if _, err := os.Stat(recordsPath); err != nil {
		t.Errorf("Records file not found: %s", recordsPath)
	}
}

func TestRecord_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	dbID := "1"

	// Create database
	db := &models.Database{
		ID:    dbID,
		Title: "Empty DB",
		Columns: []models.Column{
			{ID: "col_1", Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
		Path:     "metadata.json",
	}
	if err := fs.WriteDatabase("", db); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Read records from empty database
	records, err := fs.ReadRecords("", dbID)
	if err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}

	if len(records) != 0 {
		t.Errorf("Expected 0 records, got %d", len(records))
	}
}

func TestDatabase_NestedPath(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	// Create database with numeric ID (no nested path needed for the new model)
	dbID := "42"
	db := &models.Database{
		ID:    dbID,
		Title: "Database 42",
		Columns: []models.Column{
			{ID: "col_1", Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
		Path:     "metadata.json",
	}

	if err := fs.WriteDatabase("", db); err != nil {
		t.Fatalf("Failed to write database: %v", err)
	}

	// Read back
	got, err := fs.ReadDatabase("", dbID)
	if err != nil {
		t.Fatalf("Failed to read database: %v", err)
	}

	if got.ID != dbID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, dbID)
	}

	// Verify file exists at correct path
	expectedPath := fs.databaseSchemaFile("", dbID)
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("Database file not found at expected path: %s", expectedPath)
	}
}
