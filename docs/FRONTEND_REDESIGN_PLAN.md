# Frontend Architecture Redesign

## Overview

This document outlines the frontend redesign to support the new **Organization → Workspace** two-level hierarchy. The frontend uses Solid.js with TypeScript.

**Breaking Change**: Complete redesign, no backward compatibility.

---

## Current State

| Aspect | Current | Target |
|--------|---------|--------|
| Hierarchy | Single "Organization" | Organization → Workspace |
| Context | `organization_id` only | `organization_id` + `workspace_id` |
| Switching | Org switcher only | Org switcher + Workspace switcher |
| URL structure | `/:orgId/:nodeId+slug` | `/:wsId/:nodeId+slug` |
| Settings | Combined org/workspace | Separate org settings + workspace settings |

---

## Component Architecture

### New Component Hierarchy

```
App
├── Auth (unchanged)
├── Header
│   ├── OrgSwitcher (new)
│   ├── WorkspaceSwitcher (renamed from OrgMenu)
│   └── UserMenu (minor updates)
├── Sidebar
│   ├── SidebarNode (unchanged)
│   └── CreateNodeButton (unchanged)
├── MainContent
│   ├── EditorPane (unchanged)
│   └── TableViews (unchanged)
├── Modals
│   ├── CreateOrgModal (new - for organizations)
│   ├── CreateWorkspaceModal (renamed from CreateOrgModal)
│   ├── OrgSettingsModal (new)
│   └── WorkspaceSettingsModal (split from WorkspaceSettings)
└── Onboarding (updated flow)
```

### Component Responsibility Matrix

| Component | Organization | Workspace | Content |
|-----------|--------------|-----------|---------|
| OrgSwitcher | ✓ Switch | - | - |
| WorkspaceSwitcher | - | ✓ Switch | - |
| OrgSettingsModal | ✓ Settings/Members | - | - |
| WorkspaceSettingsModal | - | ✓ Settings/Members | - |
| Sidebar | - | - | ✓ Navigate |
| EditorPane | - | - | ✓ Edit |

---

## State Management

### Context Providers

Create dedicated context providers for clean separation:

```typescript
// src/contexts/AuthContext.tsx
interface AuthContextValue {
  user: Accessor<UserResponse | null>;
  token: Accessor<string | null>;
  login: (token: string) => Promise<void>;
  logout: () => void;
  refreshUser: () => Promise<void>;
}

// src/contexts/OrgContext.tsx
interface OrgContextValue {
  organization: Accessor<OrganizationResponse | null>;
  orgRole: Accessor<OrganizationRole | null>;
  switchOrg: (orgId: string) => Promise<void>;
  organizations: Accessor<OrganizationMembershipDTO[]>;
}

// src/contexts/WorkspaceContext.tsx
interface WorkspaceContextValue {
  workspace: Accessor<WorkspaceResponse | null>;
  wsRole: Accessor<WorkspaceRole | null>;
  switchWorkspace: (wsId: string) => Promise<void>;
  workspaces: Accessor<WorkspaceMembershipDTO[]>;
}
```

### State Flow

```
User Login
    │
    ▼
Load User (includes all memberships)
    │
    ├─► Load Organizations (from memberships)
    │       │
    │       ▼
    │   Set Active Org (from user.organization_id or first)
    │       │
    │       ▼
    │   Load Workspaces (for active org)
    │       │
    │       ▼
    │   Set Active Workspace (from user.workspace_id or first)
    │       │
    │       ▼
    └─► Load Nodes (from active workspace)
            │
            ▼
        Render Content
```

### Derived State

```typescript
// Computed permissions
const canManageOrg = createMemo(() =>
  orgRole() === 'owner' || orgRole() === 'admin'
);

const canManageWorkspace = createMemo(() =>
  canManageOrg() || wsRole() === 'admin'
);

const canEditContent = createMemo(() =>
  canManageWorkspace() || wsRole() === 'editor'
);

const isViewer = createMemo(() =>
  wsRole() === 'viewer' && !canManageOrg()
);
```

