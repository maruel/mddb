// Tests serialized settings publication, failed updates, and complete restart snapshots.

package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

func TestServerSettings(t *testing.T) {
	t.Parallel()
	t.Run("Update", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg, err := LoadServerConfig(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		s := NewServerSettings(cfg)
		previous := s.Snapshot()
		for _, failure := range []string{"edit", "validation", "persistence"} {
			t.Run(failure, func(t *testing.T) {
				before, err := os.ReadFile(filepath.Join(dir, "settings.json")) //nolint:gosec // G304: isolated test configuration directory.
				if err != nil {
					t.Fatal(err)
				}
				destination := dir
				if failure == "persistence" {
					destination = filepath.Join(dir, "settings.json")
				}
				published := false
				err = s.Update(destination, func(next *storagev1.ServerConfig) error {
					next.Quotas.MaxPages = 27
					if failure == "edit" {
						return errors.New("rejected edit")
					}
					if failure == "validation" {
						next.RateLimits.WriteRatePerMin = -1
					}
					return nil
				}, func(*storagev1.ServerConfig) { published = true })
				if err == nil {
					t.Fatal("failed update succeeded")
				}
				if s.Snapshot() != previous || published {
					t.Fatal("failed update published settings")
				}
				after, err := os.ReadFile(filepath.Join(dir, "settings.json")) //nolint:gosec // G304: isolated test configuration directory.
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("failed update modified persisted settings")
				}
			})
		}
		if err := s.Update(dir, func(next *storagev1.ServerConfig) error { next.Quotas.MaxPages = 27; return nil }, func(next *storagev1.ServerConfig) {
			if s.Snapshot() != previous {
				t.Error("candidate published before dependent controls")
			}
			persisted, err := LoadServerConfig(dir, false)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(persisted, next) {
				t.Error("dependent control saw unpersisted candidate")
			}
		}); err != nil {
			t.Fatal(err)
		}
		if previous.Quotas.MaxPages == 27 || s.Snapshot().Quotas.MaxPages != 27 {
			t.Fatal("update changed old snapshot or failed to publish")
		}
		// An interrupted save can leave a private staging file; startup reads only
		// the complete published file, never an incomplete candidate.
		if err := os.WriteFile(filepath.Join(dir, ".settings-interrupted"), []byte(`{"quotas":`), 0o600); err != nil {
			t.Fatal(err)
		}
		restarted, err := LoadServerConfig(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restarted, s.Snapshot()) {
			t.Fatal("restart did not load the complete published snapshot")
		}
	})
	t.Run("Snapshot/concurrent publication", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg, err := LoadServerConfig(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		cfg.RateLimits.WriteRatePerMin = cfg.Quotas.MaxPages
		s := NewServerSettings(cfg)
		done := make(chan struct{})
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				for {
					select {
					case <-done:
						return
					default:
					}
					v := s.Snapshot()
					if v.RateLimits.WriteRatePerMin != v.Quotas.MaxPages {
						t.Error("reader observed a partial configuration")
						return
					}
				}
			})
		}
		for i := 1; i <= 20; i++ {
			if err := s.Update(dir, func(v *storagev1.ServerConfig) error {
				v.Quotas.MaxPages = i
				v.RateLimits.WriteRatePerMin = i
				return nil
			}, nil); err != nil {
				t.Error(err)
				break
			}
		}
		close(done)
		wg.Wait()
	})
}
