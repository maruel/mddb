# First-Time Login Flow

## Overview
When a user logs in for the first time, the frontend automatically checks for an existing organization and workspace. If missing, the system automatically creates them with default names for a seamless onboarding experience.

## Implementation

### Frontend Flow
Implemented in `frontend/src/App.tsx`:
1. **Load User Data**: Fetch profile via `GET /api/v1/auth/me`.
2. **Organization Check**: If `user.organizations` is empty → Auto-create organization with default name.
3. **Workspace Check**: If first org has no workspaces → Auto-create workspace with default name.

The auto-creation happens silently without prompting the user, using the user's first name:
- Organization: "{Name}'s Organization" (localized, e.g., "John's Organization")
- Workspace: "{Name}'s Workspace" (localized, e.g., "John's Workspace")

Falls back to "My Organization" / "Main" if the user's name is unavailable.

Users can rename organizations and workspaces later in Settings → Workspace.

For more frontend details, see [Frontend Implementation](../frontend/docs/FIRST_LOGIN_FLOW.md).

### Backend Behavior
The backend supports this flow via:
- `POST /api/v1/organizations` - Create organization
- `POST /api/v1/organizations/{orgID}/workspaces` - Create workspace
- `POST /api/v1/workspaces/{wsID}` - Rename workspace

When a workspace is created (either during onboarding or manually):
- **Permissions**: The creating user is automatically assigned the `admin` role for that workspace.
- **Git**: A local Git repository is initialized for the workspace data.

## User Experience

### Scenario 1: New User (No Organization)
1. User logs in.
2. Organization and workspace are auto-created with personalized names.
3. User proceeds directly to the main application.

### Scenario 2: User with Organization but No Workspace
1. User logs in (e.g., accepted invitation but no workspace exists).
2. Workspace is auto-created with personalized name.
3. User proceeds directly to the main application.

### Scenario 3: Returning User
1. User logs in.
2. Both org and workspace exist; proceed to main application.

## Renaming Organizations and Workspaces

Both organization and workspace names can be changed in Settings → Workspace tab:
- **Organization Name**: Updates the organization's display name.
- **Workspace Name**: Updates the workspace's display name (requires admin role).
