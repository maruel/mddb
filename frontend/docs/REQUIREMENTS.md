# Frontend Requirements

## Functional Requirements

### 1. Document Management (UI)
- [ ] **WYSIWYG Editor**: Block-based, Notion-like editor behavior (drag-and-drop blocks, slash commands, inline formatting) replacing the raw Markdown editor.
- [ ] **Interlinking**: UI for selecting and inserting links to other pages.
- [ ] **Graph View**: Visual representation of the knowledge graph.

### 2. Tables & Data Visualization
- [x] **Table Views**: Spreadsheet-like interface for record editing.
- [ ] **View Modes**: Implementation of Board (Kanban), Gallery, and Grid views.
- [ ] **Property Editing**: UI for dynamically modifying table schemas (adding/renaming columns).
- [/] **Filtering/Sorting**: UI controls for complex queries and persistent sorting.

### 3. User Experience (UX)
- [ ] **Unified Sidebar**: Hierarchical tree navigation for all content.
- [ ] **Seamless Tables**: Integration of table views directly into markdown pages.
- [x] **Auto-save**: Background saving with visual indicators.
- [x] **Search UI**: Global search interface with real-time results.
- [x] **History UI**: Interface for viewing and restoring page versions.
- [ ] **Onboarding Templates**: Propose starting from a template Git repository during the initial setup flow.

### 4. Globalization & PWA
- [x] **i18n**: Support for English, French, German, and Spanish via `@solid-primitives/i18n`.
- [ ] **Regional Formatting**: Locale-aware date, time, and number formatting.
- [/] **PWA**: App installation support and offline-ready UI components.

### 5. Command-Centric UX
- [ ] **Command Palette**: Central `Ctrl+K` modal for navigation and executing system commands.
- [ ] **Slash Commands**: Inline `/` commands in the editor for inserting components or calling AI.

### 6. Performance & Scale
- [ ] **Table Virtualization**: Row and column virtualization for high-performance rendering of 50k+ records.
- [ ] **Optimistic UI**: Immediate local state updates for reordering and edits to ensure zero-latency feel.

### 7. Visual Intelligence
- [ ] **Relationship Graph**: High-performance visualization of backlinks and node connections.
- [ ] **Adaptive Themes**: Per-organization branding and customizable accent colors.

## Non-Functional Requirements

### UI/UX Standards
- [x] **SolidJS**: High-performance reactive UI.
- [x] **CSS Modules**: Component-scoped styling to prevent pollution.
- [x] **Responsiveness**: Mobile-friendly layout and interactions.

### Development
- [x] **Type Safety**: Use of generated TypeScript types from the backend.
- [x] **Deterministic Builds**: Consistent Vite-based build pipeline.