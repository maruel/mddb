# Frontend Requirements

## Functional Requirements

### 1. Document Management (UI)
- [x] **Editor**: Markdown editing interface with live preview.
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

### 4. Globalization & PWA
- [x] **i18n**: Support for English, French, German, and Spanish via `@solid-primitives/i18n`.
- [ ] **Regional Formatting**: Locale-aware date, time, and number formatting.
- [/] **PWA**: App installation support and offline-ready UI components.

## Non-Functional Requirements

### UI/UX Standards
- [x] **SolidJS**: High-performance reactive UI.
- [x] **CSS Modules**: Component-scoped styling to prevent pollution.
- [x] **Responsiveness**: Mobile-friendly layout and interactions.

### Development
- [x] **Type Safety**: Use of generated TypeScript types from the backend.
- [x] **Deterministic Builds**: Consistent Vite-based build pipeline.
