# First-Time Login Flow

## Overview

When a user logs in for the first time, the frontend automatically checks that the user has both an organization and a workspace. If either is missing, appropriate onboarding modals are shown in sequence.

## Implementation

### Frontend Flow

The first-time login check is implemented in `frontend/src/App.tsx` with the following logic:

1. **Load User Data**: When a user authenticates, their user profile is fetched via `GET /api/auth/me`, which includes:
   - `organizations` - List of organizations the user is a member of (`OrgMembershipResponse[]`)
   - `workspaces` - List of workspaces the user has access to (`WSMembershipResponse[]`)

2. **Organization Check**: After user data is loaded, check if `user.organizations` is empty:
   - If empty → Show `CreateOrgModal` (can't proceed without an organization)
   - If not empty → Proceed to workspace check

3. **Workspace Check**: For the first organization in the list, check if the user has any workspaces in that organization:
   - If first org has no workspaces → Show `CreateWorkspaceModal` (can't proceed without a workspace)
   - If first org has workspaces → Proceed normally

### Components

#### CreateWorkspaceModal (`frontend/src/components/CreateWorkspaceModal.tsx`)

New component similar to `CreateOrgModal`, allowing users to create a new workspace within their organization.

**Props:**
- `onClose: () => void` - Called when modal is dismissed (only for non-first workspaces)
- `onCreate: (data: CreateWorkspaceData) => Promise<void>` - Called to create the workspace
- `isFirstWorkspace?: boolean` - When true, makes the modal non-dismissible

**Flow:**
1. Accept workspace name input
2. Call `onCreate()` with the workspace name
3. The parent component (`App.tsx`) handles API call, user data refresh, and workspace switch

### Backend API Changes

#### New Endpoint

**Route:** `POST /api/organizations/{orgID}/workspaces`

**Request Type:** `CreateWorkspaceRequest`
```go
type CreateWorkspaceRequest struct {
	OrgID jsonldb.ID `path:"orgID" tstype:"-"`
	Name  string     `json:"name"`
}
```

**Response Type:** `WorkspaceResponse`

**Handler:** `OrganizationHandler.CreateWorkspace()`

**Actions:**
1. Create new workspace in organization
2. Create workspace membership (user becomes admin)
3. Initialize workspace storage
4. **Automatically create welcome page** with default content:
   - Title: "Welcome"
   - Content: "# Welcome to mddb\n\nThis is your new workspace. You can create pages, tables, and upload assets here."
5. Return workspace details

**Authorization:** Organization admin role required

#### Handler Implementation

`backend/internal/server/handlers/organizations.go`:

```go
func (h *OrganizationHandler) CreateWorkspace(
	ctx context.Context,
	orgID jsonldb.ID,
	user *identity.User,
	req *dto.CreateWorkspaceRequest,
) (*dto.WorkspaceResponse, error)
```

### Frontend API Changes

#### Generated Types

- `CreateWorkspaceRequest` - Request to create workspace in organization

#### Generated API Client

```typescript
org(orgID: string).workspaces.create(options: CreateWorkspaceRequest) => WorkspaceResponse
```

### Internationalization

New strings added to `i18n/types.ts` and all dictionary files:

```typescript
createWorkspace: {
  title: string;
  description: string;
  firstWorkspaceTitle: string;
  firstWorkspaceDescription: string;
  nameLabel: string;
  namePlaceholder: string;
  create: string;
}
```

Translations provided for: English (en), French (fr), German (de), Spanish (es)

**Note:** The welcome page content is now hardcoded in the backend and not customizable via frontend fields, ensuring consistency across all first-time workspace creation.

## User Experience

### Scenario 1: New User (No Organization)
1. User logs in
2. First-login check runs: no organizations found
3. `CreateOrgModal` appears with message: "Create Your Workspace"
4. User enters organization name and clicks "Create"
5. Organization is created with default "Main" workspace
6. User is automatically switched to the new organization
7. Git setup prompt appears (existing flow)

### Scenario 2: User with Organization but No Workspace
1. User logs in (e.g., accepted org invitation but workspace was deleted)
2. First-login check runs: organizations found, but no workspaces in first org
3. `CreateWorkspaceModal` appears with message: "Create Your First Workspace"
4. User enters workspace name and clicks "Create"
5. Workspace is created and user is switched to it
6. Git setup prompt appears

### Scenario 3: User with Organization and Workspace
1. User logs in
2. First-login check runs: both organization and workspace exist
3. No modal shown, proceed to main app

## Files Modified

### Frontend
- `frontend/src/App.tsx` - Added first-login check and createWorkspace function, simplified createOrganization to not pass welcome page content
- `frontend/src/components/CreateOrgModal.tsx` - Simplified to only ask for organization name, removed welcome page fields
- `frontend/src/components/CreateWorkspaceModal.tsx` - New component for creating workspaces
- `frontend/src/i18n/types.ts` - Added createWorkspace strings, removed welcome page content strings from createOrg
- `frontend/src/i18n/dictionaries/en.ts` - English translations (removed welcome content)
- `frontend/src/i18n/dictionaries/fr.ts` - French translations (removed welcome content)
- `frontend/src/i18n/dictionaries/de.ts` - German translations (removed welcome content)
- `frontend/src/i18n/dictionaries/es.ts` - Spanish translations (removed welcome content)

### Backend
- `backend/internal/server/dto/request.go` - Removed welcome page fields from `CreateOrganizationRequest`, added `CreateWorkspaceRequest` type
- `backend/internal/server/handlers/auth.go` - Updated to automatically create welcome page when organization is created
- `backend/internal/server/handlers/organizations.go` - Added `CreateWorkspace` handler that automatically creates welcome page
- `backend/internal/server/router.go` - Added route for workspace creation endpoint

### Generated
- `frontend/src/types.gen.ts` - Generated TypeScript types (including `CreateWorkspaceRequest`)
- `frontend/src/api.gen.ts` - Generated API client (including `org().workspaces.create()`)

## Testing

To test the first-login flow:

1. **New User (No Org):**
   - Create new account
   - Should see CreateOrgModal on first login
   - Create organization with name "Test Org"
   - Should be switched to "Main" workspace
   - Should see git setup prompt

2. **User with Org Only:**
   - Delete all workspaces for user's first organization
   - Login
   - Should see CreateWorkspaceModal
   - Create workspace
   - Should be switched to new workspace
   - Should see git setup prompt

3. **User with Both:**
   - Login with user who has org and workspace
   - Should skip to main app (no modals)
