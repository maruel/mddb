package storage

import (
	"os"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

func BenchmarkDatabaseOperations(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "mddb-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	fs, err := NewFileStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}

	orgID := testID(0) // zero org for tests
	dbID := jsonldb.NewID()
	db := &models.Database{
		ID:       dbID,
		Title:    "Benchmark Database",
		Version:  "1.0",
		Created:  time.Now(),
		Modified: time.Now(),
		Columns: []models.Column{
			{ID: testID(1), Name: "Title", Type: "text"},
			{ID: testID(2), Name: "Value", Type: "number"},
		},
	}

	if err := fs.WriteDatabase(orgID, db); err != nil {
		b.Fatal(err)
	}

	// Benchmark Record Creation (Append)
	b.Run("CreateRecord", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			record := &models.DataRecord{
				ID:       jsonldb.NewID(),
				Created:  time.Now(),
				Modified: time.Now(),
				Data: map[string]any{
					"c1": "Item",
					"c2": i,
				},
			}
			if err := fs.AppendRecord(orgID, dbID, record); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Benchmark Reading All Records
	// We need a fresh file store or just read from the existing one populated by previous step
	// To make this isolated, we can pre-populate a DB with N records
	b.Run("ReadRecords", func(b *testing.B) {
		// Prepare a database with 1000 records
		readDBID := jsonldb.NewID()
		readDB := &models.Database{ID: readDBID, Title: "Read Bench", Version: "1.0", Created: time.Now(), Modified: time.Now()}
		if err := fs.WriteDatabase(orgID, readDB); err != nil {
			b.Fatal(err)
		}
		for i := 0; i < 1000; i++ {
			record := &models.DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"c1": "test"},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord(orgID, readDBID, record); err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			records, err := fs.ReadRecords(orgID, readDBID)
			if err != nil {
				b.Fatal(err)
			}
			if len(records) != 1000 {
				b.Errorf("expected 1000 records, got %d", len(records))
			}
		}
	})

	// Benchmark Reading Page of Records
	b.Run("ReadRecordsPage", func(b *testing.B) {
		readDBID := testID(100)
		readDB := &models.Database{ID: readDBID, Title: "Read Bench Page", Version: "1.0", Created: time.Now(), Modified: time.Now()}
		if err := fs.WriteDatabase(orgID, readDB); err != nil {
			b.Fatal(err)
		}
		// Write 10,000 records
		for i := 0; i < 10000; i++ {
			record := &models.DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"c1": "test"},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord(orgID, readDBID, record); err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Read 50 records from middle
			records, err := fs.ReadRecordsPage(orgID, readDBID, 5000, 50)
			if err != nil {
				b.Fatal(err)
			}
			if len(records) != 50 {
				b.Errorf("expected 50 records, got %d", len(records))
			}
		}
	})
}