---

## URL Structure

### Route Patterns

```
/                                     # Landing/dashboard
/login                                # Auth page
/privacy, /terms                      # Static pages

# Organization-scoped (admin only)
/org/:orgId/settings                  # Organization settings

# Workspace-scoped (workspace ID is globally unique)
/:wsId                                # Workspace root (redirects to first node)
/:wsId/:nodeId+slug                   # Node view with SEO slug (e.g., /ws123/node456-my-page)
/:wsId/settings                       # Workspace settings
```

### URL Canonicalization

```typescript
// Generate canonical URL (workspace ID is globally unique, no org needed)
function getNodeUrl(ws: WorkspaceResponse, node: NodeResponse): string {
  const slug = slugify(node.title);
  return `/${ws.id}/${node.id}-${slug}`;
}

// Parse URL - nodeId+slug combined in one segment
function parseRoute(path: string): RouteParams {
  const [, wsId, nodeIdSlug] = path.split('/');
  const nodeId = nodeIdSlug?.split('-')[0];  // Extract ID from "node456-my-page"
  return { wsId, nodeId };
}
```

---

## UI/UX Design

### Header Layout

```
┌─────────────────────────────────────────────────────────────────────┐
│ [Logo]  [OrgSwitcher ▼]  /  [WorkspaceSwitcher ▼]    [🔔] [UserMenu ▼] │
└─────────────────────────────────────────────────────────────────────┘
```

### OrgSwitcher Component

```
┌─────────────────────────┐
│ Acme Corp           ▼   │
└─────────────────────────┘
        │
        ▼
┌─────────────────────────┐
│ ● Acme Corp      owner  │  ← Current
│   StartupXYZ     admin  │
│   Personal       owner  │
├─────────────────────────┤
│ + Create Organization   │
└─────────────────────────┘
```

### WorkspaceSwitcher Component

```
┌─────────────────────────┐
│ Engineering Docs    ▼   │
└─────────────────────────┘
        │
        ▼
┌─────────────────────────┐
│ ● Engineering Docs admin│  ← Current
│   Marketing        editor│
│   HR Policies      viewer│
├─────────────────────────┤
│ + Create Workspace      │  ← Only if org admin/owner
└─────────────────────────┘
```

### Settings Navigation

**Organization Settings** (org admin/owner only):
```
┌─────────────────────────────────────────────────────────────────┐
│ [X] Organization Settings                                        │
├─────────────────────────────────────────────────────────────────┤
│ General │ Members │ Workspaces │ Billing                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Organization Name                                               │
│  ┌──────────────────────────────────────────────┐                │
│  │ Acme Corp                                    │                │
│  └──────────────────────────────────────────────┘                │
│                                                                  │
│  Billing Email                                                   │
│  ┌──────────────────────────────────────────────┐                │
│  │ billing@acme.com                             │                │
│  └──────────────────────────────────────────────┘                │
│                                                                  │
│  Allowed Email Domains                                           │
│  ┌──────────────────────────────────────────────┐                │
│  │ acme.com, acme.io                            │                │
│  └──────────────────────────────────────────────┘                │
│                                                                  │
│  [ ] Require SSO for all members                                 │
│                                                                  │
│                                    [Save Changes]                │
└─────────────────────────────────────────────────────────────────┘
```

**Workspace Settings** (ws admin):
```
┌─────────────────────────────────────────────────────────────────┐
│ [X] Workspace Settings                                           │
├─────────────────────────────────────────────────────────────────┤
│ General │ Members │ Git Sync │ Personal                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Workspace Name                                                  │
│  ┌──────────────────────────────────────────────┐                │
│  │ Engineering Docs                             │                │
│  └──────────────────────────────────────────────┘                │
│                                                                  │
│  URL Slug                                                        │
│  ┌──────────────────────────────────────────────┐                │
│  │ engineering-docs                             │                │
│  └──────────────────────────────────────────────┘                │
│                                                                  │
│  [ ] Allow public access (unauthenticated read)                  │
│                                                                  │
│                                    [Save Changes]                │
└─────────────────────────────────────────────────────────────────┘
```

