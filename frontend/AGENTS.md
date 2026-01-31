# Frontend Development Guidelines

## Frontend Development (SolidJS)

### Code Organization

- Components in `src/components/`
- Global state in `src/stores/` (if needed) or Context
- **CSS Modules**: Each `.tsx` file should have a corresponding `.module.css` file in the same directory. Import styles as `import styles from './ComponentName.module.css'`. This keeps component logic and styling colocated and prevents global CSS pollution.

### Navigation & Links

Prefer `<a>` tags when the destination URL is known and deterministic:

- **Use `<a href="..." onClick={...}>`** for navigation with known destinations. This shows the URL on hover and enables "Copy link" for power users.
- Use `e.preventDefault()` in `onClick` when you need programmatic navigation (e.g., updating state before navigating).
- **Use `<button>`** when the destination is dynamic/unknown (e.g., switching context requires an API call to determine the final URL).

```tsx
// Good: Deterministic URL - use <a> so users can see/copy the link
<a
  href={`/o/${orgId}+${orgSlug}/settings`}
  onClick={(e) => {
    e.preventDefault();
    navigateToOrgSettings(orgId);
  }}
>
  Settings
</a>

// OK: Non-navigation action - use <button>
<button onClick={() => doSomething()}>
  Action
</button>
```

### Reactivity & Routing State

**Never use `window.location` directly for UI state.** SolidJS won't re-render when `window.location` changes because it's not reactive.

```tsx
// BAD: Not reactive - UI won't update on navigation
const isActive = (url: string) => {
  return window.location.pathname === url;  // Won't trigger re-render
};

// GOOD: Use reactive props/signals passed from router
const isActive = (url: string) => {
  return props.currentRoute.path === url;  // Reactive, will re-render
};
```

When determining active states for navigation items, always derive from reactive route state (props, signals, or context) rather than reading `window.location` directly.

**When passing callbacks to child components, pass reactive values as parameters.** SolidJS only tracks dependencies accessed within the component's own rendering context. If a callback accesses `props.foo` from the parent's closure, the child won't re-render when `foo` changes.

```tsx
// BAD: Child won't re-render when props.currentRoute changes
// Parent component:
const isActive = (url: string) => {
  return props.currentRoute.path === url;  // Accessed from parent's closure
};
<ChildComponent isActive={isActive} />

// Child component:
const classes = () => props.isActive('/foo') ? 'active' : '';  // Not reactive!

// GOOD: Pass reactive value as parameter so child tracks the dependency
// Parent component:
const isActive = (url: string, route: Route) => {
  return route.path === url;  // Route passed as parameter
};
<ChildComponent isActive={isActive} currentRoute={props.currentRoute} />

// Child component:
const classes = () => props.isActive('/foo', props.currentRoute) ? 'active' : '';  // Reactive!
```

### Build & Distribution

mddb uses `go:embed` to include the frontend in the mddb binary created from ../backend/cmd/mddb:

```bash
# Build frontend + Go binary with embedded frontend
make build-all

# Result: ./mddb (single executable, self-contained)
```

The compiled `../backend/frontend/dist/` folder is tracked in git for reproducible builds.

### Development Workflow

**Frontend development** (live reload):
```bash
make frontend-dev
# Frontend at http://localhost:5173 (proxies API to :8080)
```

**Backend + embedded frontend** (for testing embedded binary):
```bash
make frontend-build   # Build frontend once
make build            # Build Go binary
./mddb                # Run with embedded frontend
```

## Internationalization (i18n)

**All user-visible strings must be localized.**

### Adding New Strings

1. **Add the key to `src/i18n/types.ts`** in the appropriate section:
   ```typescript
   // In the Dictionary interface
   mySection: {
     existingKey: string;
     newKey: string;  // Add your new key
   };
   ```

2. **Add translations to ALL dictionary files**:
   - `src/i18n/dictionaries/en.ts` (English - required)
   - `src/i18n/dictionaries/fr.ts` (French)
   - `src/i18n/dictionaries/de.ts` (German)
   - `src/i18n/dictionaries/es.ts` (Spanish)

