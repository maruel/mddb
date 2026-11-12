// Tests session quotas, partial creation, and race-free request lifetime cancellation.

package identity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestSessionService(t *testing.T) {
	t.Run("Context/revocation precedes persistence completion", func(t *testing.T) {
		s, err := NewSessionService(filepath.Join(t.TempDir(), "sessions.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		session, err := s.Create(ksid.NewID(), "hash", "Browser", "", "", storage.ToTime(time.Now().Add(time.Hour)), 0)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel, err := s.Context(t.Context(), session)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cancel)
		entered, release := make(chan struct{}), make(chan struct{})
		s.table.AddObserver(&gatedSessionUpdate{entered: entered, release: release})
		var wg sync.WaitGroup
		t.Cleanup(func() { close(release); wg.Wait() })
		wg.Go(func() {
			if err := s.Revoke(session.ID); err != nil {
				t.Error(err)
			}
		})
		<-entered
		select {
		case <-ctx.Done():
		default:
			t.Fatal("revocation waited for mutation completion before canceling its requests")
		}
	})
	t.Run("Context/concurrent revocation", func(t *testing.T) {
		s, err := NewSessionService(filepath.Join(t.TempDir(), "sessions.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		for range 32 {
			session, err := s.Create(ksid.NewID(), "hash", "Browser", "", "", storage.ToTime(time.Now().Add(time.Hour)), 0)
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			var ctx context.Context
			var cancel context.CancelFunc
			var bindErr error
			var wg sync.WaitGroup
			wg.Go(func() {
				<-start
				ctx, cancel, bindErr = s.Context(t.Context(), session)
			})
			wg.Go(func() {
				<-start
				if err := s.Revoke(session.ID); err != nil {
					t.Error(err)
				}
			})
			close(start)
			wg.Wait()
			if bindErr != nil && !errors.Is(bindErr, errSessionInactive) {
				t.Fatal(bindErr)
			}
			if ctx != nil {
				if ctx.Err() == nil {
					cancel()
					t.Fatal("concurrent revocation left a live request")
				}
				cancel()
			}
		}
	})
	t.Run("Context", func(t *testing.T) {
		for _, action := range []string{"cancel", "delete", "expire", "replace-credential", "revoke", "revoke-all"} {
			t.Run(action, func(t *testing.T) {
				user, id := ksid.NewID(), ksid.NewID()
				synctest.Test(t, func(t *testing.T) {
					s, err := NewSessionService(filepath.Join(t.TempDir(), "sessions.jsonl"))
					if err != nil {
						t.Fatal(err)
					}
					session, err := s.CreateWithID(id, user, "hash", "Browser", "", "", storage.ToTime(time.Now().Add(LastUsedResolution+time.Hour)), 0)
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel, err := s.Context(t.Context(), session)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(cancel)
					time.Sleep(LastUsedResolution)
					if err := s.RecordUse(session.ID); err != nil {
						t.Fatal(err)
					}
					if got, err := s.Get(session.ID); err != nil || got.LastUsed == session.LastUsed {
						t.Fatalf("RecordUse did not record: %v", err)
					}
					if ctx.Err() != nil {
						t.Fatal("activity update canceled the session")
					}
					switch action {
					case "cancel":
						cancel()
					case "delete":
						if _, err := s.table.Delete(session.ID); err != nil {
							t.Fatal(err)
						}
					case "expire":
						time.Sleep(time.Hour)
					case "revoke":
						if err := s.Revoke(session.ID); err != nil {
							t.Fatal(err)
						}
					case "revoke-all":
						if _, err := s.RevokeAllForUser(user); err != nil {
							t.Fatal(err)
						}
					case "replace-credential":
						if _, err := s.table.Modify(session.ID, func(row *Session) error { row.TokenHash = "replacement"; return nil }); err != nil {
							t.Fatal(err)
						}
					}
					synctest.Wait()
					if ctx.Err() == nil {
						t.Fatal("bound request did not stop")
					}
					s.lifetimes.mu.Lock()
					bindings := len(s.lifetimes.bindings)
					s.lifetimes.mu.Unlock()
					if bindings != 0 {
						t.Fatalf("retained %d session bindings after cancellation", bindings)
					}
					if action != "cancel" {
						if _, cancel, err := s.Context(t.Context(), session); err == nil {
							cancel()
							t.Fatal("retired session established a new lifetime")
						}
					}
				})
			})
		}
	})
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
	t.Run("Create/concurrent at quota", func(t *testing.T) {
		s, err := NewSessionService(filepath.Join(t.TempDir(), "sessions.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		userID := ksid.NewID()
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 16 {
			wg.Go(func() {
				<-start
				if _, err := s.Create(userID, "hash", "Browser", "", "", storage.ToTime(time.Now().Add(time.Hour)), 1); err != nil {
					t.Error(err)
				}
			})
		}
		close(start)
		wg.Wait()
		if n := s.CountActive(); n != 1 {
			t.Fatalf("active sessions = %d, want 1", n)
		}
	})
	t.Run("Create/revokes least recently used", func(t *testing.T) {
		// ksid needs real time: the synctest epoch precedes it.
		userID, other := ksid.NewID(), ksid.NewID()
		ids := make(chan ksid.ID, 6)
		for range cap(ids) {
			ids <- ksid.NewID()
		}
		synctest.Test(t, func(t *testing.T) {
			s, err := NewSessionService(filepath.Join(t.TempDir(), "sessions.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			expires := storage.ToTime(time.Now().Add(24 * time.Hour))
			create := func(user ksid.ID, maxSessions int) *Session {
				session, err := s.CreateWithID(<-ids, user, "hash", "Browser", "", "", expires, maxSessions)
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(time.Minute)
				return session
			}
			first, second, third := create(userID, 3), create(userID, 3), create(userID, 3)
			otherSession := create(other, 3)
			time.Sleep(LastUsedResolution)
			if err := s.RecordUse(first.ID); err != nil {
				t.Fatal(err)
			}
			// At quota 3, the fourth sign-in revokes second, which is used least
			// recently although first is older.
			fourth := create(userID, 3)
			// Lowering the quota to 2 revokes third, then first: fourth
			// was created after first's recorded use.
			fifth := create(userID, 2)
			for _, want := range []struct {
				session *Session
				active  bool
			}{{first, false}, {second, false}, {third, false}, {fourth, true}, {fifth, true}, {otherSession, true}} {
				got, err := s.Get(want.session.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got.RevokedAt.IsZero() != want.active {
					t.Errorf("session created at %v active = %t, want %t", want.session.Created.AsTime(), got.RevokedAt.IsZero(), want.active)
				}
			}
		})
	})
}

type gatedSessionUpdate struct {
	entered chan<- struct{}
	release <-chan struct{}
}

func (*gatedSessionUpdate) OnAppend(*Session) {}

func (g *gatedSessionUpdate) OnUpdate(_, _ *Session) {
	close(g.entered)
	<-g.release
}

func (*gatedSessionUpdate) OnDelete(*Session) {}

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

// BenchmarkSessionContext measures the one-time binding cost for long-lived streams.
// Ordinary MCP reads do not create this binding or its deadline/cleanup callback.
func BenchmarkSessionContext(b *testing.B) {
	s, err := NewSessionService(filepath.Join(b.TempDir(), "sessions.jsonl"))
	if err != nil {
		b.Fatal(err)
	}
	session, err := s.Create(ksid.NewID(), "hash", "Browser", "", "", storage.ToTime(time.Now().Add(time.Hour)), 0)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, cancel, err := s.Context(b.Context(), session)
		if err != nil {
			b.Fatal(err)
		}
		cancel()
	}
}