### Member Management Differences

**Organization Members Tab:**
- Shows all org members with org-level roles (owner/admin/member)
- Org admins see all members
- Can invite to organization
- Role changes affect org-wide access

**Workspace Members Tab:**
- Shows workspace members with ws-level roles (admin/editor/viewer)
- Includes implicit members (org admins shown with badge)
- Can invite to workspace (also adds to org as member if needed)
- Role changes affect workspace access only

```
┌─────────────────────────────────────────────────────────────────┐
│ Workspace Members                                                │
├─────────────────────────────────────────────────────────────────┤
│ Name              Email                 Role         Actions    │
├─────────────────────────────────────────────────────────────────┤
│ Jane Smith        jane@acme.com         admin [ORG]    -        │  ← Implicit
│ John Doe          john@acme.com         editor         [▼][X]   │
│ Guest User        guest@ext.com         viewer         [▼][X]   │
├─────────────────────────────────────────────────────────────────┤
│ [+ Invite Member]                                                │
└─────────────────────────────────────────────────────────────────┘
```

### Onboarding Flow

```
New User Registration
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Welcome to MDDB!                               │
│                                                                  │
│   Let's set up your first organization.                          │
│                                                                  │
│   Organization Name                                              │
│   ┌──────────────────────────────────────────────┐               │
│   │ My Organization                              │               │
│   └──────────────────────────────────────────────┘               │
│                                                                  │
│   [✓] Create a default "Main" workspace                          │
│                                                                  │
│                              [Get Started →]                     │
└─────────────────────────────────────────────────────────────────┘
        │
        ▼
   Git Sync Setup (optional, skippable)
        │
        ▼
   Workspace Ready (Main workspace with welcome page)
```

---

## API Layer Changes

### Generated Types (from backend)

```typescript
// types.gen.ts (auto-generated, showing key additions)

// Roles
type OrganizationRole = 'owner' | 'admin' | 'member';
type WorkspaceRole = 'admin' | 'editor' | 'viewer';

// Responses
interface OrganizationResponse {
  id: string;
  name: string;
  slug: string;
  billing_email?: string;         // Only for owner
  quotas: OrganizationQuotas;
  settings: OrganizationSettings;
  member_count: number;
  workspace_count: number;
  created: string;
}

interface WorkspaceResponse {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  quotas: WorkspaceQuotas;
  settings: WorkspaceSettings;
  git_remote?: GitRemote;
  member_count: number;
  created: string;
}

interface UserResponse {
  id: string;
  email: string;
  name: string;
  is_global_admin?: boolean;
  settings: UserSettings;

  // Current context
  organization_id: string;
  org_role: OrganizationRole;
  workspace_id: string;
  workspace_role: WorkspaceRole;

  // All memberships
  organizations: OrganizationMembershipDTO[];
  workspaces: WorkspaceMembershipDTO[];
}

interface OrganizationMembershipDTO {
  organization_id: string;
  organization_name: string;
  organization_slug: string;
  role: OrganizationRole;
  created: string;
}

interface WorkspaceMembershipDTO {
  workspace_id: string;
  workspace_name: string;
  workspace_slug: string;
  organization_id: string;
  role: WorkspaceRole;
  created: string;
}
```

### API Client Updates

