package identity

import (
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestNotificationService(t *testing.T) {
	tempDir := t.TempDir()
	tablePath := filepath.Join(tempDir, "notifications.jsonl")

	svc, err := NewNotificationService(tablePath)
	if err != nil {
		t.Fatalf("NewNotificationService failed: %v", err)
	}

	userID := jsonldb.NewID()
	actorID := jsonldb.NewID()

	t.Run("Create", func(t *testing.T) {
		n, err := svc.Create(userID, NotifOrgInvite, "You were invited", "To Acme org", "org123", actorID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if n.UserID != userID {
			t.Errorf("UserID: got %v, want %v", n.UserID, userID)
		}
		if n.Type != NotifOrgInvite {
			t.Errorf("Type: got %v, want %v", n.Type, NotifOrgInvite)
		}
		if n.Title != "You were invited" {
			t.Errorf("Title: got %q, want %q", n.Title, "You were invited")
		}
		if n.Read {
			t.Error("new notification should be unread")
		}
	})

	t.Run("Get", func(t *testing.T) {
		n, err := svc.Create(userID, NotifWSInvite, "Workspace invite", "", "", 0)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		got, err := svc.Get(n.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got.ID != n.ID {
			t.Errorf("ID mismatch: got %v, want %v", got.ID, n.ID)
		}
	})

	t.Run("GetNotFound", func(t *testing.T) {
		_, err := svc.Get(jsonldb.NewID())
		if err == nil {
			t.Fatal("expected error for missing notification")
		}
	})

	t.Run("ListByUser", func(t *testing.T) {
		svc2, err := NewNotificationService(filepath.Join(t.TempDir(), "n.jsonl"))
		if err != nil {
			t.Fatalf("NewNotificationService failed: %v", err)
		}
		uid := jsonldb.NewID()
		otherUID := jsonldb.NewID()

		for i := range 5 {
			title := "notif " + string(rune('A'+i))
			if _, err := svc2.Create(uid, NotifMemberJoined, title, "", "", 0); err != nil {
				t.Fatalf("Create %d failed: %v", i, err)
			}
		}
		// Create one for another user.
		if _, err := svc2.Create(otherUID, NotifMemberJoined, "other", "", "", 0); err != nil {
			t.Fatalf("Create other failed: %v", err)
		}

		all := svc2.ListByUser(uid, 0, 0, false)
		if len(all) != 5 {
			t.Fatalf("ListByUser: got %d, want 5", len(all))
		}
		// Newest first.
		if all[0].Title <= all[4].Title {
			t.Error("expected newest-first ordering")
		}
	})

	t.Run("ListByUserWithLimitOffset", func(t *testing.T) {
		svc2, err := NewNotificationService(filepath.Join(t.TempDir(), "n.jsonl"))
		if err != nil {
			t.Fatalf("NewNotificationService failed: %v", err)
		}
		uid := jsonldb.NewID()
		for i := range 10 {
			title := "n" + string(rune('0'+i))
			if _, err := svc2.Create(uid, NotifPageMention, title, "", "", 0); err != nil {
				t.Fatalf("Create %d failed: %v", i, err)
			}
		}

		page := svc2.ListByUser(uid, 3, 2, false)
		if len(page) != 3 {
			t.Fatalf("ListByUser with limit/offset: got %d, want 3", len(page))
		}
	})

	t.Run("ListByUserUnreadOnly", func(t *testing.T) {
		svc2, err := NewNotificationService(filepath.Join(t.TempDir(), "n.jsonl"))
		if err != nil {
			t.Fatalf("NewNotificationService failed: %v", err)
		}
		uid := jsonldb.NewID()
		n1, err := svc2.Create(uid, NotifOrgInvite, "a", "", "", 0)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if _, err := svc2.Create(uid, NotifOrgInvite, "b", "", "", 0); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := svc2.MarkRead(n1.ID, uid); err != nil {
			t.Fatalf("MarkRead failed: %v", err)
		}

		unread := svc2.ListByUser(uid, 0, 0, true)
		if len(unread) != 1 {
			t.Fatalf("unread only: got %d, want 1", len(unread))
		}
	})

	t.Run("CountByUser", func(t *testing.T) {
		svc2, err := NewNotificationService(filepath.Join(t.TempDir(), "n.jsonl"))
		if err != nil {
			t.Fatalf("NewNotificationService failed: %v", err)
		}
		uid := jsonldb.NewID()
		if _, err := svc2.Create(uid, NotifOrgInvite, "a", "", "", 0); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if _, err := svc2.Create(uid, NotifOrgInvite, "b", "", "", 0); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if got := svc2.CountByUser(uid); got != 2 {
			t.Errorf("CountByUser: got %d, want 2", got)
		}
	})

	t.Run("CountUnread", func(t *testing.T) {
		svc2, err := NewNotificationService(filepath.Join(t.TempDir(), "n.jsonl"))
		if err != nil {
			t.Fatalf("NewNotificationService failed: %v", err)
		}
		uid := jsonldb.NewID()
		n1, err := svc2.Create(uid, NotifOrgInvite, "a", "", "", 0)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if _, err := svc2.Create(uid, NotifOrgInvite, "b", "", "", 0); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := svc2.MarkRead(n1.ID, uid); err != nil {
			t.Fatalf("MarkRead failed: %v", err)
		}

		if got := svc2.CountUnread(uid); got != 1 {
			t.Errorf("CountUnread: got %d, want 1", got)
		}
	})

	t.Run("MarkRead", func(t *testing.T) {
		n, err := svc.Create(userID, NotifOrgInvite, "mark me", "", "", 0)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := svc.MarkRead(n.ID, userID); err != nil {
			t.Fatalf("MarkRead failed: %v", err)
		}
		got, err := svc.Get(n.ID)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if !got.Read {
			t.Error("notification should be read")
		}
	})

	t.Run("MarkReadWrongUser", func(t *testing.T) {
		n, err := svc.Create(userID, NotifOrgInvite, "wrong user", "", "", 0)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := svc.MarkRead(n.ID, jsonldb.NewID()); err == nil {
			t.Fatal("expected error marking read for wrong user")
		}
	})

	t.Run("MarkAllRead", func(t *testing.T) {
		svc2, err := NewNotificationService(filepath.Join(t.TempDir(), "n.jsonl"))
		if err != nil {
			t.Fatalf("NewNotificationService failed: %v", err)
		}
		uid := jsonldb.NewID()
		for _, title := range []string{"a", "b", "c"} {
			if _, err := svc2.Create(uid, NotifOrgInvite, title, "", "", 0); err != nil {
				t.Fatalf("Create failed: %v", err)
			}
		}

		if err := svc2.MarkAllRead(uid); err != nil {
			t.Fatalf("MarkAllRead failed: %v", err)
		}
		if got := svc2.CountUnread(uid); got != 0 {
			t.Errorf("CountUnread after MarkAllRead: got %d, want 0", got)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		n, err := svc.Create(userID, NotifOrgInvite, "delete me", "", "", 0)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := svc.Delete(n.ID, userID); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if _, err := svc.Get(n.ID); err == nil {
			t.Fatal("expected error after delete")
		}
	})

	t.Run("DeleteWrongUser", func(t *testing.T) {
		n, err := svc.Create(userID, NotifOrgInvite, "wrong user", "", "", 0)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if err := svc.Delete(n.ID, jsonldb.NewID()); err == nil {
			t.Fatal("expected error deleting for wrong user")
		}
	})

	t.Run("DeleteOlderThan", func(t *testing.T) {
		svc2, err := NewNotificationService(filepath.Join(t.TempDir(), "n.jsonl"))
		if err != nil {
			t.Fatalf("NewNotificationService failed: %v", err)
		}
		uid := jsonldb.NewID()
		if _, err := svc2.Create(uid, NotifOrgInvite, "old", "", "", 0); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if _, err := svc2.Create(uid, NotifOrgInvite, "new", "", "", 0); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Delete everything before "now + 1 minute" (should delete all).
		cutoff := storage.Now() + 60_000
		count, err := svc2.DeleteOlderThan(cutoff)
		if err != nil {
			t.Fatalf("DeleteOlderThan failed: %v", err)
		}
		if count != 2 {
			t.Errorf("DeleteOlderThan: deleted %d, want 2", count)
		}
	})

	t.Run("DeleteExcessPerUser", func(t *testing.T) {
		svc2, err := NewNotificationService(filepath.Join(t.TempDir(), "n.jsonl"))
		if err != nil {
			t.Fatalf("NewNotificationService failed: %v", err)
		}
		uid := jsonldb.NewID()
		for range 5 {
			if _, err := svc2.Create(uid, NotifOrgInvite, "x", "", "", 0); err != nil {
				t.Fatalf("Create failed: %v", err)
			}
		}

		count, err := svc2.DeleteExcessPerUser(3)
		if err != nil {
			t.Fatalf("DeleteExcessPerUser failed: %v", err)
		}
		if count != 2 {
			t.Errorf("DeleteExcessPerUser: deleted %d, want 2", count)
		}
		if got := svc2.CountByUser(uid); got != 3 {
			t.Errorf("CountByUser after excess deletion: got %d, want 3", got)
		}
	})

	t.Run("Validate", func(t *testing.T) {
		tests := []struct {
			name    string
			n       *Notification
			wantErr bool
		}{
			{
				name:    "valid",
				n:       &Notification{ID: jsonldb.NewID(), UserID: jsonldb.NewID(), Type: NotifOrgInvite, Title: "hi"},
				wantErr: false,
			},
			{
				name:    "missing_id",
				n:       &Notification{UserID: jsonldb.NewID(), Type: NotifOrgInvite, Title: "hi"},
				wantErr: true,
			},
			{
				name:    "missing_user_id",
				n:       &Notification{ID: jsonldb.NewID(), Type: NotifOrgInvite, Title: "hi"},
				wantErr: true,
			},
			{
				name:    "missing_type",
				n:       &Notification{ID: jsonldb.NewID(), UserID: jsonldb.NewID(), Title: "hi"},
				wantErr: true,
			},
			{
				name:    "missing_title",
				n:       &Notification{ID: jsonldb.NewID(), UserID: jsonldb.NewID(), Type: NotifOrgInvite},
				wantErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := tt.n.Validate()
				if (err != nil) != tt.wantErr {
					t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})
}

func TestNotificationPreferences(t *testing.T) {
	t.Run("DefaultChannels", func(t *testing.T) {
		cs := DefaultChannels(NotifOrgInvite)
		if !cs.Email || !cs.Web {
			t.Errorf("org_invite defaults: got email=%v web=%v, want both true", cs.Email, cs.Web)
		}

		cs = DefaultChannels(NotifMemberJoined)
		if cs.Email || !cs.Web {
			t.Errorf("member_joined defaults: got email=%v web=%v, want email=false web=true", cs.Email, cs.Web)
		}
	})

	t.Run("EffectiveChannelsNoOverride", func(t *testing.T) {
		prefs := &NotificationPreferences{}
		cs := prefs.EffectiveChannels(NotifOrgInvite)
		if !cs.Email || !cs.Web {
			t.Error("expected defaults when no override")
		}
	})

	t.Run("EffectiveChannelsWithOverride", func(t *testing.T) {
		prefs := &NotificationPreferences{
			Overrides: map[NotificationType]ChannelSet{
				NotifOrgInvite: {Email: false, Web: true},
			},
		}
		cs := prefs.EffectiveChannels(NotifOrgInvite)
		if cs.Email {
			t.Error("expected email=false from override")
		}
		if !cs.Web {
			t.Error("expected web=true from override")
		}
	})

	t.Run("EffectiveChannelsNilPrefs", func(t *testing.T) {
		var prefs *NotificationPreferences
		cs := prefs.EffectiveChannels(NotifOrgInvite)
		if !cs.Email || !cs.Web {
			t.Error("expected defaults for nil prefs")
		}
	})

	t.Run("AllNotificationTypes", func(t *testing.T) {
		types := AllNotificationTypes()
		if len(types) != 6 {
			t.Errorf("AllNotificationTypes: got %d, want 6", len(types))
		}
	})
}
