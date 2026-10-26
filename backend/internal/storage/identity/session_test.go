// Tests concurrent session quotas and propagation of partially created sessions.

package identity

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestSessionService(t *testing.T) {
	t.Run("CreateWithID/duplicate does not return another session", func(t *testing.T) {
		s, err := NewSessionService(filepath.Join(t.TempDir(), "sessions.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		id := ksid.NewID()
		if _, err := s.CreateWithID(id, ksid.NewID(), "hash", "Browser", "", "", storage.ToTime(time.Now().Add(time.Hour)), 0); err != nil {
			t.Fatal(err)
		}
		if created, err := s.CreateWithID(id, ksid.NewID(), "other-hash", "Browser", "", "", storage.ToTime(time.Now().Add(time.Hour)), 0); created != nil || err == nil {
			t.Fatalf("duplicate handle=%v error=%v", created, err)
		}
	})
	t.Run("CreateWithID/close failure preserves handle", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("requires Linux descriptor inspection")
		}
		path := filepath.Join(t.TempDir(), "sessions.jsonl")
		s, err := NewSessionService(path)
		if err != nil {
			t.Fatal(err)
		}
		observer := &closeAppendFile{t: t, path: path}
		s.table.AddObserver(observer)
		id := ksid.NewID()
		created, err := s.CreateWithID(id, ksid.NewID(), "hash", "Browser", "", "", storage.ToTime(time.Now().Add(time.Hour)), 0)
		if !observer.injected || err == nil || created == nil || created.ID != id {
			t.Fatalf("partial creation handle=%v error=%v injected=%v", created, err, observer.injected)
		}
		if s.CountActive() != 1 {
			t.Fatal("regression did not install a session before Close failed")
		}
		if err := s.Revoke(created.ID); err != nil {
			t.Fatal(err)
		}
		restarted, err := NewSessionService(path)
		if err != nil {
			t.Fatal(err)
		}
		if restarted.CountActive() != 0 {
			t.Fatal("partial session cleanup was not persistent")
		}
	})
	t.Run("Create", func(t *testing.T) {
		s, err := NewSessionService(filepath.Join(t.TempDir(), "sessions.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		userID := ksid.NewID()
		var created atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 16 {
			wg.Go(func() {
				<-start
				_, err := s.Create(userID, "hash", "Browser", "", "", storage.ToTime(time.Now().Add(time.Hour)), 1)
				if err == nil {
					created.Add(1)
				} else if !errors.Is(err, ErrSessionQuotaExceeded) {
					t.Error(err)
				}
			})
		}
		close(start)
		wg.Wait()
		if n := created.Load(); n != 1 {
			t.Fatalf("created %d sessions for quota 1", n)
		}
		if n := s.CountActive(); n != 1 {
			t.Fatalf("active sessions = %d", n)
		}
	})
}

// closeAppendFile uses the existing observer boundary to reproduce an OS Close
// error after a session row and its indexes have been installed. No storage seam
// or production filesystem option is introduced for fault injection.
type closeAppendFile struct {
	t        *testing.T
	path     string
	injected bool
}

func (o *closeAppendFile) OnAppend(_ *Session) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		o.t.Fatal(err)
	}
	for _, entry := range entries {
		target, err := os.Readlink("/proc/self/fd/" + entry.Name())
		if err != nil || target != o.path {
			continue
		}
		fd, err := strconv.ParseUint(entry.Name(), 10, 64)
		if err != nil {
			o.t.Fatal(err)
		}
		// Close the descriptor immediately before Append's owning os.File.Close.
		f := os.NewFile(uintptr(fd), o.path)
		if err := f.Close(); err != nil {
			o.t.Fatal(err)
		}
		o.injected = true
		return
	}
	o.t.Fatal("append descriptor not found")
}
func (o *closeAppendFile) OnUpdate(_, _ *Session) {}
func (o *closeAppendFile) OnDelete(_ *Session)    {}