```typescript
// api.gen.ts structure (auto-generated)

interface Api {
  auth: {
    login(req: LoginRequest): Promise<LoginResponse>;
    register(req: RegisterRequest): Promise<LoginResponse>;
    me(): Promise<UserResponse>;
    switchOrg(req: SwitchOrgRequest): Promise<UserResponse>;
    switchWorkspace(req: SwitchWorkspaceRequest): Promise<UserResponse>;
    acceptInvitation(req: AcceptInvitationRequest): Promise<void>;
  };

  orgs: {
    list(): Promise<OrganizationResponse[]>;
    create(req: CreateOrganizationRequest): Promise<OrganizationResponse>;
    get(orgId: string): Promise<OrganizationResponse>;
    update(orgId: string, req: UpdateOrganizationRequest): Promise<OrganizationResponse>;
    delete(orgId: string): Promise<void>;

    members: {
      list(orgId: string): Promise<OrganizationMemberResponse[]>;
      update(orgId: string, userId: string, req: UpdateMemberRequest): Promise<void>;
      remove(orgId: string, userId: string): Promise<void>;
    };

    invitations: {
      list(orgId: string): Promise<OrganizationInvitationResponse[]>;
      create(orgId: string, req: InviteOrgMemberRequest): Promise<void>;
      cancel(orgId: string, inviteId: string): Promise<void>;
    };

    workspaces: {
      list(orgId: string): Promise<WorkspaceResponse[]>;
      create(orgId: string, req: CreateWorkspaceRequest): Promise<WorkspaceResponse>;
    };
  };

  workspaces: {
    get(wsId: string): Promise<WorkspaceResponse>;
    update(wsId: string, req: UpdateWorkspaceRequest): Promise<WorkspaceResponse>;
    delete(wsId: string): Promise<void>;

    members: {
      list(wsId: string): Promise<WorkspaceMemberResponse[]>;
      update(wsId: string, userId: string, req: UpdateMemberRequest): Promise<void>;
      remove(wsId: string, userId: string): Promise<void>;
    };

    invitations: {
      list(wsId: string): Promise<WorkspaceInvitationResponse[]>;
      create(wsId: string, req: InviteWorkspaceMemberRequest): Promise<void>;
      cancel(wsId: string, inviteId: string): Promise<void>;
    };

    nodes: {
      list(wsId: string): Promise<NodeResponse[]>;
      create(wsId: string, req: CreateNodeRequest): Promise<NodeResponse>;
      get(wsId: string, nodeId: string): Promise<NodeResponse>;
      // ... etc
    };

    git: {
      get(wsId: string): Promise<GitRemoteResponse>;
      update(wsId: string, req: GitRemoteRequest): Promise<void>;
      push(wsId: string): Promise<void>;
      delete(wsId: string): Promise<void>;
    };
  };
}
```

### useApi Hook Updates

```typescript
// useApi.ts

export function useApi() {
  const token = getToken();

  return {
    api: createApi(token),

    // Convenience wrappers for current context
    useOrg(orgId: string) {
      return {
        get: () => this.api.orgs.get(orgId),
        update: (req) => this.api.orgs.update(orgId, req),
        members: this.api.orgs.members,
        invitations: this.api.orgs.invitations,
        workspaces: this.api.orgs.workspaces,
      };
    },

    useWorkspace(wsId: string) {
      return {
        get: () => this.api.workspaces.get(wsId),
        update: (req) => this.api.workspaces.update(wsId, req),
        members: this.api.workspaces.members,
        invitations: this.api.workspaces.invitations,
        nodes: this.api.workspaces.nodes,
        git: this.api.workspaces.git,
      };
    },
  };
}
```

---

## File Changes

### New Files

| File | Description |
|------|-------------|
| `src/contexts/AuthContext.tsx` | Authentication context provider |
| `src/contexts/OrgContext.tsx` | Organization context provider |
| `src/contexts/WorkspaceContext.tsx` | Workspace context provider |
| `src/contexts/index.ts` | Context exports |
| `src/components/OrgSwitcher.tsx` | Organization switcher dropdown |
| `src/components/OrgSwitcher.module.css` | OrgSwitcher styles |
| `src/components/CreateOrgModal.tsx` | Create organization modal |
| `src/components/OrgSettingsModal.tsx` | Organization settings modal |
| `src/components/OrgSettingsModal.module.css` | OrgSettingsModal styles |
| `src/components/OrgMembersTab.tsx` | Organization members management |
| `src/components/OrgWorkspacesTab.tsx` | List/manage workspaces in org |
| `src/hooks/usePermissions.ts` | Permission check hooks |

