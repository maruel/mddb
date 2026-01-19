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
		{Name: "status", Type: "select"},
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

func TestDatabaseService_TypeCoercion(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	cache := NewCache()
	service := NewDatabaseService(fs, nil, cache, nil)
	ctx := newTestContext(testID(100).String())

	// Create a database with various column types
	columns := []models.Column{
		{Name: "name", Type: "text"},
		{Name: "count", Type: "number"},
		{Name: "price", Type: "number"},
		{Name: "active", Type: "checkbox"},
		{Name: "category", Type: "select"},
	}
	db, err := service.CreateDatabase(ctx, "Type Test DB", columns)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create a record with types that need coercion
	// JSON decodes numbers as float64, booleans as bool
	data := map[string]any{
		"name":     123,            // number → text: "123"
		"count":    float64(42),    // float64 whole → number: int64(42)
		"price":    float64(19.99), // float64 decimal → number: float64(19.99)
		"active":   true,           // bool → checkbox: int64(1)
		"category": "electronics",  // string → select: "electronics"
	}
	record, err := service.CreateRecord(ctx, db.ID.String(), data)
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	// Verify coercion results
	if name, ok := record.Data["name"].(string); !ok || name != "123" {
		t.Errorf("name coercion: got %v (%T), want '123' (string)", record.Data["name"], record.Data["name"])
	}
	if count, ok := record.Data["count"].(int64); !ok || count != 42 {
		t.Errorf("count coercion: got %v (%T), want 42 (int64)", record.Data["count"], record.Data["count"])
	}
	if price, ok := record.Data["price"].(float64); !ok || price != 19.99 {
		t.Errorf("price coercion: got %v (%T), want 19.99 (float64)", record.Data["price"], record.Data["price"])
	}
	if active, ok := record.Data["active"].(int64); !ok || active != 1 {
		t.Errorf("active coercion: got %v (%T), want 1 (int64)", record.Data["active"], record.Data["active"])
	}
	if category, ok := record.Data["category"].(string); !ok || category != "electronics" {
		t.Errorf("category coercion: got %v (%T), want 'electronics' (string)", record.Data["category"], record.Data["category"])
	}

	// Test update coercion
	updateData := map[string]any{
		"name":     456,
		"count":    "100",   // string → number: int64(100)
		"price":    "29.99", // string → number: float64(29.99)
		"active":   false,   // bool → checkbox: int64(0)
		"category": "books",
	}
	updated, err := service.UpdateRecord(ctx, db.ID.String(), record.ID.String(), updateData)
	if err != nil {
		t.Fatalf("Failed to update record: %v", err)
	}

	if name, ok := updated.Data["name"].(string); !ok || name != "456" {
		t.Errorf("updated name coercion: got %v (%T), want '456' (string)", updated.Data["name"], updated.Data["name"])
	}
	if count, ok := updated.Data["count"].(int64); !ok || count != 100 {
		t.Errorf("updated count coercion: got %v (%T), want 100 (int64)", updated.Data["count"], updated.Data["count"])
	}
	if price, ok := updated.Data["price"].(float64); !ok || price != 29.99 {
		t.Errorf("updated price coercion: got %v (%T), want 29.99 (float64)", updated.Data["price"], updated.Data["price"])
	}
	if active, ok := updated.Data["active"].(int64); !ok || active != 0 {
		t.Errorf("updated active coercion: got %v (%T), want 0 (int64)", updated.Data["active"], updated.Data["active"])
	}
}
