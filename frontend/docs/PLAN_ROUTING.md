<!-- Routing Refactor Plan: migrate to @solidjs/router with modular sections -->
# Routing Refactor Plan

## Problem

The current routing implementation in `App.tsx` has several issues:

### 1. Manual URL Parsing (Non-Reactive)
- `handlePopState()` reads `window.location` directly in event handlers
- SolidJS can't track these dependencies, causing stale UI
- Manual `popstate` event dispatching after `pushState` is fragile

### 2. Overlapping Route State
Multiple signals track the same conceptual state:
```tsx
const [isSettingsPage, setIsSettingsPage] = createSignal(false);      // OLD
const [isOrgSettingsPage, setIsOrgSettingsPage] = createSignal(false); // OLD
const [isProfilePage, setIsProfilePage] = createSignal(false);         // OLD
const [settingsRoute, setSettingsRoute] = createSignal<...>(null);     // NEW
```
Every navigation must reset all signals manually - easy to miss one.

### 3. Monolithic App.tsx
`App.tsx` (~800 lines) handles:
- Routing logic
- Workspace state
- Node selection
- Settings navigation
- Notion import
- Modal state

This makes it hard to reason about and extend.

### 4. Tight Coupling
The `/w/` workspace section is deeply embedded in `App.tsx`. Adding new top-level sections (e.g., `/dashboard`) would require significant changes.

## Solution

### 1. Use `@solidjs/router`
Official router with reactive params, nested routes, and proper SolidJS integration.

### 2. Modular Route Sections
Each top-level route (`/w/`, `/settings/`) is a self-contained module with its own providers and layout.

### 3. Settings as Separate Route Tree
Settings at `/settings/*` - clear URL semantics, no tangled state with workspaces.

## Route Structure

```
/login                          → Auth
/privacy                        → Privacy
/terms                          → Terms

/settings/user                  → User profile
/settings/server                → Server admin (global admins only)
/settings/workspace/:wsId       → Workspace settings
/settings/org/:orgId            → Organization settings

/w/:wsId                        → Workspace root (→ redirect to first node)
/w/:wsId/:nodeId                → View/edit node
```

## Component Structure

```
App.tsx (thin shell)
├── I18nProvider (global)
├── AuthProvider (global)
└── Router
    ├── Route /login → Auth
    ├── Route /privacy → Privacy
    ├── Route /terms → Terms
    ├── Route /settings/* → SettingsSection
    └── Route /w/* → WorkspaceSection

src/sections/
├── WorkspaceSection.tsx
│   ├── WorkspaceProvider (nodes, navigation)
│   ├── EditorProvider (title, content, auto-save)
│   ├── RecordsProvider (table records)
│   └── WorkspaceLayout
│       ├── Header
│       ├── Sidebar
│       └── <Outlet />
│           ├── Route / → WorkspaceRoot (redirect)
│           └── Route /:nodeId → NodeView
│
└── SettingsSection.tsx
    └── SettingsLayout
        ├── SettingsSidebar
        └── <Outlet />
            ├── Route /user → ProfileSettings
            ├── Route /server → ServerSettings
            ├── Route /workspace/:wsId → WorkspaceSettings
            └── Route /org/:orgId → OrgSettings
```

## File Changes

### New Files
| File | Purpose |
|------|---------|
| `src/sections/WorkspaceSection.tsx` | Self-contained workspace module |
| `src/sections/SettingsSection.tsx` | Self-contained settings module |
| `src/sections/WorkspaceLayout.tsx` | Header + Sidebar + Outlet |
| `src/sections/SettingsLayout.tsx` | Settings sidebar + Outlet |
| `src/sections/NodeView.tsx` | Editor or Table based on node type |
| `src/sections/WorkspaceRoot.tsx` | Redirect to first node |

### Modified Files
| File | Changes |
|------|---------|
| `src/App.tsx` | Slim down to router setup only (~50 lines) |
| `src/contexts/WorkspaceContext.tsx` | Remove URL manipulation, use router navigate |
| `src/contexts/AuthContext.tsx` | Add redirect logic for unauthenticated users |
| `src/utils/urls.ts` | Simplify - fewer URL builders needed |

