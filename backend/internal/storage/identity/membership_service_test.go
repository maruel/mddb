package identity

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

func TestMembershipService(t *testing.T) {
	tempDir := t.TempDir()

	service, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatalf("NewMembershipService failed: %v", err)
	}

	userID := jsonldb.ID(100)
	orgID := jsonldb.ID(200)

	// Test CreateMembership
	membership, err := service.CreateMembership(userID, orgID, entity.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateMembership failed: %v", err)
	}

	if membership.UserID != userID {
		t.Errorf("UserID = %v, want %v", membership.UserID, userID)
	}
	if membership.OrganizationID != orgID {
		t.Errorf("OrganizationID = %v, want %v", membership.OrganizationID, orgID)
	}
	if membership.Role != entity.UserRoleAdmin {
		t.Errorf("Role = %v, want %v", membership.Role, entity.UserRoleAdmin)
	}
	if membership.ID.IsZero() {
		t.Error("Expected non-zero membership ID")
	}

	// Test creating duplicate membership
	_, err = service.CreateMembership(userID, orgID, entity.UserRoleEditor)
	if err == nil {
		t.Error("Expected error when creating duplicate membership")
	}

	// Test CreateMembership with invalid user ID (contains invalid character @)
	_, err = service.CreateMembership(jsonldb.ID(0), orgID, entity.UserRoleAdmin)
	if err == nil {
		t.Error("Expected error for invalid user ID")
	}

	// Test CreateMembership with invalid org ID (contains invalid character @)
	_, err = service.CreateMembership(userID, jsonldb.ID(0), entity.UserRoleAdmin)
	if err == nil {
		t.Error("Expected error for invalid org ID")
	}

	// Test GetMembership
	retrieved, err := service.GetMembership(userID, orgID)
	if err != nil {
		t.Fatalf("GetMembership failed: %v", err)
	}
	if retrieved.Role != entity.UserRoleAdmin {
		t.Errorf("Role = %v, want %v", retrieved.Role, entity.UserRoleAdmin)
	}

	// Test GetMembership with non-existent
	_, err = service.GetMembership(jsonldb.ID(999), orgID)
	if err == nil {
		t.Error("Expected error for non-existent membership")
	}

	// Create second user in same org
	userID2 := jsonldb.ID(101)
	_, _ = service.CreateMembership(userID2, orgID, entity.UserRoleViewer)

	// Create same user in different org
	orgID2 := jsonldb.ID(201)
	_, _ = service.CreateMembership(userID, orgID2, entity.UserRoleEditor)

	// Test ListByUser
	userMemberships, err := service.ListByUser(userID)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(userMemberships) != 2 {
		t.Errorf("Expected 2 memberships for user, got %d", len(userMemberships))
	}

	// Test ListByUser with invalid ID (contains invalid character @)
	_, err = service.ListByUser(jsonldb.ID(0))
	if err == nil {
		t.Error("Expected error for invalid user ID in ListByUser")
	}

	// Test UpdateRole
	err = service.UpdateRole(userID, orgID, entity.UserRoleEditor)
	if err != nil {
		t.Fatalf("UpdateRole failed: %v", err)
	}

	updated, _ := service.GetMembership(userID, orgID)
	if updated.Role != entity.UserRoleEditor {
		t.Errorf("Role after update = %v, want %v", updated.Role, entity.UserRoleEditor)
	}

	// Test UpdateRole with non-existent
	err = service.UpdateRole(jsonldb.ID(999), orgID, entity.UserRoleAdmin)
	if err == nil {
		t.Error("Expected error when updating role for non-existent membership")
	}

	// Test UpdateSettings
	newSettings := entity.MembershipSettings{Notifications: true}
	err = service.UpdateSettings(userID, orgID, newSettings)
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}

	updatedSettings, _ := service.GetMembership(userID, orgID)
	if !updatedSettings.Settings.Notifications {
		t.Error("Settings.Notifications = false, want true")
	}

	// Test UpdateSettings with non-existent
	err = service.UpdateSettings(jsonldb.ID(999), orgID, newSettings)
	if err == nil {
		t.Error("Expected error when updating settings for non-existent membership")
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

	_, err = service1.CreateMembership(userID, orgID, entity.UserRoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	// Create new service instance (simulating restart)
	service2, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify membership persisted
	retrieved, err := service2.GetMembership(userID, orgID)
	if err != nil {
		t.Fatalf("Failed to retrieve persisted membership: %v", err)
	}
	if retrieved.Role != entity.UserRoleAdmin {
		t.Errorf("Persisted Role = %v, want %v", retrieved.Role, entity.UserRoleAdmin)
	}
}