### Modified Files

| File | Changes |
|------|---------|
| `src/App.tsx` | Context providers, routing, two-level switching |
| `src/App.module.css` | Header layout adjustments |
| `src/components/OrgMenu.tsx` | **Rename to WorkspaceSwitcher.tsx** |
| `src/components/OrgMenu.module.css` | **Rename to WorkspaceSwitcher.module.css** |
| `src/components/CreateOrgModal.tsx` | **Rename to CreateWorkspaceModal.tsx**, update terminology |
| `src/components/WorkspaceSettings.tsx` | Remove org-level settings, ws-only |
| `src/components/UserMenu.tsx` | Show current org/workspace context |
| `src/components/Onboarding.tsx` | Two-step onboarding flow |
| `src/useApi.ts` | Context wrappers, new endpoints |
| `src/i18n/types.ts` | New keys for org/workspace terminology |
| `src/i18n/dictionaries/en.ts` | English translations |
| `src/i18n/dictionaries/fr.ts` | French translations |
| `src/i18n/dictionaries/de.ts` | German translations |
| `src/i18n/dictionaries/es.ts` | Spanish translations |

### Deleted Files

| File | Reason |
|------|--------|
| - | No files deleted, all renamed/refactored |

---

## i18n Updates

### New Dictionary Keys

```typescript
interface Dictionary {
  // ... existing keys ...

  // Organization
  organization: {
    title: string;                    // "Organization"
    create: string;                   // "Create Organization"
    switch: string;                   // "Switch Organization"
    settings: string;                 // "Organization Settings"
    members: string;                  // "Organization Members"
    workspaces: string;               // "Workspaces"
    name: string;                     // "Organization Name"
    billingEmail: string;             // "Billing Email"
    allowedDomains: string;           // "Allowed Email Domains"
    requireSSO: string;               // "Require SSO for all members"
    deleteConfirm: string;            // "Are you sure? This will delete all workspaces."
  };

  organizationRoles: {
    owner: string;                    // "Owner"
    admin: string;                    // "Admin"
    member: string;                   // "Member"
  };

  // Workspace (update existing)
  workspace: {
    title: string;                    // "Workspace"
    create: string;                   // "Create Workspace"
    switch: string;                   // "Switch Workspace"
    settings: string;                 // "Workspace Settings"
    members: string;                  // "Workspace Members"
    name: string;                     // "Workspace Name"
    slug: string;                     // "URL Slug"
    publicAccess: string;             // "Allow public access"
    deleteConfirm: string;            // "Are you sure? This will delete all content."
    implicitAdmin: string;            // "Organization Admin"
  };

  workspaceRoles: {
    admin: string;                    // "Admin"
    editor: string;                   // "Editor"
    viewer: string;                   // "Viewer"
  };

  // Onboarding (update existing)
  onboarding: {
    welcome: string;                  // "Welcome to MDDB!"
    setupOrg: string;                 // "Let's set up your first organization."
    orgName: string;                  // "Organization Name"
    createDefaultWs: string;          // "Create a default 'Main' workspace"
    getStarted: string;               // "Get Started"
    setupGit: string;                 // "Set up Git synchronization"
    skipForNow: string;               // "Skip for now"
  };

  // Invitations
  invitations: {
    inviteToOrg: string;              // "Invite to Organization"
    inviteToWorkspace: string;        // "Invite to Workspace"
    pendingOrgInvites: string;        // "Pending Organization Invitations"
    pendingWsInvites: string;         // "Pending Workspace Invitations"
  };
}
```

---

## Implementation Phases

### Phase 1: Foundation (Context & State)

