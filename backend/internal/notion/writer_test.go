// Tests Notion ID mapping persistence and its version check.

package notion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/ksid"
)

func TestIDMapping(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		w := NewWriter(t.TempDir(), "ws", nil)
		if err := os.MkdirAll(w.workspacePath(), 0o750); err != nil {
			t.Fatal(err)
		}
		want := map[string]ksid.ID{"notion-page": ksid.NewID()}
		if err := w.SaveIDMapping(want); err != nil {
			t.Fatal(err)
		}
		got, err := w.LoadIDMapping()
		if err != nil || len(got) != 1 || got["notion-page"] != want["notion-page"] {
			t.Fatalf("LoadIDMapping() = %v, %v, want %v", got, err, want)
		}
	})
	t.Run("unsupported version", func(t *testing.T) {
		w := NewWriter(t.TempDir(), "ws", nil)
		for _, data := range []string{`{"ids":{}}`, `{"version":2,"ids":{}}`} {
			if err := os.MkdirAll(w.workspacePath(), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(w.workspacePath(), "notion_id_mapping.json"), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := w.LoadIDMapping(); err == nil || !strings.Contains(err.Error(), "unsupported ID mapping version") {
				t.Fatalf("LoadIDMapping(%s) error = %v", data, err)
			}
		}
	})
}