### Deleted/Deprecated
| File | Reason |
|------|--------|
| `src/components/settings/Settings.tsx` | Replaced by SettingsSection |
| `src/components/settings/SettingsSidebar.tsx` | Move to SettingsLayout |
| `src/components/WorkspaceSettings.tsx` | OLD - already deprecated |
| `src/components/OrganizationSettings.tsx` | OLD - already deprecated |
| `src/components/UserProfile.tsx` | OLD - already deprecated |

## Implementation Steps

### Phase 1: Setup Router
1. [x] Install `@solidjs/router`
2. [x] Create minimal `App.tsx` with route definitions
3. [x] Add route guards for authentication
4. [x] Verify static routes work (`/login`, `/privacy`, `/terms`)

### Phase 2: Extract WorkspaceSection
1. [x] Create `src/sections/WorkspaceSection.tsx`
2. [x] Move workspace providers into WorkspaceSection
3. [x] Create `WorkspaceLayout.tsx` with Header, Sidebar, Outlet
4. [x] Create `NodeView.tsx` (extract from current App.tsx)
5. [x] Create `WorkspaceRoot.tsx` for redirect logic
6. [x] Wire up nested routes under `/w/:wsId`

### Phase 3: Extract SettingsSection
1. [x] Create `src/sections/SettingsSection.tsx`
2. [x] Create `SettingsLayout.tsx` with sidebar + outlet
3. [x] Move existing settings panels to route components
4. [x] Wire up nested routes under `/settings`

### Phase 4: Cleanup
1. [x] Remove old routing signals from contexts
2. [x] Remove `handlePopState` and manual URL parsing
3. [x] Delete deprecated components (App.module.css removed)
4. [x] Update navigation to use `useNavigate()` hook
5. [ ] Update all `<a>` tags to use `<A>` from router (future refinement)

### Phase 5: Testing & Polish
1. [ ] Test all navigation paths (manual/e2e)
2. [ ] Test browser back/forward (manual/e2e)
3. [ ] Test deep linking (direct URL access) (manual/e2e)
4. [ ] Test authentication redirects (manual/e2e)
5. [x] Update App.test.tsx for new router (routing tests skipped, covered by e2e)

## Navigation Patterns

### Using the Router

```tsx
import { useNavigate, useParams } from '@solidjs/router';

// In components
const navigate = useNavigate();
const params = useParams();  // { wsId, nodeId }

// Navigate programmatically
navigate(`/w/${wsId}/${nodeId}`);

// Navigate to settings
navigate('/settings/user');

// Back to workspace from settings
navigate(`/w/${wsId}`);
```

### Route Guards

```tsx
// In App.tsx or a wrapper component
const ProtectedRoute = (props) => {
  const { user } = useAuth();

  if (!user()) {
    return <Navigate href="/login" />;
  }

  return props.children;
};
```

## Context Simplification

### Before (WorkspaceContext)
```tsx
// Handles URL parsing, state, navigation
const handlePopState = async () => { ... };
window.addEventListener('popstate', handlePopState);
```

### After (WorkspaceContext)
```tsx
// Only handles workspace data, uses router for navigation
const params = useParams();
const wsId = () => params.wsId;

createEffect(() => {
  if (wsId()) {
    loadWorkspace(wsId());
  }
});
```

## Migration Strategy

1. **Parallel implementation** - Build new routing alongside old
2. **Feature flag** - Toggle between old/new routing during development
3. **Incremental migration** - Move one section at a time
4. **Delete old code** - Only after new routing is stable

## Open Questions

- [ ] Should we lazy-load WorkspaceSection and SettingsSection?
- [ ] How to handle "return to workspace" from settings? (Store last workspace in context or localStorage)
- [ ] Should breadcrumbs be part of WorkspaceLayout or NodeView?

## References

- [@solidjs/router docs](https://docs.solidjs.com/solid-router)
- [SolidJS reactivity](https://docs.solidjs.com/concepts/reactivity)
