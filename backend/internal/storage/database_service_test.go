package storage

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

func TestDatabaseService_Create(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	columns := []models.Column{
		{Name: "title", Type: "text"},
		{Name: "status", Type: "select", Options: []string{"todo", "done"}},
	}

	db, err := service.CreateDatabase(ctx, "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	if db.ID.IsZero() {
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
		if col.ID.IsZero() {
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
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

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
			_, err := service.CreateDatabase(ctx, tt.title, tt.columns)
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
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create a database
	columns := []models.Column{
		{Name: "name", Type: "text"},
	}
	created, err := service.CreateDatabase(ctx, "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Retrieve it
	got, err := service.GetDatabase(ctx, created.ID.String())
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
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create multiple databases
	titles := []string{"DB 1", "DB 2", "DB 3"}
	createdIDs := []jsonldb.ID{}

	for _, title := range titles {
		columns := []models.Column{{Name: "col", Type: "text"}}
		db, err := service.CreateDatabase(ctx, title, columns)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		createdIDs = append(createdIDs, db.ID)
	}

	// List databases
	databases, err := service.ListDatabases(ctx)
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
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create a database
	columns := []models.Column{{Name: "col", Type: "text"}}
	created, err := service.CreateDatabase(ctx, "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Delete it
	err = service.DeleteDatabase(ctx, created.ID.String())
	if err != nil {
		t.Fatalf("Failed to delete database: %v", err)
	}

	// Verify it's gone
	_, err = service.GetDatabase(ctx, created.ID.String())
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
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create a database
	columns := []models.Column{
		{Name: "title", Type: "text"},
		{Name: "status", Type: "select"},
	}
	db, err := service.CreateDatabase(ctx, "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create a record
	data := map[string]any{
		"title":  "My Task",
		"status": "todo",
	}
	record, err := service.CreateRecord(ctx, db.ID.String(), data)
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	if record.ID.IsZero() {
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
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create a database
	columns := []models.Column{{Name: "name", Type: "text"}}
	db, err := service.CreateDatabase(ctx, "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create multiple records
	recordCount := 3
	for i := range recordCount {
		data := map[string]any{"name": "Record " + string(rune(i))}
		_, err := service.CreateRecord(ctx, db.ID.String(), data)
		if err != nil {
			t.Fatalf("Failed to create record: %v", err)
		}
	}

	// Get all records
	records, err := service.GetRecords(ctx, db.ID.String())
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
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create a database
	columns := []models.Column{{Name: "name", Type: "text"}}
	db, err := service.CreateDatabase(ctx, "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create a record
	data := map[string]any{"name": "Test Record"}
	created, err := service.CreateRecord(ctx, db.ID.String(), data)
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	// Retrieve it
	got, err := service.GetRecord(ctx, db.ID.String(), created.ID.String())
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

func TestDatabaseService_UpdateRecord(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create a database
	columns := []models.Column{{Name: "name", Type: "text"}}
	db, err := service.CreateDatabase(ctx, "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create a record
	data := map[string]any{"name": "Original Name"}
	created, err := service.CreateRecord(ctx, db.ID.String(), data)
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	// Update it
	newData := map[string]any{"name": "Updated Name"}
	updated, err := service.UpdateRecord(ctx, db.ID.String(), created.ID.String(), newData)
	if err != nil {
		t.Fatalf("Failed to update record: %v", err)
	}

	if updated.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", updated.ID, created.ID)
	}
	if updated.Data["name"] != "Updated Name" {
		t.Errorf("Data mismatch: got %q, want %q", updated.Data["name"], "Updated Name")
	}

	// Verify persistence
	got, err := service.GetRecord(ctx, db.ID.String(), created.ID.String())
	if err != nil {
		t.Fatalf("Failed to get record: %v", err)
	}
	if got.Data["name"] != "Updated Name" {
		t.Errorf("Data mismatch in storage: got %q, want %q", got.Data["name"], "Updated Name")
	}
}

func TestDatabaseService_DeleteRecord(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create a database
	columns := []models.Column{{Name: "name", Type: "text"}}
	db, err := service.CreateDatabase(ctx, "Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create a record
	data := map[string]any{"name": "To be deleted"}
	created, err := service.CreateRecord(ctx, db.ID.String(), data)
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	// Delete it
	err = service.DeleteRecord(ctx, db.ID.String(), created.ID.String())
	if err != nil {
		t.Fatalf("Failed to delete record: %v", err)
	}

	// Verify it's gone
	_, err = service.GetRecord(ctx, db.ID.String(), created.ID.String())
	if err == nil {
		t.Error("Record should not exist after deletion")
	}
}