3. **Use the `t()` function in components**:
   ```tsx
   import { useI18n } from '../i18n';

   function MyComponent() {
     const { t } = useI18n();
     return <button>{t('mySection.newKey')}</button>;
   }
   ```

### Dictionary Structure

- `common.*` - Reusable strings (loading, save, cancel, delete)
- `app.*` - App-level UI (title, navigation, footer links)
- `auth.*` - Authentication forms
- `editor.*` - Document editor
- `welcome.*` - Welcome/empty states
- `onboarding.*` - Onboarding wizard
- `settings.*` - Settings panels
- `table.*` - Table views
- `errors.*` - Error messages (keyed by ErrorCode for API errors)
- `success.*` - Success messages

### Guidelines

- Never hardcode user-visible strings in components
- Use `t('key') || 'fallback'` for placeholders/titles that need string type
- Error messages from API use `translateError(code)` helper
- Keep translations concise - UI space is limited
- Test with longer languages (German) to catch overflow issues
- Dismissable popups (modals, dropdowns, menus) must be dismissable with the Escape key

## Icons

mddb uses Material Symbols via the `@material-symbols/svg-400` package. Icons are imported as Solid components using `vite-solid-svg`.

### Finding Icons

To find a suitable icon:

1.  **Search locally**: Search the package contents directly using `grep`:
    ```bash
    ls node_modules/@material-symbols/svg-400/outlined | grep "keyword"
    ```
2.  **Verify existing usage**: Check how similar icons are used in the codebase to maintain consistency:
    ```bash
    grep -r "Icon from '@material-symbols" frontend/src
    ```
3.  **Import and Use**: Always use the `?solid` suffix to import the SVG as a Solid component:
    ```tsx
    import HomeIcon from '@material-symbols/svg-400/outlined/home.svg?solid';

    // Use as a component
    <HomeIcon />
    ```

### Styling Icons

Icons behave like text. They default to `1em` size and inherit `currentColor`.

- **Global rules**: Defined in `src/variables.css`.
- **Custom sizing**: Set `font-size` on the parent container or the `svg` element itself.
- **Vertical alignment**: Use `display: inline-flex` and `align-items: center` on the parent for perfect centering.

## ProseMirror

When inserting styled content programmatically, use marks rather than raw markdown text:

```tsx
// BAD: Raw markdown text won't render as a link
const text = schema.text(`[${title}](${url})`);

// GOOD: Use marks for inline formatting
const linkMark = schema.marks.link.create({ href: url });
const text = schema.text(title, [linkMark]);
```

## Code Quality & Linting

**All code must pass linting before commits.**

### Frontend (ESLint + Prettier)

Configured in root `eslint.config.js` and `.prettierrc`. Enforces strict equality, no-unused-vars, and consistent formatting (single quotes, 2 spaces).

## Useful Resources

