package content

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/git"
)

func BenchmarkDatabaseOperations(b *testing.B) {
	tmpDir := b.TempDir()
	ctx := context.Background()
	author := Author{Name: "Benchmark", Email: "bench@test.com"}

	gitClient, err := git.New(ctx, tmpDir, "test", "test@test.com")
	if err != nil {
		b.Fatal(err)
	}

	fs, err := NewFileStore(tmpDir, gitClient)
	if err != nil {
		b.Fatal(err)
	}
	fs.SetQuotaChecker(&noopQuotaChecker{})

	orgID := jsonldb.ID(1) // non-zero org for tests

	// Create org directory and initialize git repo
	if err := os.MkdirAll(filepath.Join(tmpDir, orgID.String()), 0o750); err != nil {
		b.Fatalf("failed to create org dir: %v", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		b.Fatalf("failed to init org git repo: %v", err)
	}

	dbID := jsonldb.NewID()
	node := &Node{
		ID:       dbID,
		Title:    "Benchmark Database",
		Type:     NodeTypeDatabase,
		Created:  time.Now(),
		Modified: time.Now(),
		Properties: []Property{
			{Name: "Title", Type: "text"},
			{Name: "Value", Type: "number"},
		},
	}

	if err := fs.WriteDatabase(ctx, orgID, node, true, author); err != nil {
		b.Fatal(err)
	}

	// Benchmark Record Creation (Append)
	b.Run("CreateRecord", func(b *testing.B) {
		for i := range b.N {
			record := &DataRecord{
				ID:       jsonldb.NewID(),
				Created:  time.Now(),
				Modified: time.Now(),
				Data: map[string]any{
					"c1": "Item",
					"c2": i,
				},
			}
			if err := fs.AppendRecord(ctx, orgID, dbID, record, author); err != nil {
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
		readNode := &Node{ID: readDBID, Title: "Read Bench", Type: NodeTypeDatabase, Created: time.Now(), Modified: time.Now()}
		if err := fs.WriteDatabase(ctx, orgID, readNode, true, author); err != nil {
			b.Fatal(err)
		}
		for range 1000 {
			record := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"c1": "test"},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord(ctx, orgID, readDBID, record, author); err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for range b.N {
			it, err := fs.IterRecords(orgID, readDBID)
			if err != nil {
				b.Fatal(err)
			}
			records := slices.Collect(it)
			if len(records) != 1000 {
				b.Errorf("expected 1000 records, got %d", len(records))
			}
		}
	})

	// Benchmark Reading Page of Records
	b.Run("ReadRecordsPage", func(b *testing.B) {
		readDBID := jsonldb.ID(100)
		readNode := &Node{ID: readDBID, Title: "Read Bench Page", Type: NodeTypeDatabase, Created: time.Now(), Modified: time.Now()}
		if err := fs.WriteDatabase(ctx, orgID, readNode, true, author); err != nil {
			b.Fatal(err)
		}
		// Write 10,000 records
		for range 10000 {
			record := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"c1": "test"},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord(ctx, orgID, readDBID, record, author); err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for range b.N {
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
