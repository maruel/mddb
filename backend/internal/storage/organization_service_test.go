package storage

import (
	"context"
	"os"
	"testing"

	"github.com/maruel/mddb/backend/internal/storage/entity"
)

func TestOrganizationService(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-org-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileStore, err := NewFileStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewOrganizationService(tempDir, fileStore, nil)
	if err != nil {
		t.Fatalf("NewOrganizationService failed: %v", err)
	}

	// Test CreateOrganization
	org, err := service.CreateOrganization(context.Background(), "Test Organization")
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	if org.Name != "Test Organization" {
		t.Errorf("Name = %q, want %q", org.Name, "Test Organization")
	}
	if org.ID.IsZero() {
		t.Error("Expected non-zero ID")
	}
	if org.Created.IsZero() {
		t.Error("Expected non-zero Created time")
	}
	if org.Onboarding.Completed {
		t.Error("Expected Onboarding.Completed = false for new org")
	}
	if org.Onboarding.Step != "name" {
		t.Errorf("Onboarding.Step = %q, want %q", org.Onboarding.Step, "name")
	}

	// Test CreateOrganization with empty name
	_, err = service.CreateOrganization(context.Background(), "")
	if err == nil {
		t.Error("Expected error when creating organization with empty name")
	}

	// Test GetOrganization
	retrieved, err := service.GetOrganization(org.ID)
	if err != nil {
		t.Fatalf("GetOrganization failed: %v", err)
	}
	if retrieved.Name != "Test Organization" {
		t.Errorf("Retrieved Name = %q, want %q", retrieved.Name, "Test Organization")
	}

	// Test GetOrganization with non-existent ID
	_, err = service.GetOrganization(testID(99999))
	if err == nil {
		t.Error("Expected error for non-existent organization")
	}

	// Test GetOrganizationByID (string version)
	retrieved2, err := service.GetOrganizationByID(org.ID.String())
	if err != nil {
		t.Fatalf("GetOrganizationByID failed: %v", err)
	}
	if retrieved2.ID != org.ID {
		t.Errorf("Retrieved ID = %v, want %v", retrieved2.ID, org.ID)
	}

	// Test GetOrganizationByID with invalid ID
	_, err = service.GetOrganizationByID("invalid-id")
	if err == nil {
		t.Error("Expected error for invalid ID string")
	}

	// Test ListOrganizations
	orgs, err := service.ListOrganizations()
	if err != nil {
		t.Fatalf("ListOrganizations failed: %v", err)
	}
	if len(orgs) != 1 {
		t.Errorf("ListOrganizations returned %d orgs, want 1", len(orgs))
	}

	// Create a second organization
	org2, err := service.CreateOrganization(context.Background(), "Second Org")
	if err != nil {
		t.Fatalf("CreateOrganization (second) failed: %v", err)
	}

	orgs, _ = service.ListOrganizations()
	if len(orgs) != 2 {
		t.Errorf("ListOrganizations returned %d orgs after second create, want 2", len(orgs))
	}

	// Test UpdateSettings
	newSettings := entity.OrganizationSettings{
		AllowedDomains: []string{"example.com"},
		PublicAccess:   true,
	}
	err = service.UpdateSettings(org.ID, newSettings)
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}

	updatedOrg, _ := service.GetOrganization(org.ID)
	if !updatedOrg.Settings.PublicAccess {
		t.Error("Expected Settings.PublicAccess = true after update")
	}
	if len(updatedOrg.Settings.AllowedDomains) != 1 {
		t.Errorf("AllowedDomains length = %d, want 1", len(updatedOrg.Settings.AllowedDomains))
	}

	// Test UpdateSettings with non-existent ID
	err = service.UpdateSettings(testID(99999), newSettings)
	if err == nil {
		t.Error("Expected error when updating settings for non-existent org")
	}

	// Test UpdateOnboarding
	newState := entity.OnboardingState{
		Completed: true,
		Step:      "done",
	}
	err = service.UpdateOnboarding(org2.ID, newState)
	if err != nil {
		t.Fatalf("UpdateOnboarding failed: %v", err)
	}

	updatedOrg2, _ := service.GetOrganization(org2.ID)
	if !updatedOrg2.Onboarding.Completed {
		t.Error("Expected Onboarding.Completed = true after update")
	}
	if updatedOrg2.Onboarding.Step != "done" {
		t.Errorf("Onboarding.Step = %q, want %q", updatedOrg2.Onboarding.Step, "done")
	}
	if updatedOrg2.Onboarding.UpdatedAt.IsZero() {
		t.Error("Expected Onboarding.UpdatedAt to be set")
	}

	// Test UpdateOnboarding with non-existent ID
	err = service.UpdateOnboarding(testID(99999), newState)
	if err == nil {
		t.Error("Expected error when updating onboarding for non-existent org")
	}

	// Test RootDir
	if service.RootDir() != tempDir {
		t.Errorf("RootDir() = %q, want %q", service.RootDir(), tempDir)
	}
}

func TestOrganizationService_Persistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-org-persist-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	// Create service and add organization
	fileStore, _ := NewFileStore(tempDir)
	service1, err := NewOrganizationService(tempDir, fileStore, nil)
	if err != nil {
		t.Fatal(err)
	}

	org, err := service1.CreateOrganization(context.Background(), "Persistent Org")
	if err != nil {
		t.Fatal(err)
	}

	orgID := org.ID

	// Create new service instance (simulating restart)
	fileStore2, _ := NewFileStore(tempDir)
	service2, err := NewOrganizationService(tempDir, fileStore2, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify organization persisted
	retrieved, err := service2.GetOrganization(orgID)
	if err != nil {
		t.Fatalf("Failed to retrieve persisted organization: %v", err)
	}
	if retrieved.Name != "Persistent Org" {
		t.Errorf("Persisted Name = %q, want %q", retrieved.Name, "Persistent Org")
	}
}