- [SolidJS Docs](https://docs.solidjs.com)
- [solid-primitives/i18n](https://github.com/solidjs-community/solid-primitives/tree/main/packages/i18n)

<!-- BEGIN FILE INDEX -->
## File Index

Autogenerated file index based on first-line comments.

- `README.md`: mddb Frontend Architecture and Setup
- `docs/FIRST_LOGIN_FLOW.md`: Frontend First-Login Implementation
- `docs/PLAN.md`: Frontend Implementation Plan
- `docs/PLAN_BLOCK_EDITOR.md`: Flat Block Editor Implementation Plan
- `docs/PLAN_UNDO.md`: Undo System Implementation Plan
- `docs/PLAN_VIEWS.md`: Table Views Implementation Plan
- `docs/REQUIREMENTS.md`: Frontend Requirements
- `src/App.tsx`: Main application component with router setup.
- `src/components/Auth.tsx`: Authentication component handling login and registration forms.
- `src/components/CreateOrgModal.tsx`: Modal component for creating a new organization.
- `src/components/CreateWorkspaceModal.tsx`: Modal component for creating a new workspace.
- `src/components/ErrorBoundary.tsx`: Error boundary component to prevent full app crashes.
- `src/components/MarkdownPreview.tsx`: Component for rendering Markdown content with custom plugins.
- `src/components/NotionImportBanner.tsx`: Banner showing Notion import progress and completion status.
- `src/components/NotionImportModal.tsx`: Modal for importing a workspace from Notion.
- `src/components/Onboarding.tsx`: Onboarding wizard for new users to configure their workspace.
- `src/components/OrganizationSettings.tsx`: Organization settings page for managing organization name, members, and preferences.
- `src/components/PWAInstallBanner.tsx`: Banner component prompting users to install the app as a PWA.
- `src/components/Privacy.tsx`: Privacy policy page component.
- `src/components/Sidebar.tsx`: Sidebar navigation component containing workspace selection and page tree.
- `src/components/SidebarNode.test.tsx`: Unit tests for SidebarNode component and sidebar data flow.
- `src/components/SidebarNode.tsx`: Recursive component for rendering navigation tree nodes in the sidebar.
- `src/components/TableBoard.tsx`: Kanban board view for table records, grouped by select/multi-select columns.
- `src/components/TableGallery.tsx`: Gallery view for table records, emphasizing images.
- `src/components/TableGrid.tsx`: Grid view for table records, displaying data in cards.
- `src/components/TableTable.tsx`: Notion-like table view with inline editing.
- `src/components/Terms.tsx`: Terms of Service page component.
- `src/components/UserMenu.tsx`: Dropdown menu for user profile and logout.
- `src/components/UserProfile.tsx`: User profile page for managing personal settings.
- `src/components/WorkspaceMenu.tsx`: Dropdown menu for switching workspaces and accessing workspace settings.
- `src/components/WorkspaceSettings.tsx`: Workspace settings page for managing workspace and members.
- `src/components/editor/BlockContextMenu.tsx`: Editor-specific block context menu component.
- `src/components/editor/BlockNodeView.ts`: ProseMirror NodeView implementation for flat blocks with integrated drag handles.
- `src/components/editor/Editor.tsx`: WYSIWYG markdown editor component using ProseMirror with prosemirror-markdown.
- `src/components/editor/EditorToolbar.tsx`: Floating editor toolbar with formatting buttons (appears on text selection).
- `src/components/editor/SlashCommandMenu.tsx`: Slash command menu overlay component for selecting block types.
- `src/components/editor/blockCommands.ts`: ProseMirror commands for block operations.
- `src/components/editor/blockDragPlugin.ts`: ProseMirror plugin for block-level drag-and-drop functionality.
- `src/components/editor/blockInputRules.ts`: Input rules for flat block editor.
- `src/components/editor/blockKeymap.ts`: Keyboard bindings for flat block editor.
- `src/components/editor/dom-parser.test.ts`: Unit tests for DOM parsing utilities.
- `src/components/editor/dom-parser.ts`: DOM parsing utilities for converting pasted/loaded HTML to flat block format.
- `src/components/editor/domParsePlugin.ts`: ProseMirror plugin that preprocesses pasted HTML before parsing.
- `src/components/editor/dropUploadPlugin.ts`: ProseMirror plugin for drag-and-drop file upload support.
- `src/components/editor/invalidLinkPlugin.ts`: ProseMirror plugin for highlighting invalid (broken) internal page links.
- `src/components/editor/markdown-parser.test.ts`: Unit tests for markdown parser: converting markdown to flat blocks.
- `src/components/editor/markdown-parser.ts`: Markdown-to-flat-blocks parser: converts markdown to ProseMirror document with flat block structure.
- `src/components/editor/markdown-serializer.test.ts`: Unit tests for markdown serializer: converting flat blocks back to markdown.
- `src/components/editor/markdown-serializer.ts`: Flat-blocks-to-markdown serializer: reconstructs nested markdown from flat block structure.
- `src/components/editor/markdown-utils.test.ts`: Unit tests for markdown utility functions.
- `src/components/editor/markdown-utils.ts`: Utility functions for asset URL handling in markdown content.
- `src/components/editor/prosemirror-config.ts`: ProseMirror configuration: input rules, keymap, markdown parser/serializer, and task list support.
- `src/components/editor/schema.ts`: Flat block schema for Notion-style editor with uniform drag-drop.
- `src/components/editor/slashCommandPlugin.ts`: ProseMirror plugin for detecting "/" slash commands and tracking menu state.
- `src/components/editor/slashCommands.test.ts`: Unit tests for slash commands.
- `src/components/editor/slashCommands.ts`: Slash command registry defining available block types for the editor menu.
- `src/components/editor/useAssetUpload.ts`: Hook for uploading assets to a node via multipart form data.
- `src/components/settings/InviteForm.tsx`: Shared invite form component for workspace and organization settings.
- `src/components/settings/LinkedAccountsSection.tsx`: Linked accounts section for managing OAuth provider connections.
- `src/components/settings/MembersTable.tsx`: Shared members table component for workspace and organization settings.
- `src/components/settings/OrgSettingsPanel.tsx`: Organization settings panel for managing organization members and preferences.
- `src/components/settings/PasswordSection.tsx`: Password management section for adding or changing password.
- `src/components/settings/ProfileSettings.tsx`: User profile settings panel for managing personal preferences.
- `src/components/settings/ServerSettingsPanel.tsx`: Server settings panel for global admins to configure SMTP and quotas.
- `src/components/settings/SettingsNavItem.tsx`: Expandable navigation item for settings sidebar.
- `src/components/settings/SettingsSidebar.tsx`: Settings sidebar navigation with expandable workspace and organization items.
- `src/components/settings/WorkspaceSettingsPanel.tsx`: Workspace settings panel for managing workspace members, settings, and git sync.
- `src/components/settings/index.ts`: Settings components barrel export.
- `src/components/shared/index.ts`: Shared components index
- `src/components/table/TableRow.tsx`: Shared table row wrapper with drag handle and context menu.
- `src/components/table/ViewTabs.tsx`: Horizontal tabs for switching between saved table views.
- `src/components/table/tableUtils.ts`: Shared utilities for table views.
- `src/composables/useClickOutside.ts`: Composable for detecting clicks outside an element.
- `src/contexts/AuthContext.tsx`: Authentication context providing user state, token management, and API clients.
- `src/contexts/EditorContext.tsx`: Editor context providing title, content, auto-save, and history management.
- `src/contexts/RecordsContext.tsx`: Records context providing table records CRUD and pagination.
- `src/contexts/WorkspaceContext.tsx`: Workspace context providing node tree, navigation, and workspace switching.
- `src/contexts/index.ts`: Context providers for global application state.
- `src/i18n/dictionaries/de.ts`: German translation dictionary.
- `src/i18n/dictionaries/en.ts`: English translation dictionary (default).
- `src/i18n/dictionaries/es.ts`: Spanish translation dictionary.
- `src/i18n/dictionaries/fr.ts`: French translation dictionary.
- `src/i18n/index.tsx`: Internationalization provider and context hooks.
- `src/i18n/types.ts`: Type definitions for internationalization (dictionaries and locales).
- `src/index.tsx`: Application entry point rendering the root component.
- `src/sections/NodeView.tsx`: Node view component displaying editor or table based on node type.
- `src/sections/Onboarding.tsx`: Onboarding component for first-time users without org/workspace.
- `src/sections/SettingsLayout.tsx`: Settings layout with sidebar navigation and content outlet.
- `src/sections/SettingsSection.tsx`: Settings section with nested routes for user, workspace, org, and server settings.
- `src/sections/WorkspaceLayout.tsx`: Workspace layout with header, sidebar, and content outlet.
- `src/sections/WorkspaceRoot.tsx`: Workspace root component that redirects to the first node.
- `src/sections/WorkspaceSection.tsx`: Workspace section module wrapping providers and nested routes.
- `src/useApi.ts`: Utilities for creating authenticated and retry-enabled API clients.
- `src/utils/debounce.ts`: Utility function for debouncing function calls.
- `src/utils/urls.ts`: URL construction utilities for consistent routing.
<!-- END FILE INDEX -->
