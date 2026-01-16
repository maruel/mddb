package storage

import (
	"testing"

	"github.com/maruel/mddb/internal/models"
)

func TestDatabaseService_Create(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache)

	columns := []models.Column{
		{Name: "title", Type: "text"},
		{Name: "status", Type: "select", Options: []string{"todo", "done"}},
	}

	db, err := service.CreateDatabase(t.Context(), "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	if db.ID == "" {
		t.Error("Database ID should not be empty")
	}
	if db.Title != "Test DB" {
		t.Errorf("Title mismatch: got %q, want %q", db.Title, "Test DB")
	}
	if len(db.Columns) != 2 {
		t.Errorf("Column count mismatch: got %d, want 2", len(db.Columns))
	}

	// Verify each column has an ID
	for i, col := range db.Columns {
		if col.ID == "" {
			t.Errorf("Column[%d] should have an ID", i)
		}
	}
}

func TestDatabaseService_CreateValidation(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache)

	tests := []struct {
		name    string
		title   string
		columns []models.Column
		wantErr bool
	}{
		{
			name:    "empty title",
			title:   "",
			columns: []models.Column{{Name: "col", Type: "text"}},
			wantErr: true,
		},
		{
			name:    "no columns",
			title:   "Test",
			columns: []models.Column{},
			wantErr: true,
		},
		{
			name:    "valid",
			title:   "Test",
			columns: []models.Column{{Name: "col", Type: "text"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.CreateDatabase(t.Context(), tt.title, tt.columns)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateDatabase() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDatabaseService_Get(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache)

	// Create a database
	columns := []models.Column{
		{Name: "name", Type: "text"},
	}
	created, err := service.CreateDatabase(t.Context(), "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Retrieve it
	got, err := service.GetDatabase(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Failed to get database: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}
	if got.Title != created.Title {
		t.Errorf("Title mismatch: got %q, want %q", got.Title, created.Title)
	}
}

func TestDatabaseService_List(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache)

	// Create multiple databases
	titles := []string{"DB 1", "DB 2", "DB 3"}
	createdIDs := []string{}

	for _, title := range titles {
		columns := []models.Column{{Name: "col", Type: "text"}}
		db, err := service.CreateDatabase(t.Context(), title, columns)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		createdIDs = append(createdIDs, db.ID)
	}

	// List databases
	databases, err := service.ListDatabases(t.Context())
	if err != nil {
		t.Fatalf("Failed to list databases: %v", err)
	}

	if len(databases) != len(titles) {
		t.Errorf("Database count mismatch: got %d, want %d", len(databases), len(titles))
	}

	for _, db := range databases {
		found := false
		for _, id := range createdIDs {
			if db.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected database in list: %s", db.ID)
		}
	}
}

func TestDatabaseService_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache)

	// Create a database
	columns := []models.Column{{Name: "col", Type: "text"}}
	created, err := service.CreateDatabase(t.Context(), "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Delete it
	err = service.DeleteDatabase(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Failed to delete database: %v", err)
	}

	// Verify it's gone
	_, err = service.GetDatabase(t.Context(), created.ID)
	if err == nil {
		t.Error("Database should not exist after deletion")
	}
}

func TestDatabaseService_CreateRecord(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache)

	// Create a database
	columns := []models.Column{
		{Name: "title", Type: "text"},
		{Name: "status", Type: "select"},
	}
	db, err := service.CreateDatabase(t.Context(), "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create a record
	data := map[string]interface{}{
		"title":  "My Task",
		"status": "todo",
	}
	record, err := service.CreateRecord(t.Context(), db.ID, data)
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	if record.ID == "" {
		t.Error("Record ID should not be empty")
	}
	if record.Data["title"] != "My Task" {
		t.Errorf("Record data mismatch: got %q, want %q", record.Data["title"], "My Task")
	}
}

func TestDatabaseService_GetRecords(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache)

	// Create a database
	columns := []models.Column{{Name: "name", Type: "text"}}
	db, err := service.CreateDatabase(t.Context(), "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create multiple records
	recordCount := 3
	for i := 0; i < recordCount; i++ {
		data := map[string]interface{}{"name": "Record " + string(rune(i))}
		_, err := service.CreateRecord(t.Context(), db.ID, data)
		if err != nil {
			t.Fatalf("Failed to create record: %v", err)
		}
	}

	// Get all records
	records, err := service.GetRecords(t.Context(), db.ID)
	if err != nil {
		t.Fatalf("Failed to get records: %v", err)
	}

	if len(records) != recordCount {
		t.Errorf("Record count mismatch: got %d, want %d", len(records), recordCount)
	}
}

func TestDatabaseService_GetRecord(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache)

	// Create a database
	columns := []models.Column{{Name: "name", Type: "text"}}
	db, err := service.CreateDatabase(t.Context(), "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create a record
	data := map[string]interface{}{"name": "Test Record"}
	created, err := service.CreateRecord(t.Context(), db.ID, data)
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	// Retrieve it
	got, err := service.GetRecord(t.Context(), db.ID, created.ID)
	if err != nil {
		t.Fatalf("Failed to get record: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}
	if got.Data["name"] != "Test Record" {
		t.Errorf("Data mismatch: got %q, want %q", got.Data["name"], "Test Record")
	}
}
