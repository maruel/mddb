# Organization/Workspace Architecture Redesign

## Overview

This document outlines the complete redesign of the multi-tenancy model from a single-level "Organization" to a two-level "Organization → Workspace" hierarchy.

**Breaking Change**: This is a complete redesign with no backward compatibility.

---

## Terminology

| Term | Definition | Scope |
|------|------------|-------|
| **Organization** | Billing and administrative entity. Represents a company, team, or individual account. | Billing, SSO, global user management |
| **Workspace** | Isolated content container. Has its own pages, tables, assets, and git remote. | Content, collaboration, per-project settings |
| **Organization Member** | User with access to an organization (may access multiple workspaces) | Cross-workspace |
| **Workspace Member** | User with access to a specific workspace | Per-workspace |

---

## Data Model

### Entity Relationship

```
┌──────────────────────────────────────────────────────────────────┐
│                         Organization                              │
│  ID, Name, BillingEmail, Quotas, Settings, Created               │
└──────────────────────────────────────────────────────────────────┘
         │                                    │
         │ 1:N                                │ 1:N
         ▼                                    ▼
┌─────────────────────┐            ┌─────────────────────┐
│ OrganizationMember  │            │     Workspace       │
│ UserID, OrgID, Role │            │ ID, OrgID, Name,    │
│ (owner/admin/member)│            │ Slug, Quotas,       │
└─────────────────────┘            │ Settings, GitRemote │
         │                         └─────────────────────┘
         │                                    │
         │                                    │ 1:N
         │                                    ▼
         │                         ┌─────────────────────┐
         │                         │  WorkspaceMember    │
         │                         │ UserID, WSID, Role  │
         │                         │ (admin/editor/viewer)│
         │                         └─────────────────────┘
         │                                    │
         ▼                                    ▼
┌──────────────────────────────────────────────────────────────────┐
│                              User                                 │
│  ID, Email, Name, IsGlobalAdmin, OAuthIdentities, Settings       │
└──────────────────────────────────────────────────────────────────┘
```

### Organization Entity

```go
type Organization struct {
    ID           jsonldb.ID
    Name         string
    BillingEmail string              // Primary billing contact
    Quotas       OrganizationQuotas
    Settings     OrganizationSettings
    Created      time.Time
}

type OrganizationQuotas struct {
    MaxWorkspaces       int   // Max workspaces in this org (default: 3)
    MaxMembersPerOrg    int   // Max members at org level (default: 10)
    MaxMembersPerWS     int   // Max members per workspace (default: 10)
    MaxTotalStorageGB   int   // Total storage across all workspaces (default: 5)
}

type OrganizationSettings struct {
    // SSO & Security
    AllowedEmailDomains []string // Restrict membership to these domains
    RequireSSO          bool     // Require SSO for all members

    // Defaults for new workspaces
    DefaultWorkspaceQuotas WorkspaceQuotas
}
```

### Workspace Entity

```go
type Workspace struct {
    ID             jsonldb.ID
    OrganizationID jsonldb.ID
    Name           string
    Slug           string            // URL-friendly: "engineering-docs"
    Quotas         WorkspaceQuotas
    Settings       WorkspaceSettings
    GitRemote      *GitRemote
    Created        time.Time
}

type WorkspaceQuotas struct {
    MaxPages           int   // Default: 1000
    MaxStorageMB       int   // Default: 1024 (1GB)
    MaxRecordsPerTable int   // Default: 10000
    MaxAssetSizeMB     int   // Default: 50
}

type WorkspaceSettings struct {
    PublicAccess   bool     // Allow unauthenticated read access
    AllowedDomains []string // Additional domain restrictions (inherits org)
    GitAutoPush    bool     // Auto-push on changes
}
```

### Membership Entities

```go
type OrganizationRole string
const (
    OrgRoleOwner  OrganizationRole = "owner"  // Full control + billing
    OrgRoleAdmin  OrganizationRole = "admin"  // Manage workspaces & members
    OrgRoleMember OrganizationRole = "member" // Access granted workspaces only
)

type OrganizationMembership struct {
    ID             jsonldb.ID
    UserID         jsonldb.ID
    OrganizationID jsonldb.ID
    Role           OrganizationRole
    Created        time.Time
}

type WorkspaceRole string
const (
    WSRoleAdmin  WorkspaceRole = "admin"  // Full workspace control
    WSRoleEditor WorkspaceRole = "editor" // Create/edit content
    WSRoleViewer WorkspaceRole = "viewer" // Read-only
)

type WorkspaceMembership struct {
    ID          jsonldb.ID
    UserID      jsonldb.ID
    WorkspaceID jsonldb.ID
    Role        WorkspaceRole
    Settings    WorkspaceMemberSettings // Notifications, etc.
    Created     time.Time
}

type WorkspaceMemberSettings struct {
    Notifications bool
}
```

