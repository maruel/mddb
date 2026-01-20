package entity

import (
	"context"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestGetOrgID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected jsonldb.ID
	}{
		{
			name:     "context with org ID",
			ctx:      context.WithValue(context.Background(), OrgKey, jsonldb.ID(123)),
			expected: jsonldb.ID(123),
		},
		{
			name:     "context without org ID",
			ctx:      context.Background(),
			expected: jsonldb.ID(0),
		},
		{
			name:     "context with wrong type value",
			ctx:      context.WithValue(context.Background(), OrgKey, "not an ID"),
			expected: jsonldb.ID(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetOrgID(tt.ctx)
			if got != tt.expected {
				t.Errorf("GetOrgID() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDataRecord_Clone(t *testing.T) {
	original := &DataRecord{
		ID:       jsonldb.ID(1),
		Data:     map[string]any{"name": "test", "count": 42},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	clone := original.Clone()

	// Check that values are copied
	if clone.ID != original.ID {
		t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
	}
	if clone.Data["name"] != original.Data["name"] {
		t.Error("Clone Data not properly copied")
	}

	// Check that modifying clone doesn't affect original
	clone.Data["name"] = "modified"
	if original.Data["name"] == "modified" {
		t.Error("Clone Data should not share reference with original")
	}
}

func TestDataRecord_Clone_NilData(t *testing.T) {
	original := &DataRecord{
		ID:   jsonldb.ID(1),
		Data: nil,
	}

	clone := original.Clone()

	if clone.Data != nil {
		t.Error("Clone of nil Data should be nil")
	}
}

func TestDataRecord_GetID(t *testing.T) {
	record := &DataRecord{ID: jsonldb.ID(42)}
	if record.GetID() != jsonldb.ID(42) {
		t.Errorf("GetID() = %v, want %v", record.GetID(), jsonldb.ID(42))
	}
}

func TestDataRecord_Validate(t *testing.T) {
	tests := []struct {
		name    string
		record  *DataRecord
		wantErr bool
	}{
		{
			name:    "valid record",
			record:  &DataRecord{ID: jsonldb.ID(1)},
			wantErr: false,
		},
		{
			name:    "zero ID",
			record:  &DataRecord{ID: jsonldb.ID(0)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.record.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMembership_Clone(t *testing.T) {
	original := &Membership{
		ID:             jsonldb.ID(1),
		UserID:         jsonldb.ID(2),
		OrganizationID: jsonldb.ID(3),
		Role:           UserRoleAdmin,
		Settings:       MembershipSettings{Notifications: true},
		Created:        time.Now(),
	}

	clone := original.Clone()

	if clone.ID != original.ID {
		t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
	}
	if clone.Role != original.Role {
		t.Errorf("Clone Role = %v, want %v", clone.Role, original.Role)
	}
}

func TestMembership_GetID(t *testing.T) {
	m := &Membership{ID: jsonldb.ID(99)}
	if m.GetID() != jsonldb.ID(99) {
		t.Errorf("GetID() = %v, want %v", m.GetID(), jsonldb.ID(99))
	}
}

func TestMembership_Validate(t *testing.T) {
	tests := []struct {
		name       string
		membership *Membership
		wantErr    bool
		errContain string
	}{
		{
			name: "valid membership",
			membership: &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(2),
				OrganizationID: jsonldb.ID(3),
				Role:           UserRoleAdmin,
			},
			wantErr: false,
		},
		{
			name: "zero ID",
			membership: &Membership{
				ID:             jsonldb.ID(0),
				UserID:         jsonldb.ID(2),
				OrganizationID: jsonldb.ID(3),
				Role:           UserRoleAdmin,
			},
			wantErr:    true,
			errContain: "id is required",
		},
		{
			name: "zero UserID",
			membership: &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(0),
				OrganizationID: jsonldb.ID(3),
				Role:           UserRoleAdmin,
			},
			wantErr:    true,
			errContain: "user_id is required",
		},
		{
			name: "zero OrganizationID",
			membership: &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(2),
				OrganizationID: jsonldb.ID(0),
				Role:           UserRoleAdmin,
			},
			wantErr:    true,
			errContain: "organization_id is required",
		},
		{
			name: "empty Role",
			membership: &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(2),
				OrganizationID: jsonldb.ID(3),
				Role:           "",
			},
			wantErr:    true,
			errContain: "role is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.membership.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errContain != "" {
				if got := err.Error(); got != tt.errContain {
					t.Errorf("Validate() error = %q, want containing %q", got, tt.errContain)
				}
			}
		})
	}
}

func TestOrganization_Clone(t *testing.T) {
	original := &Organization{
		ID:   jsonldb.ID(1),
		Name: "Test Org",
		Settings: OrganizationSettings{
			AllowedDomains: []string{"example.com", "test.com"},
			PublicAccess:   true,
		},
		Created: time.Now(),
	}

	clone := original.Clone()

	if clone.ID != original.ID {
		t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
	}
	if clone.Name != original.Name {
		t.Errorf("Clone Name = %v, want %v", clone.Name, original.Name)
	}

	// Check that AllowedDomains is deep copied
	clone.Settings.AllowedDomains[0] = "modified.com"
	if original.Settings.AllowedDomains[0] == "modified.com" {
		t.Error("Clone should not share AllowedDomains reference with original")
	}
}

func TestOrganization_Clone_NilAllowedDomains(t *testing.T) {
	original := &Organization{
		ID:       jsonldb.ID(1),
		Name:     "Test Org",
		Settings: OrganizationSettings{AllowedDomains: nil},
	}

	clone := original.Clone()

	if clone.Settings.AllowedDomains != nil {
		t.Error("Clone of nil AllowedDomains should be nil")
	}
}

func TestOrganization_GetID(t *testing.T) {
	o := &Organization{ID: jsonldb.ID(77)}
	if o.GetID() != jsonldb.ID(77) {
		t.Errorf("GetID() = %v, want %v", o.GetID(), jsonldb.ID(77))
	}
}

func TestOrganization_Validate(t *testing.T) {
	tests := []struct {
		name    string
		org     *Organization
		wantErr bool
	}{
		{
			name:    "valid organization",
			org:     &Organization{ID: jsonldb.ID(1), Name: "Test Org"},
			wantErr: false,
		},
		{
			name:    "zero ID",
			org:     &Organization{ID: jsonldb.ID(0), Name: "Test Org"},
			wantErr: true,
		},
		{
			name:    "empty Name",
			org:     &Organization{ID: jsonldb.ID(1), Name: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.org.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGitRemote_Clone(t *testing.T) {
	original := &GitRemote{
		ID:             jsonldb.ID(1),
		OrganizationID: jsonldb.ID(2),
		Name:           "origin",
		URL:            "https://github.com/test/repo",
		Type:           "github",
		AuthType:       "token",
		Created:        time.Now(),
	}

	clone := original.Clone()

	if clone.ID != original.ID {
		t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
	}
	if clone.URL != original.URL {
		t.Errorf("Clone URL = %v, want %v", clone.URL, original.URL)
	}
}

func TestGitRemote_GetID(t *testing.T) {
	g := &GitRemote{ID: jsonldb.ID(55)}
	if g.GetID() != jsonldb.ID(55) {
		t.Errorf("GetID() = %v, want %v", g.GetID(), jsonldb.ID(55))
	}
}

func TestGitRemote_Validate(t *testing.T) {
	tests := []struct {
		name    string
		remote  *GitRemote
		wantErr bool
	}{
		{
			name: "valid remote",
			remote: &GitRemote{
				ID:             jsonldb.ID(1),
				OrganizationID: jsonldb.ID(2),
				URL:            "https://github.com/test/repo",
			},
			wantErr: false,
		},
		{
			name: "zero ID",
			remote: &GitRemote{
				ID:             jsonldb.ID(0),
				OrganizationID: jsonldb.ID(2),
				URL:            "https://github.com/test/repo",
			},
			wantErr: true,
		},
		{
			name: "zero OrganizationID",
			remote: &GitRemote{
				ID:             jsonldb.ID(1),
				OrganizationID: jsonldb.ID(0),
				URL:            "https://github.com/test/repo",
			},
			wantErr: true,
		},
		{
			name: "empty URL",
			remote: &GitRemote{
				ID:             jsonldb.ID(1),
				OrganizationID: jsonldb.ID(2),
				URL:            "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.remote.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInvitation_Clone(t *testing.T) {
	original := &Invitation{
		ID:             jsonldb.ID(1),
		Email:          "test@example.com",
		OrganizationID: jsonldb.ID(2),
		Role:           UserRoleEditor,
		Token:          "abc123",
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		Created:        time.Now(),
	}

	clone := original.Clone()

	if clone.ID != original.ID {
		t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
	}
	if clone.Email != original.Email {
		t.Errorf("Clone Email = %v, want %v", clone.Email, original.Email)
	}
}

func TestInvitation_GetID(t *testing.T) {
	i := &Invitation{ID: jsonldb.ID(33)}
	if i.GetID() != jsonldb.ID(33) {
		t.Errorf("GetID() = %v, want %v", i.GetID(), jsonldb.ID(33))
	}
}

func TestInvitation_Validate(t *testing.T) {
	tests := []struct {
		name    string
		inv     *Invitation
		wantErr bool
	}{
		{
			name: "valid invitation",
			inv: &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(2),
				Token:          "abc123",
			},
			wantErr: false,
		},
		{
			name: "zero ID",
			inv: &Invitation{
				ID:             jsonldb.ID(0),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(2),
				Token:          "abc123",
			},
			wantErr: true,
		},
		{
			name: "empty Email",
			inv: &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "",
				OrganizationID: jsonldb.ID(2),
				Token:          "abc123",
			},
			wantErr: true,
		},
		{
			name: "zero OrganizationID",
			inv: &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(0),
				Token:          "abc123",
			},
			wantErr: true,
		},
		{
			name: "empty Token",
			inv: &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(2),
				Token:          "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.inv.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUser_GetID(t *testing.T) {
	u := &User{ID: jsonldb.ID(88)}
	if u.GetID() != jsonldb.ID(88) {
		t.Errorf("GetID() = %v, want %v", u.GetID(), jsonldb.ID(88))
	}
}
