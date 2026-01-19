package storage

import (
	"os"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

func TestDatabase_ReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	orgID := testID(100)
	tests := []struct {
		name     string
		database *models.Database
	}{
		{
			name: "simple database",
			database: &models.Database{
				ID:      testID(1),
				Title:   "Test Database",
				Version: "1.0",
				Columns: []models.Column{
					{ID: testID(101), Name: "title", Type: "text"},
					{ID: testID(102), Name: "status", Type: "select", Options: []string{"todo", "done"}},
				},
				Created:  time.Now(),
				Modified: time.Now(),
			},
		},
		{
			name: "database with all column types",
			database: &models.Database{
				ID:      testID(2),
				Title:   "Complex Database",
				Version: "1.0",
				Columns: []models.Column{
					{ID: testID(201), Name: "text_field", Type: "text", Required: true},
					{ID: testID(202), Name: "number_field", Type: "number"},
					{ID: testID(203), Name: "select_field", Type: "select", Options: []string{"a", "b", "c"}},
					{ID: testID(204), Name: "multi_select", Type: "multi_select", Options: []string{"x", "y", "z"}},
					{ID: testID(205), Name: "checkbox_field", Type: "checkbox"},
					{ID: testID(206), Name: "date_field", Type: "date"},
				},
				Created:  time.Now(),
				Modified: time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write database
			err := fs.WriteDatabase(orgID, tt.database)
			if err != nil {
				t.Fatalf("Failed to write database: %v", err)
			}

			// Read database
			got, err := fs.ReadDatabase(orgID, tt.database.ID)
			if err != nil {
				t.Fatalf("Failed to read database: %v", err)
			}

			// Verify basic fields
			if got.ID != tt.database.ID {
				t.Errorf("ID mismatch: got %v, want %v", got.ID, tt.database.ID)
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
					t.Errorf("Column[%d] ID mismatch: got %v, want %v", i, col.ID, expCol.ID)
				}
				if col.Name != expCol.Name {
					t.Errorf("Column[%d] Name mismatch: got %q, want %q", i, col.Name, expCol.Name)
				}
				if col.Type != expCol.Type {
					t.Errorf("Column[%d] Type mismatch: got %q, want %q", i, col.Type, expCol.Type)
				}
			}

			// Verify file exists
			filePath := fs.databaseRecordsFile(orgID, tt.database.ID)
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

	orgID := testID(100)
	db := &models.Database{
		ID:      testID(1),
		Title:   "Test",
		Version: "1.0",
		Columns: []models.Column{
			{ID: testID(101), Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	// Should not exist initially
	if fs.DatabaseExists(orgID, db.ID) {
		t.Error("Database should not exist initially")
	}

	// Write database
	if err := fs.WriteDatabase(orgID, db); err != nil {
		t.Fatalf("Failed to write database: %v", err)
	}

	// Should exist after write
	if !fs.DatabaseExists(orgID, db.ID) {
		t.Error("Database should exist after write")
	}
}

func TestDatabase_List(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	orgID := testID(100)

	// Create multiple databases
	dbIDs := []jsonldb.ID{testID(1), testID(2), testID(3)}
	for i, id := range dbIDs {
		db := &models.Database{
			ID:      id,
			Title:   "Database " + id.String(),
			Version: "1.0",
			Columns: []models.Column{
				{ID: testID(uint64(101 + i)), Name: "name", Type: "text"},
			},
			Created:  time.Now(),
			Modified: time.Now(),
		}
		if err := fs.WriteDatabase(orgID, db); err != nil {
			t.Fatalf("Failed to write database %v: %v", id, err)
		}
	}

	// List databases
	databases, err := fs.ListDatabases(orgID)
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
			t.Errorf("Unexpected database: %v", db.ID)
		}
	}
}

func TestDatabase_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	orgID := testID(100)
	db := &models.Database{
		ID:      testID(1),
		Title:   "Test",
		Version: "1.0",
		Columns: []models.Column{
			{ID: testID(101), Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	// Write database
	if err := fs.WriteDatabase(orgID, db); err != nil {
		t.Fatalf("Failed to write database: %v", err)
	}

	// Verify file exists
	recordsPath := fs.databaseRecordsFile(orgID, db.ID)
	if _, err := os.Stat(recordsPath); err != nil {
		t.Fatalf("Database records file not found: %v", err)
	}

	// Delete database
	err = fs.DeleteDatabase(orgID, db.ID)
	if err != nil {
		t.Fatalf("Failed to delete database: %v", err)
	}

	// Verify file is deleted
	if _, err := os.Stat(recordsPath); err == nil {
		t.Error("Database records file should be deleted")
	}
}

func TestRecord_AppendRead(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	orgID := testID(100)
	dbID := testID(1)

	// Create database first
	db := &models.Database{
		ID:      dbID,
		Title:   "Test",
		Version: "1.0",
		Columns: []models.Column{
			{ID: testID(101), Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}
	if err := fs.WriteDatabase(orgID, db); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Append records
	records := []*models.DataRecord{
		{
			ID:       testID(1001),
			Data:     map[string]any{"name": "Record 1"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
		{
			ID:       testID(1002),
			Data:     map[string]any{"name": "Record 2"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
		{
			ID:       testID(1003),
			Data:     map[string]any{"name": "Record 3"},
			Created:  time.Now(),
			Modified: time.Now(),
		},
	}

	for _, rec := range records {
		err := fs.AppendRecord(orgID, dbID, rec)
		if err != nil {
			t.Fatalf("Failed to append record: %v", err)
		}
	}

	// Read records
	got, err := fs.ReadRecords(orgID, dbID)
	if err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}

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
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	orgID := testID(100)
	dbID := testID(1)

	// Create database
	db := &models.Database{
		ID:      dbID,
		Title:   "Empty DB",
		Version: "1.0",
		Columns: []models.Column{
			{ID: testID(101), Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}
	if err := fs.WriteDatabase(orgID, db); err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Read records from empty database
	records, err := fs.ReadRecords(orgID, dbID)
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

	orgID := testID(100)

	// Create database with base64 encoded ID
	dbID := testID(42)
	db := &models.Database{
		ID:      dbID,
		Title:   "Database 42",
		Version: "1.0",
		Columns: []models.Column{
			{ID: testID(101), Name: "name", Type: "text"},
		},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	if err := fs.WriteDatabase(orgID, db); err != nil {
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

	// Verify file exists at correct path
	expectedPath := fs.databaseRecordsFile(orgID, dbID)
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("Database file not found at expected path: %s", expectedPath)
	}
}