### Invitation Entities

```go
type OrganizationInvitation struct {
    ID             jsonldb.ID
    OrganizationID jsonldb.ID
    Email          string
    Role           OrganizationRole
    Token          string
    InvitedBy      jsonldb.ID
    Created        time.Time
    Expires        time.Time
}

type WorkspaceInvitation struct {
    ID          jsonldb.ID
    WorkspaceID jsonldb.ID
    Email       string
    Role        WorkspaceRole
    Token       string
    InvitedBy   jsonldb.ID
    Created     time.Time
    Expires     time.Time
}
```

### User Entity Updates

```go
type User struct {
    ID              jsonldb.ID
    Email           string
    Name            string
    IsGlobalAdmin   bool
    OAuthIdentities []OAuthIdentity
    Quotas          UserQuotas
    Settings        UserSettings
    Created         time.Time
    Modified        time.Time
}

type UserQuotas struct {
    MaxOrganizations int // Max orgs user can create (default: 3)
}
```

---

## RBAC Permission Model

### Role Hierarchy & Permissions

```
ORGANIZATION LEVEL
┌─────────────────────────────────────────────────────────────────┐
│ Owner                                                           │
│ └─ All admin permissions PLUS:                                  │
│    • Manage billing & subscription                              │
│    • Transfer ownership                                         │
│    • Delete organization                                        │
├─────────────────────────────────────────────────────────────────┤
│ Admin                                                           │
│ └─ All member permissions PLUS:                                 │
│    • Create/delete workspaces                                   │
│    • Invite/remove organization members                         │
│    • Change member roles (except owner)                         │
│    • Configure organization settings                            │
│    • Access all workspaces as admin                             │
├─────────────────────────────────────────────────────────────────┤
│ Member                                                          │
│ └─ • View organization details                                  │
│    • Access workspaces where explicitly granted                 │
│    • Leave organization                                         │
└─────────────────────────────────────────────────────────────────┘

WORKSPACE LEVEL
┌─────────────────────────────────────────────────────────────────┐
│ Admin                                                           │
│ └─ All editor permissions PLUS:                                 │
│    • Invite/remove workspace members                            │
│    • Change member roles                                        │
│    • Configure workspace settings                               │
│    • Manage git remote                                          │
│    • Delete workspace (if org admin/owner)                      │
├─────────────────────────────────────────────────────────────────┤
│ Editor                                                          │
│ └─ All viewer permissions PLUS:                                 │
│    • Create/edit/delete pages                                   │
│    • Create/edit/delete tables & records                        │
│    • Upload/delete assets                                       │
├─────────────────────────────────────────────────────────────────┤
│ Viewer                                                          │
│ └─ • View pages, tables, records                                │
│    • View assets                                                │
│    • View workspace settings (read-only)                        │
└─────────────────────────────────────────────────────────────────┘
```

### Implicit Permissions

Organization admins/owners get **implicit admin access** to all workspaces in their organization:

```go
func GetEffectiveWorkspaceRole(user User, ws Workspace, orgMembership OrganizationMembership, wsMembership *WorkspaceMembership) WorkspaceRole {
    // Org owners and admins are implicitly workspace admins
    if orgMembership.Role == OrgRoleOwner || orgMembership.Role == OrgRoleAdmin {
        return WSRoleAdmin
    }

    // Otherwise use explicit workspace membership
    if wsMembership != nil {
        return wsMembership.Role
    }

    // No access
    return ""
}
```

---

## API Design

### URL Structure

