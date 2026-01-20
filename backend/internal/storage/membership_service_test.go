package storage

import (
	"os"
	"testing"

	"github.com/maruel/mddb/backend/internal/storage/entity"
)

func TestMembershipService(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-membership-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	service, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatalf("NewMembershipService failed: %v", err)
	}

	userID := testID(100)
	orgID := testID(200)

	// Test CreateMembership
	membership, err := service.CreateMembership(userID.String(), orgID.String(), entity.UserRoleAdmin)
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
	_, err = service.CreateMembership(userID.String(), orgID.String(), entity.UserRoleEditor)
	if err == nil {
		t.Error("Expected error when creating duplicate membership")
	}

	// Test CreateMembership with invalid user ID (contains invalid character @)
	_, err = service.CreateMembership("invalid@user", orgID.String(), entity.UserRoleAdmin)
	if err == nil {
		t.Error("Expected error for invalid user ID")
	}

	// Test CreateMembership with invalid org ID (contains invalid character @)
	_, err = service.CreateMembership(userID.String(), "invalid@org", entity.UserRoleAdmin)
	if err == nil {
		t.Error("Expected error for invalid org ID")
	}

	// Test GetMembership
	retrieved, err := service.GetMembership(userID.String(), orgID.String())
	if err != nil {
		t.Fatalf("GetMembership failed: %v", err)
	}
	if retrieved.Role != entity.UserRoleAdmin {
		t.Errorf("Role = %v, want %v", retrieved.Role, entity.UserRoleAdmin)
	}

	// Test GetMembership with non-existent
	_, err = service.GetMembership(testID(999).String(), orgID.String())
	if err == nil {
		t.Error("Expected error for non-existent membership")
	}

	// Create second user in same org
	userID2 := testID(101)
	_, _ = service.CreateMembership(userID2.String(), orgID.String(), entity.UserRoleViewer)

	// Create same user in different org
	orgID2 := testID(201)
	_, _ = service.CreateMembership(userID.String(), orgID2.String(), entity.UserRoleEditor)

	// Test ListByUser
	userMemberships, err := service.ListByUser(userID.String())
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(userMemberships) != 2 {
		t.Errorf("Expected 2 memberships for user, got %d", len(userMemberships))
	}

	// Test ListByUser with invalid ID (contains invalid character @)
	_, err = service.ListByUser("invalid@user")
	if err == nil {
		t.Error("Expected error for invalid user ID in ListByUser")
	}

	// Test ListByOrganization
	orgMemberships, err := service.ListByOrganization(orgID.String())
	if err != nil {
		t.Fatalf("ListByOrganization failed: %v", err)
	}
	if len(orgMemberships) != 2 {
		t.Errorf("Expected 2 memberships for org, got %d", len(orgMemberships))
	}

	// Test ListByOrganization with invalid ID (contains invalid character @)
	_, err = service.ListByOrganization("invalid@org")
	if err == nil {
		t.Error("Expected error for invalid org ID in ListByOrganization")
	}

	// Test UpdateRole
	err = service.UpdateRole(userID.String(), orgID.String(), entity.UserRoleEditor)
	if err != nil {
		t.Fatalf("UpdateRole failed: %v", err)
	}

	updated, _ := service.GetMembership(userID.String(), orgID.String())
	if updated.Role != entity.UserRoleEditor {
		t.Errorf("Role after update = %v, want %v", updated.Role, entity.UserRoleEditor)
	}

	// Test UpdateRole with non-existent
	err = service.UpdateRole(testID(999).String(), orgID.String(), entity.UserRoleAdmin)
	if err == nil {
		t.Error("Expected error when updating role for non-existent membership")
	}

	// Test UpdateSettings
	newSettings := entity.MembershipSettings{Notifications: true}
	err = service.UpdateSettings(userID.String(), orgID.String(), newSettings)
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}

	updatedSettings, _ := service.GetMembership(userID.String(), orgID.String())
	if !updatedSettings.Settings.Notifications {
		t.Error("Settings.Notifications = false, want true")
	}

	// Test UpdateSettings with non-existent
	err = service.UpdateSettings(testID(999).String(), orgID.String(), newSettings)
	if err == nil {
		t.Error("Expected error when updating settings for non-existent membership")
	}

	// Test DeleteMembership
	err = service.DeleteMembership(userID2.String(), orgID.String())
	if err != nil {
		t.Fatalf("DeleteMembership failed: %v", err)
	}

	// Verify deletion
	_, err = service.GetMembership(userID2.String(), orgID.String())
	if err == nil {
		t.Error("Expected error getting deleted membership")
	}

	// Test DeleteMembership with non-existent
	err = service.DeleteMembership(testID(999).String(), orgID.String())
	if err == nil {
		t.Error("Expected error deleting non-existent membership")
	}
}

func TestMembershipService_Persistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-membership-persist-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	userID := testID(100)
	orgID := testID(200)

	// Create service and add membership
	service1, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service1.CreateMembership(userID.String(), orgID.String(), entity.UserRoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	// Create new service instance (simulating restart)
	service2, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify membership persisted
	retrieved, err := service2.GetMembership(userID.String(), orgID.String())
	if err != nil {
		t.Fatalf("Failed to retrieve persisted membership: %v", err)
	}
	if retrieved.Role != entity.UserRoleAdmin {
		t.Errorf("Persisted Role = %v, want %v", retrieved.Role, entity.UserRoleAdmin)
	}
}
