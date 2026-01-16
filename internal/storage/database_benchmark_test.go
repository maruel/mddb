package storage

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/maruel/mddb/internal/models"
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

	dbID := fs.NextID("")
	db := &models.Database{
		ID:       dbID,
		Title:    "Benchmark Database",
		Created:  time.Now(),
		Modified: time.Now(),
		Columns: []models.Column{
			{ID: "c1", Name: "Title", Type: "text"},
			{ID: "c2", Name: "Value", Type: "number"},
		},
	}

	if err := fs.WriteDatabase("", db); err != nil {
		b.Fatal(err)
	}

	// Benchmark Record Creation (Append)
	b.Run("CreateRecord", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			record := &models.Record{
				ID:       fmt.Sprintf("r%d", i),
				Created:  time.Now(),
				Modified: time.Now(),
				Data: map[string]interface{}{
					"c1": fmt.Sprintf("Item %d", i),
					"c2": i,
				},
			}
			if err := fs.AppendRecord("", dbID, record); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Benchmark Reading All Records
	// We need a fresh file store or just read from the existing one populated by previous step
	// To make this isolated, we can pre-populate a DB with N records
	b.Run("ReadRecords", func(b *testing.B) {
		// Prepare a database with 1000 records
		readDBID := fs.NextID("")
		readDB := &models.Database{ID: readDBID, Title: "Read Bench", Created: time.Now(), Modified: time.Now()}
		if err := fs.WriteDatabase("", readDB); err != nil {
			b.Fatal(err)
		}
		for i := 0; i < 1000; i++ {
			record := &models.Record{
				ID:       fmt.Sprintf("r%d", i),
				Data:     map[string]interface{}{"c1": "test"},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord("", readDBID, record); err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			records, err := fs.ReadRecords("", readDBID)
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
		readDBID := "read_bench_page"
		readDB := &models.Database{ID: readDBID, Title: "Read Bench Page", Created: time.Now(), Modified: time.Now()}
		if err := fs.WriteDatabase("", readDB); err != nil {
			b.Fatal(err)
		}
		// Write 10,000 records
		for i := 0; i < 10000; i++ {
			record := &models.Record{
				ID:       fmt.Sprintf("r%d", i),
				Data:     map[string]interface{}{"c1": "test"},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord("", readDBID, record); err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Read 50 records from middle
			records, err := fs.ReadRecordsPage("", readDBID, 5000, 50)
			if err != nil {
				b.Fatal(err)
			}
			if len(records) != 50 {
				b.Errorf("expected 50 records, got %d", len(records))
			}
		}
	})
}