```
# Organizations
GET    /api/orgs                           # List user's organizations
POST   /api/orgs                           # Create organization
GET    /api/orgs/{orgID}                   # Get organization details
PATCH  /api/orgs/{orgID}                   # Update organization
DELETE /api/orgs/{orgID}                   # Delete organization (owner only)

# Organization Members
GET    /api/orgs/{orgID}/members           # List organization members
POST   /api/orgs/{orgID}/members           # Add member (via invitation)
PATCH  /api/orgs/{orgID}/members/{userID}  # Update member role
DELETE /api/orgs/{orgID}/members/{userID}  # Remove member

# Organization Invitations
GET    /api/orgs/{orgID}/invitations       # List pending invitations
POST   /api/orgs/{orgID}/invitations       # Create invitation
DELETE /api/orgs/{orgID}/invitations/{id}  # Cancel invitation

# Workspaces
GET    /api/orgs/{orgID}/workspaces        # List workspaces in org
POST   /api/orgs/{orgID}/workspaces        # Create workspace
GET    /api/workspaces/{wsID}              # Get workspace details
PATCH  /api/workspaces/{wsID}              # Update workspace
DELETE /api/workspaces/{wsID}              # Delete workspace

# Workspace Members
GET    /api/workspaces/{wsID}/members           # List workspace members
POST   /api/workspaces/{wsID}/members           # Add member
PATCH  /api/workspaces/{wsID}/members/{userID}  # Update member role
DELETE /api/workspaces/{wsID}/members/{userID}  # Remove member

# Workspace Invitations
GET    /api/workspaces/{wsID}/invitations       # List pending invitations
POST   /api/workspaces/{wsID}/invitations       # Create invitation
DELETE /api/workspaces/{wsID}/invitations/{id}  # Cancel invitation

# Workspace Content (pages, tables, etc.)
GET    /api/workspaces/{wsID}/pages             # List pages
POST   /api/workspaces/{wsID}/pages             # Create page
GET    /api/workspaces/{wsID}/pages/{pageID}    # Get page
# ... etc for all content operations

# Workspace Git
GET    /api/workspaces/{wsID}/git               # Get git remote config
POST   /api/workspaces/{wsID}/git               # Configure git remote
POST   /api/workspaces/{wsID}/git/push          # Push to remote
DELETE /api/workspaces/{wsID}/git               # Remove git remote

# Auth
POST   /api/auth/login                     # Login
POST   /api/auth/register                  # Register
POST   /api/auth/switch-org                # Switch active organization
POST   /api/auth/switch-workspace          # Switch active workspace
GET    /api/auth/me                        # Get current user with memberships
POST   /api/auth/invitations/accept        # Accept any invitation (org or ws)

# Admin (global admin only)
GET    /api/admin/stats                    # Server stats
GET    /api/admin/organizations            # All organizations
GET    /api/admin/users                    # All users
```

### Key DTOs

```go
// Responses
type OrganizationResponse struct {
    ID             string                    `json:"id"`
    Name           string                    `json:"name"`
    BillingEmail   string                    `json:"billing_email,omitempty"` // Only for owner
    Quotas         OrganizationQuotasDTO     `json:"quotas"`
    Settings       OrganizationSettingsDTO   `json:"settings"`
    MemberCount    int                       `json:"member_count"`
    WorkspaceCount int                       `json:"workspace_count"`
    Created        time.Time                 `json:"created"`
}

type WorkspaceResponse struct {
    ID             string                `json:"id"`
    OrganizationID string                `json:"organization_id"`
    Name           string                `json:"name"`
    Slug           string                `json:"slug"`
    Quotas         WorkspaceQuotasDTO    `json:"quotas"`
    Settings       WorkspaceSettingsDTO  `json:"settings"`
    GitRemote      *GitRemoteDTO         `json:"git_remote,omitempty"`
    MemberCount    int                   `json:"member_count"`
    Created        time.Time             `json:"created"`
}

type UserResponse struct {
    ID              string                         `json:"id"`
    Email           string                         `json:"email"`
    Name            string                         `json:"name"`
    IsGlobalAdmin   bool                           `json:"is_global_admin,omitempty"`
    OAuthIdentities []OAuthIdentityDTO             `json:"oauth_identities"`
    Settings        UserSettingsDTO                `json:"settings"`

    // Current context
    OrganizationID  string                         `json:"organization_id"`     // Active org
    OrgRole         string                         `json:"org_role"`            // Role in active org
    WorkspaceID     string                         `json:"workspace_id"`        // Active workspace
    WorkspaceRole   string                         `json:"workspace_role"`      // Role in active ws

    // All memberships
    Organizations   []OrganizationMembershipDTO    `json:"organizations"`
    Workspaces      []WorkspaceMembershipDTO       `json:"workspaces"`
}

type OrganizationMembershipDTO struct {
    OrganizationID   string    `json:"organization_id"`
    OrganizationName string    `json:"organization_name"`
    Role             string    `json:"role"`
    Created          time.Time `json:"created"`
}

type WorkspaceMembershipDTO struct {
    WorkspaceID      string    `json:"workspace_id"`
    WorkspaceName    string    `json:"workspace_name"`
    OrganizationID   string    `json:"organization_id"`
    Role             string    `json:"role"`
    Created          time.Time `json:"created"`
}

// Requests
type CreateOrganizationRequest struct {
    Name                  string `json:"name" validate:"required,min=1,max=100"`
    CreateDefaultWorkspace bool   `json:"create_default_workspace"` // Default: true
    WorkspaceName         string `json:"workspace_name"`            // Default: "Main"
}

type CreateWorkspaceRequest struct {
    Name              string `json:"name" validate:"required,min=1,max=100"`
    Slug              string `json:"slug" validate:"omitempty,slug"`  // Auto-generated if empty
    CreateWelcomePage bool   `json:"create_welcome_page"`             // Default: true
}

type InviteOrgMemberRequest struct {
    Email string `json:"email" validate:"required,email"`
    Role  string `json:"role" validate:"required,oneof=admin member"`
}

type InviteWorkspaceMemberRequest struct {
    Email string `json:"email" validate:"required,email"`
    Role  string `json:"role" validate:"required,oneof=admin editor viewer"`
}
```

