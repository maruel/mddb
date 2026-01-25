package content

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func BenchmarkTableOperations(b *testing.B) {
	tmpDir := b.TempDir()
	ctx := b.Context()
	author := git.Author{Name: "Benchmark", Email: "bench@test.com"}

	gitMgr := git.NewManager(tmpDir, "test", "test@test.com")

	orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
	if err != nil {
		b.Fatal(err)
	}

	wsService, err := identity.NewWorkspaceService(filepath.Join(tmpDir, "workspaces.jsonl"))
	if err != nil {
		b.Fatal(err)
	}

	// Create a test organization with very high quotas (practically unlimited)
	org, err := orgService.Create(ctx, "Benchmark Organization", "bench@test.com")
	if err != nil {
		b.Fatal(err)
	}
	_, err = orgService.Modify(org.ID, func(o *identity.Organization) error {
		o.Quotas.MaxWorkspaces = 1_000
		o.Quotas.MaxMembersPerOrg = 10_000
		o.Quotas.MaxMembersPerWorkspace = 10_000
		o.Quotas.MaxTotalStorageGB = 1_000_000 // 1EB
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}

	// Create a test workspace with very high quotas (practically unlimited)
	ws, err := wsService.Create(ctx, org.ID, "Benchmark Workspace", "bench")
	if err != nil {
		b.Fatal(err)
	}
	_, err = wsService.Modify(ws.ID, func(w *identity.Workspace) error {
		w.Quotas.MaxPages = 1_000_000
		w.Quotas.MaxStorageMB = 1_000_000 // 1TB
		w.Quotas.MaxRecordsPerTable = 1_000_000
		w.Quotas.MaxAssetSizeMB = 1_000 // 1GB
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}

	fs, err := NewFileStore(tmpDir, gitMgr, wsService, orgService)
	if err != nil {
		b.Fatal(err)
	}

	wsID := ws.ID

	// Initialize git repo for workspace
	if err := fs.InitWorkspace(ctx, wsID); err != nil {
		b.Fatalf("failed to init workspace: %v", err)
	}

	dbID := jsonldb.NewID()
	node := &Node{
		ID:       dbID,
		Title:    "Benchmark Table",
		Type:     NodeTypeTable,
		Created:  storage.Now(),
		Modified: storage.Now(),
		Properties: []Property{
			{Name: "Title", Type: "text"},
			{Name: "Value", Type: "number"},
		},
	}

	if err := fs.WriteTable(ctx, wsID, node, true, author); err != nil {
		b.Fatal(err)
	}

	// Benchmark Record Creation (Append)
	b.Run("CreateRecord", func(b *testing.B) {
		for i := range b.N {
			record := &DataRecord{
				ID:       jsonldb.NewID(),
				Created:  storage.Now(),
				Modified: storage.Now(),
				Data: map[string]any{
					"c1": "Item",
					"c2": i,
				},
			}
			if err := fs.AppendRecord(ctx, wsID, dbID, record, author); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Benchmark Reading All Records
	// We need a fresh file store or just read from the existing one populated by previous step
	// To make this isolated, we can pre-populate a DB with N records
	b.Run("ReadRecords", func(b *testing.B) {
		// Prepare a table with 1000 records
		readDBID := jsonldb.NewID()
		readNode := &Node{ID: readDBID, Title: "Read Bench", Type: NodeTypeTable, Created: storage.Now(), Modified: storage.Now()}
		if err := fs.WriteTable(ctx, wsID, readNode, true, author); err != nil {
			b.Fatal(err)
		}
		for range 1000 {
			record := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"c1": "test"},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := fs.AppendRecord(ctx, wsID, readDBID, record, author); err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for range b.N {
			it, err := fs.IterRecords(wsID, readDBID)
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
		readNode := &Node{ID: readDBID, Title: "Read Bench Page", Type: NodeTypeTable, Created: storage.Now(), Modified: storage.Now()}
		if err := fs.WriteTable(ctx, wsID, readNode, true, author); err != nil {
			b.Fatal(err)
		}
		// Write 10,000 records
		for range 10000 {
			record := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"c1": "test"},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := fs.AppendRecord(ctx, wsID, readDBID, record, author); err != nil {
				b.Fatal(err)
			}
		}

		b.ResetTimer()
		for range b.N {
			// Read 50 records from middle
			records, err := fs.ReadRecordsPage(wsID, readDBID, 5000, 50)
			if err != nil {
				b.Fatal(err)
			}
			if len(records) != 50 {
				b.Errorf("expected 50 records, got %d", len(records))
			}
		}
	})
}