1. Create context providers
   - `AuthContext.tsx`
   - `OrgContext.tsx`
   - `WorkspaceContext.tsx`
2. Create permission hooks
   - `usePermissions.ts`
3. Update `App.tsx` to use contexts
4. Ensure backward compatibility during transition

### Phase 2: Organization Layer

1. Create `OrgSwitcher.tsx` component
2. Create `CreateOrgModal.tsx` (new, for organizations)
3. Create `OrgSettingsModal.tsx`
4. Create `OrgMembersTab.tsx`
5. Create `OrgWorkspacesTab.tsx`
6. Update routing for org-level pages

### Phase 3: Workspace Layer

1. Rename `OrgMenu.tsx` → `WorkspaceSwitcher.tsx`
2. Rename `CreateOrgModal.tsx` → `CreateWorkspaceModal.tsx`
3. Update `WorkspaceSettings.tsx` (remove org-level settings)
4. Update member management for workspace scope
5. Update git sync for workspace scope

### Phase 4: Integration

1. Update `App.tsx` for full two-level flow
2. Update URL routing (`/:wsId/:nodeId+slug`)
3. Update `UserMenu.tsx` to show context
4. Update `Onboarding.tsx` for new flow
5. Wire up all components

### Phase 5: Polish

1. Update all i18n dictionaries
2. Update CSS for new layout
3. Add loading states
4. Add error handling
5. Keyboard navigation for switchers

### Phase 6: Testing

1. Unit tests for new components
2. Integration tests for flows
3. Manual testing checklist
4. Accessibility audit

---

## Testing Checklist

### User Flows

- [ ] New user registration → org creation → workspace ready
- [ ] Login → correct org/workspace restored
- [ ] Switch organization → workspaces refresh
- [ ] Switch workspace → content refreshes
- [ ] Create organization (from switcher)
- [ ] Create workspace (from switcher)
- [ ] Invite member to organization
- [ ] Invite member to workspace
- [ ] Accept organization invitation
- [ ] Accept workspace invitation
- [ ] Update organization settings
- [ ] Update workspace settings
- [ ] Configure git sync (workspace level)
- [ ] Role-based UI restrictions

### Permissions

- [ ] Org owner sees billing settings
- [ ] Org admin can create workspaces
- [ ] Org member cannot create workspaces
- [ ] Org admin has implicit ws admin access
- [ ] Workspace editor can edit content
- [ ] Workspace viewer is read-only
- [ ] Implicit admin badge shows correctly

### Edge Cases

- [ ] User with single org/workspace (no switching needed)
- [ ] User with multiple orgs, single workspace each
- [ ] User with single org, multiple workspaces
- [ ] Deep link to specific node preserves context
- [ ] OAuth callback preserves intended destination
- [ ] Removed from workspace → redirect handled
- [ ] Removed from org → redirect handled

---

## Migration Notes

Since no backward compatibility is required:

1. **Regenerate types**: After backend changes, regenerate `types.gen.ts` and `api.gen.ts`
2. **Clear localStorage**: User context stored in localStorage will be invalid
3. **Update tests**: All component tests need updates for new props/context
4. **CSS cleanup**: Remove unused styles after renaming

---

## Open Design Questions

1. **Dashboard view**: Should there be a cross-workspace dashboard showing recent activity across all workspaces?
   - Recommendation: Future enhancement, not in initial scope.

2. **Workspace templates**: Should workspace creation support templates?
   - Recommendation: Future enhancement.

3. **Breadcrumb navigation**: Should the header show `Org > Workspace` breadcrumb style?
   - Recommendation: Use two separate dropdowns for clearer UX.

4. **Mobile layout**: How should org/workspace switching work on mobile?
   - Recommendation: Slide-out panel with both in sequence.

5. **Quick switcher**: Keyboard shortcut (Cmd+K) for quick org/workspace switching?
   - Recommendation: Nice to have, add in polish phase.