---

## File Changes

### Backend - Models (Priority 1)

| File | Action | Description |
|------|--------|-------------|
| `backend/internal/storage/identity/organization.go` | **Rewrite** | Split into Organization (new) and Workspace (current org logic) |
| `backend/internal/storage/identity/workspace.go` | **Create** | New file for Workspace entity and service |
| `backend/internal/storage/identity/membership.go` | **Rewrite** | Split into OrganizationMembership and WorkspaceMembership |
| `backend/internal/storage/identity/org_membership.go` | **Create** | New file for OrganizationMembership |
| `backend/internal/storage/identity/invitation.go` | **Rewrite** | Split into OrgInvitation and WorkspaceInvitation |
| `backend/internal/storage/identity/user.go` | **Update** | Update UserQuotas |

### Backend - DTOs (Priority 2)

| File | Action | Description |
|------|--------|-------------|
| `backend/internal/server/dto/types.go` | **Update** | Add OrganizationRole, WorkspaceRole |
| `backend/internal/server/dto/request.go` | **Update** | Add all new request types |
| `backend/internal/server/dto/response.go` | **Update** | Add OrganizationResponse, WorkspaceResponse, update UserResponse |

### Backend - Handlers (Priority 3)

| File | Action | Description |
|------|--------|-------------|
| `backend/internal/server/handlers/organizations.go` | **Rewrite** | Organization-level operations only |
| `backend/internal/server/handlers/workspaces.go` | **Create** | Workspace CRUD operations |
| `backend/internal/server/handlers/org_members.go` | **Create** | Organization member management |
| `backend/internal/server/handlers/ws_members.go` | **Create** | Workspace member management |
| `backend/internal/server/handlers/org_invitations.go` | **Create** | Organization invitations |
| `backend/internal/server/handlers/ws_invitations.go` | **Create** | Workspace invitations |
| `backend/internal/server/handlers/auth.go` | **Update** | Update context switching, user response |
| `backend/internal/server/handlers/pages.go` | **Update** | Change orgID to wsID |
| `backend/internal/server/handlers/tables.go` | **Update** | Change orgID to wsID |
| `backend/internal/server/handlers/records.go` | **Update** | Change orgID to wsID |
| `backend/internal/server/handlers/assets.go` | **Update** | Change orgID to wsID |
| `backend/internal/server/handlers/git_remotes.go` | **Update** | Change orgID to wsID |
| `backend/internal/server/handlers/convert.go` | **Update** | Add conversion functions |

### Backend - Routing & Middleware (Priority 4)

| File | Action | Description |
|------|--------|-------------|
| `backend/internal/server/router.go` | **Rewrite** | New route structure |
| `backend/internal/server/handler_wrapper.go` | **Update** | Two-level auth wrapper |
| `backend/internal/server/middleware.go` | **Update** | Two-level permission checking |

### Backend - Storage (Priority 5)

| File | Action | Description |
|------|--------|-------------|
| `backend/internal/storage/content/filestore.go` | **Update** | orgID → wsID throughout |

### Frontend (Priority 6)

