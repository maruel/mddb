# Frontend First-Login Implementation

## Frontend Flow
Implemented in `frontend/src/App.tsx`:
1. **Load User Data**: Fetch profile via `GET /api/auth/me`.
2. **Organization Check**: If `user.organizations` is empty → Auto-create organization with default name.
3. **Workspace Check**: If first org has no workspaces → Auto-create workspace with default name.

## Auto-Creation Details

When a user has no organization or workspace, the system automatically creates them using the user's first name with localized templates:
- Organization: Localized "{name}'s Organization" (e.g., "John's Organization" in English, "Organisation de John" in French)
- Workspace: Localized "{name}'s Workspace" (e.g., "John's Workspace" in English, "Espace de John" in French)

If the user's name is not available, falls back to localized "My Organization" and "Main".

This provides a seamless onboarding experience without requiring manual input.

## Renaming

Users can rename organizations and workspaces in Settings → Workspace:
- **Organization Name**: Field in the Workspace settings tab
- **Workspace Name**: Field in the Workspace settings tab (requires admin role)

## Key Functions

- `autoCreateOrganization()`: Creates organization without prompting
- `autoCreateWorkspace()`: Creates workspace without prompting
- `createOrganization(data)`: API call to create organization (also used for additional orgs)
- `createWorkspace(data)`: API call to create workspace (also used for additional workspaces)

## Git Setup

Git remote synchronization can be configured in Settings → Sync tab.
