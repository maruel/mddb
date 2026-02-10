package identity

import (
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/ksid"
)

func TestPushSubscriptionService(t *testing.T) {
	tempDir := t.TempDir()
	tablePath := filepath.Join(tempDir, "push_subscriptions.jsonl")

	svc, err := NewPushSubscriptionService(tablePath)
	if err != nil {
		t.Fatalf("NewPushSubscriptionService failed: %v", err)
	}

	userID := ksid.NewID()

	t.Run("Create", func(t *testing.T) {
		sub, err := svc.Create(userID, "https://push.example.com/1", "p256dh-key", "auth-secret")
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if sub.UserID != userID {
			t.Errorf("UserID: got %v, want %v", sub.UserID, userID)
		}
		if sub.Endpoint != "https://push.example.com/1" {
			t.Errorf("Endpoint: got %q, want %q", sub.Endpoint, "https://push.example.com/1")
		}
		if sub.P256dh != "p256dh-key" {
			t.Errorf("P256dh: got %q, want %q", sub.P256dh, "p256dh-key")
		}
		if sub.Auth != "auth-secret" {
			t.Errorf("Auth: got %q, want %q", sub.Auth, "auth-secret")
		}
	})

	t.Run("UpsertByEndpoint", func(t *testing.T) {
		endpoint := "https://push.example.com/upsert"
		sub1, err := svc.Create(userID, endpoint, "key1", "auth1")
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Create with same endpoint should replace.
		sub2, err := svc.Create(userID, endpoint, "key2", "auth2")
		if err != nil {
			t.Fatalf("Create (upsert) failed: %v", err)
		}
		if sub2.ID == sub1.ID {
			t.Error("upsert should create a new ID")
		}
		if sub2.P256dh != "key2" {
			t.Errorf("P256dh after upsert: got %q, want %q", sub2.P256dh, "key2")
		}
	})

	t.Run("ListByUser", func(t *testing.T) {
		svc2, err := NewPushSubscriptionService(filepath.Join(t.TempDir(), "ps.jsonl"))
		if err != nil {
			t.Fatalf("NewPushSubscriptionService failed: %v", err)
		}
		uid := ksid.NewID()
		otherUID := ksid.NewID()

		if _, err := svc2.Create(uid, "https://a.com", "k1", "a1"); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if _, err := svc2.Create(uid, "https://b.com", "k2", "a2"); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if _, err := svc2.Create(otherUID, "https://c.com", "k3", "a3"); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		var count int
		for range svc2.ListByUser(uid) {
			count++
		}
		if count != 2 {
			t.Errorf("ListByUser: got %d, want 2", count)
		}
	})

	t.Run("DeleteByEndpoint", func(t *testing.T) {
		svc2, err := NewPushSubscriptionService(filepath.Join(t.TempDir(), "ps.jsonl"))
		if err != nil {
			t.Fatalf("NewPushSubscriptionService failed: %v", err)
		}
		uid := ksid.NewID()
		endpoint := "https://push.example.com/delete-ep"
		if _, err := svc2.Create(uid, endpoint, "k", "a"); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if err := svc2.DeleteByEndpoint(endpoint); err != nil {
			t.Fatalf("DeleteByEndpoint failed: %v", err)
		}

		// Should be gone.
		var count int
		for range svc2.ListByUser(uid) {
			count++
		}
		if count != 0 {
			t.Errorf("after delete: got %d, want 0", count)
		}
	})

	t.Run("DeleteByEndpointNotFound", func(t *testing.T) {
		if err := svc.DeleteByEndpoint("https://nonexistent.com"); err == nil {
			t.Fatal("expected error for missing endpoint")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		sub, err := svc.Create(userID, "https://push.example.com/del-id", "k", "a")
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := svc.Delete(sub.ID); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	})

	t.Run("Validate", func(t *testing.T) {
		tests := []struct {
			name    string
			sub     *PushSubscription
			wantErr bool
		}{
			{
				name:    "valid",
				sub:     &PushSubscription{ID: ksid.NewID(), UserID: ksid.NewID(), Endpoint: "https://push.example.com"},
				wantErr: false,
			},
			{
				name:    "missing_id",
				sub:     &PushSubscription{UserID: ksid.NewID(), Endpoint: "https://push.example.com"},
				wantErr: true,
			},
			{
				name:    "missing_user_id",
				sub:     &PushSubscription{ID: ksid.NewID(), Endpoint: "https://push.example.com"},
				wantErr: true,
			},
			{
				name:    "missing_endpoint",
				sub:     &PushSubscription{ID: ksid.NewID(), UserID: ksid.NewID()},
				wantErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := tt.sub.Validate()
				if (err != nil) != tt.wantErr {
					t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})
}