| File | Action | Description |
|------|--------|-------------|
| `frontend/src/App.tsx` | **Update** | Two-level context handling |
| `frontend/src/components/OrgMenu.tsx` | **Rename/Split** | OrgSwitcher + WorkspaceSwitcher |
| `frontend/src/components/WorkspaceSettings.tsx` | **Split** | OrgSettings + WorkspaceSettings |
| `frontend/src/components/CreateOrgModal.tsx` | **Update** | Organization creation flow |
| `frontend/src/components/CreateWorkspaceModal.tsx` | **Create** | Workspace creation |
| `frontend/src/useApi.ts` | **Update** | Workspace-scoped API |
| `frontend/src/i18n/dictionaries/*.ts` | **Update** | New terminology |

### Tests (Priority 7)

| File | Action |
|------|--------|
| All `*_test.go` files in handlers/ | Update for new API |
| All `*_test.go` files in identity/ | Update for new models |
| `integration_test.go` | Rewrite for new flow |
| `isolation_test.go` | Update for new RBAC |

---

## Implementation Order

### Phase 1: Data Layer
1. Create new Organization model
2. Rename existing Organization → Workspace
3. Create OrganizationMembership model
4. Rename Membership → WorkspaceMembership
5. Update invitation models
6. Update User model
7. Write unit tests

### Phase 2: DTO Layer
1. Add new role types
2. Add Organization DTOs
3. Update Workspace DTOs (rename from Organization)
4. Update UserResponse with two-level memberships
5. Add all request/response types

### Phase 3: Handler Layer
1. Create organization handlers
2. Create workspace handlers
3. Split member management handlers
4. Split invitation handlers
5. Update content handlers (pages, tables, etc.)
6. Update auth handlers

### Phase 4: Routing & Middleware
1. Update permission checking for two levels
2. Update auth wrapper for two-level context
3. Implement new route structure
4. Wire up all handlers

### Phase 5: Frontend
1. Regenerate TypeScript types
2. Update App.tsx context
3. Create OrgSwitcher component
4. Update WorkspaceSwitcher (from OrgMenu)
5. Split settings components
6. Update all API calls
7. Update i18n

### Phase 6: Testing & Cleanup
1. Update all tests
2. Manual testing
3. Remove dead code
4. Documentation

---

## User Flows

### New User Registration

```
1. User signs up with email/OAuth
2. System creates User
3. User lands on "Create Organization" screen
4. User enters organization name
5. System creates:
   - Organization (user as owner)
   - Default workspace "Main" (user as admin)
   - Welcome page in workspace
6. User redirected to workspace
```

### Existing User Creates New Workspace

```
1. User clicks "Create Workspace" in org menu
2. User enters workspace name
3. System creates workspace (user as admin)
4. User switched to new workspace
```

### Inviting to Organization vs Workspace

```
Organization Invite:
- Invitee gets org member role (admin/member)
- If admin: automatic access to all workspaces
- If member: no workspace access until explicitly granted

Workspace Invite:
- Inviter must be org member OR ws admin
- If invitee not in org: also adds as org member
- Invitee gets workspace role (admin/editor/viewer)
```

### Context Switching

```
Current Context = (Organization, Workspace)

Switch Organization:
1. User selects different org
2. System updates active org
3. System selects first/last workspace in new org
4. UI refreshes with new context

Switch Workspace:
1. User selects different workspace (within same org)
2. System updates active workspace
3. UI refreshes with new content
```

---

## Migration Notes

Since we're not caring about backward compatibility:

1. **Wipe existing data** or create migration script that:
   - Creates one Organization per existing "Organization"
   - Renames existing Organization to Workspace
   - Promotes existing memberships appropriately

2. **Database cleanup**:
   - All JSONL files will have new schemas
   - Indexes need rebuilding

3. **Frontend**:
   - Complete UI refresh
   - New component hierarchy

---

## Open Questions

1. **Billing scope**: Where does billing/subscription data live?
   - Recommendation: Organization level, with plan determining quotas

2. **Cross-org workspace sharing**: Can a workspace be shared across orgs?
   - Recommendation: No, keep it simple. Workspaces belong to one org.

3. **Personal orgs**: Should solo users have a "Personal" org?
   - Recommendation: Yes, auto-create on registration

4. **Workspace templates**: Should we support workspace templates?
   - Recommendation: Future enhancement, not in initial scope

5. **Audit logging**: Should we add audit logs for org/ws changes?
   - Recommendation: Yes, at organization level
