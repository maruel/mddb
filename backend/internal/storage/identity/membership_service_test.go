package identity

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestMembershipService(t *testing.T) {
	tempDir := t.TempDir()

	service, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatalf("NewMembershipService failed: %v", err)
	}

	userID := jsonldb.ID(100)
	orgID := jsonldb.ID(200)

	// Test Create
	membership, err := service.Create(userID, orgID, UserRoleAdmin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if membership.UserID != userID {
		t.Errorf("UserID = %v, want %v", membership.UserID, userID)
	}
	if membership.OrganizationID != orgID {
		t.Errorf("OrganizationID = %v, want %v", membership.OrganizationID, orgID)
	}
	if membership.Role != UserRoleAdmin {
		t.Errorf("Role = %v, want %v", membership.Role, UserRoleAdmin)
	}
	if membership.ID.IsZero() {
		t.Error("Expected non-zero membership ID")
	}

	// Test creating duplicate membership
	_, err = service.Create(userID, orgID, UserRoleEditor)
	if err == nil {
		t.Error("Expected error when creating duplicate membership")
	}

	// Test Create with invalid user ID (contains invalid character @)
	_, err = service.Create(jsonldb.ID(0), orgID, UserRoleAdmin)
	if err == nil {
		t.Error("Expected error for invalid user ID")
	}

	// Test Create with invalid org ID (contains invalid character @)
	_, err = service.Create(userID, jsonldb.ID(0), UserRoleAdmin)
	if err == nil {
		t.Error("Expected error for invalid org ID")
	}

	// Test Get
	retrieved, err := service.Get(userID, orgID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Role != UserRoleAdmin {
		t.Errorf("Role = %v, want %v", retrieved.Role, UserRoleAdmin)
	}

	// Test Get with non-existent
	_, err = service.Get(jsonldb.ID(999), orgID)
	if err == nil {
		t.Error("Expected error for non-existent membership")
	}

	// Create second user in same org
	userID2 := jsonldb.ID(101)
	_, _ = service.Create(userID2, orgID, UserRoleViewer)

	// Create same user in different org
	orgID2 := jsonldb.ID(201)
	_, _ = service.Create(userID, orgID2, UserRoleEditor)

	// Test Iter
	iter, err := service.Iter(userID)
	if err != nil {
		t.Fatalf("Iter failed: %v", err)
	}
	count := 0
	for range iter {
		count++
	}
	if count != 2 {
		t.Errorf("Expected 2 memberships for user, got %d", count)
	}

	// Test Iter with invalid ID
	_, err = service.Iter(jsonldb.ID(0))
	if err == nil {
		t.Error("Expected error for invalid user ID in Iter")
	}

	// Test Modify (role change)
	toUpdate, _ := service.Get(userID, orgID)
	_, err = service.Modify(toUpdate.ID, func(m *Membership) error {
		m.Role = UserRoleEditor
		return nil
	})
	if err != nil {
		t.Fatalf("Modify failed: %v", err)
	}

	updated, _ := service.Get(userID, orgID)
	if updated.Role != UserRoleEditor {
		t.Errorf("Role after update = %v, want %v", updated.Role, UserRoleEditor)
	}

	// Test Modify with non-existent
	_, err = service.Modify(jsonldb.ID(999), func(m *Membership) error {
		return nil
	})
	if err == nil {
		t.Error("Expected error when modifying non-existent membership")
	}

	// Test Modify (settings change)
	toUpdate, _ = service.Get(userID, orgID)
	_, err = service.Modify(toUpdate.ID, func(m *Membership) error {
		m.Settings = MembershipSettings{Notifications: true}
		return nil
	})
	if err != nil {
		t.Fatalf("Modify failed: %v", err)
	}

	updatedSettings, _ := service.Get(userID, orgID)
	if !updatedSettings.Settings.Notifications {
		t.Error("Settings.Notifications = false, want true")
	}
}

func TestMembershipService_Persistence(t *testing.T) {
	tempDir := t.TempDir()

	userID := jsonldb.ID(100)
	orgID := jsonldb.ID(200)

	// Create service and add membership
	service1, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service1.Create(userID, orgID, UserRoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	// Create new service instance (simulating restart)
	service2, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify membership persisted
	retrieved, err := service2.Get(userID, orgID)
	if err != nil {
		t.Fatalf("Failed to retrieve persisted membership: %v", err)
	}
	if retrieved.Role != UserRoleAdmin {
		t.Errorf("Persisted Role = %v, want %v", retrieved.Role, UserRoleAdmin)
	}
}
