# First-Time Login Flow

## Overview
When a user logs in for the first time, the frontend automatically checks for an existing organization and workspace. If missing, onboarding modals guide the user through creation.

## Implementation

### Frontend Flow
Implemented in `frontend/src/App.tsx`:
1. **Load User Data**: Fetch profile via `GET /api/auth/me`.
2. **Organization Check**: If `user.organizations` is empty → Show `CreateOrgModal`.
3. **Workspace Check**: If first org has no workspaces → Show `CreateWorkspaceModal`.

For more frontend details, see [Frontend Implementation](../frontend/docs/FIRST_LOGIN_FLOW.md).

### Backend Behavior
The backend supports this flow via the `POST /api/organizations/{orgID}/workspaces` endpoint.
When a workspace is created (either during onboarding or manually):
- **Permissions**: The creating user is automatically assigned the `admin` role for that workspace.
- **Git**: A local Git repository is initialized for the workspace data.

## User Experience

### Scenario 1: New User (No Organization)
1. User logs in.
2. `CreateOrgModal` appears.
3. User creates organization (which automatically gets a "Main" workspace).
4. User is switched to the new org and sees the Git setup prompt.

### Scenario 2: User with Organization but No Workspace
1. User logs in (e.g., accepted invitation but no workspace exists).
2. `CreateWorkspaceModal` appears.
3. User creates workspace and is switched to it.
4. User sees the Git setup prompt.

### Scenario 3: Returning User
1. User logs in.
2. Both org and workspace exist; skip modals.
3. Proceed to main application.
