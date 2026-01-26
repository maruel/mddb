# Frontend First-Login Implementation

## Frontend Flow
Implemented in `frontend/src/App.tsx`:
1. **Load User Data**: Fetch profile via `GET /api/auth/me`.
2. **Organization Check**: If `user.organizations` is empty → Show `CreateOrgModal`.
3. **Workspace Check**: If first org has no workspaces → Show `CreateWorkspaceModal`.
