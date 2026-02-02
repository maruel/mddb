# Flat Block Editor Implementation Plan

Architectural redesign of the ProseMirror editor to use a flat block model (Notion-style), enabling uniform drag-and-drop for all content including individual list items.

> **Status: ⚠️ MVP FUNCTIONAL BUT BUGGY** (2026-02-01)
> 
> Core architecture implemented (flat block model, 308 unit tests passing). However, several UI and interaction bugs remain that prevent full usability.
> 
> **Critical Bugs:**
> - Drag handle alignment broken (left-aligned instead of centered, 3-dot icon in wrong div)
> - Drag-and-drop doesn't work (callbacks exist but don't fire correctly)
> - Numbered/bullet list vertical alignment issues (task lists OK)
> - Context menu broken (hover only works on first item, keyboard nav broken, undo stack corrupted after use)
> 
> **Remaining Work:**
> - Fix the above bugs
> - E2E drag-reorder tests (blocked by Playwright DND API)
> - Table row reorder persistence (pending backend API)

## Problem Statement

The current ProseMirror schema uses nested containers:
```
document
├── bullet_list          ← container node
│   ├── list_item        ← child, not independently draggable
│   └── list_item
└── paragraph
```

This makes per-item drag-and-drop complex with many edge cases (dragging out of lists, across lists, merging adjacent lists).

## Target Architecture

Flat block model where every visible line is a top-level node:
```
document
├── block (type=bullet, indent=0)
├── block (type=bullet, indent=1)   ← indentation via attribute
├── block (type=paragraph)
└── block (type=heading, level=2)
```

**Benefits:**
- Uniform drag-drop for all blocks (single code path)
- Every block gets an identical handle
- No edge cases for "drag item out of list"
- Matches user mental model (lines, not containers)

---

## Phase 1: Schema Redesign ✓ COMPLETE

**File:** `src/components/editor/schema.ts`

Implemented flat block schema with single `block` node type replacing all block-level nodes. Attributes: `type` (paragraph, heading, bullet, number, task, quote, code, divider), `level` (for headings), `indent` (for nesting), `checked` (for tasks), `language` (for code).

**DOM Parsing:** `src/components/editor/dom-parser.ts` and `domParsePlugin.ts` preprocess pasted HTML, detecting list types (`<ul>` vs `<ol>`), extracting nesting depth, and detecting task lists. Code language extraction from class patterns implemented.

**DOM Serialization:** `blockSpec.toDOM()` renders blocks as appropriate HTML elements with `data-type` and `data-indent` attributes for positioning and reconstruction.

**Status:** All tasks complete. 20 unit tests passing for HTML→block conversion. CSS classes for list styling implemented.

---

## Phase 2: Markdown Serialization ✓ COMPLETE

**Files:** `src/components/editor/markdown-parser.ts`, `markdown-serializer.ts`

**Parser:** Converts markdown to flat blocks using `MarkdownParser` with nested schema, then recursively flattens nested structures via `flattenDocument()` → `flattenList()` → `flattenListItem()`. Detects task lists and extracts code block languages.

**Serializer:** Reconstructs markdown from flat blocks by walking sequentially, tracking open list contexts, maintaining per-indent number counters, and emitting proper markers and indentation. Handles all block types and edge cases (interruptions, mixed types, code blocks, task lists).

**Status:** All tasks complete. 23 unit tests for parser, all round-trip tests passing (MD → blocks → MD). Edge cases for nested structures, numbered list restarts, and mixed content validated.

---

## Phase 3: Shared Row Handle System

The row handle must be reusable across two contexts:
1. **Editor blocks** — ProseMirror document blocks (paragraphs, lists, headings)
2. **Table rows** — Records in Table, Grid, Gallery, and Board views

Both share identical appearance and behavior but differ in their data model and drag-drop mechanics.

### 3.1 Shared Handle Component ⚠️ BUGGY

**Files:** `src/components/shared/RowHandle.tsx`, `RowHandle.module.css`

Context-agnostic handle component receiving callbacks for drag and context menu. Renders draggable icon with proper event handling and accessibility attributes.

**Status:** Integrated but **alignment broken** — handle is left-aligned instead of centered, 3-dot icon in wrong div. See Known Issues #1.

### 3.2 Shared Context Menu ✓ COMPLETE

**Files:** `src/components/table/RowContextMenu.tsx`, `RowContextMenu.module.css`

Floating context menu with dismiss-on-click-outside and Escape-key handling. Reusable for both editor blocks and table rows.

**Status:** Complete. Event handling and styling finalized.

### 3.3 Drop Indicator ✓ COMPLETE

**Files:** `src/components/shared/DropIndicator.tsx`, `DropIndicator.module.css`

Visual feedback component showing drop position during drag operations. Positioned absolutely and z-indexed above content.

**Status:** Complete. Styled and positioned correctly.

### 3.4 Editor Integration (BlockNodeView)

**File:** `src/components/editor/BlockNodeView.ts`

ProseMirror `NodeView` implementation for blocks. Wires `RowHandle` and `RowContextMenu` to editor drag-drop and selection.

**Tasks:**
- [x] Integrate `RowHandle` into block DOM
- [x] Connect handle to `blockDragPlugin`
- [x] Connect context menu to block position
- [x] Multi-block selection support (getSelection spans multiple blocks)
- [x] Show handle only for topmost block in selection

### 3.5 Table Integration ✓ COMPLETE

**File:** `src/components/table/TableRow.tsx`

Reusable `TableRow` wrapper component with integrated `RowHandle` and `RowContextMenu`. Used across TableTable, TableGrid, TableGallery, and TableBoard views for consistent drag-and-drop behavior.

**Implemented:**
- [x] `RowHandle` integration with drag event handlers
- [x] Context menu actions: open record, duplicate, delete
- [x] `TABLE_RECORD_MIME` type for drag data
- [x] Drop indicator and dragging visual states
- [x] Props for all CRUD callbacks (`onOpen`, `onDuplicate`, `onDelete`)
- [x] Drag lifecycle callbacks (`onDragStart`, `onDragOver`, `onDrop`)

**Deferred:**
- Row reordering persistence (pending: table sorting API decision)

### 3.6-3.8 Additional Phases

Complete but summarized: Multi-block selection visible indicators, visibility toggling on selection, batch operations across multiple blocks.

---

## Phase 4: Block Drag-and-Drop ⚠️ BUGGY

**Files:** `src/components/editor/blockDragPlugin.ts`, ProseMirror event handlers

**Plugin State:** Tracks `sourcePos`, `dropTarget`, `dropIndicatorY` during drag lifecycle.

**Drag Event Handlers:**
- `onDragStart`: Set drag data with source block position
- `onDragOver`: Calculate nearest block boundary and update drop indicator
- `onDrop`: Execute move transaction, adjusting positions as needed
- `dragend/dragleave`: Clear plugin state

**Features:**
- Single and multi-block drag support
- Drop indicator line shows insertion point
- Automatic position adjustment when source before target
- Prevents drop on self

**Status:** Code complete but **drag doesn't work**. Callbacks exist but don't fire correctly. See Known Issues #1 and #2.

---

## Phase 5: Context Menu ⚠️ BUGGY

**Files:** `src/components/editor/BlockContextMenu.tsx`, `blockCommands.ts`

**Menu Component:** Floating context menu positioning at mouse cursor, with options for delete, duplicate, indent/outdent, and type conversion. Supports single and multi-block operations. Click-outside and Escape-key dismissal implemented. i18n keys added.

**Commands:** Implemented ProseMirror commands: `deleteBlock`/`deleteBlocks`, `duplicateBlock`/`duplicateBlocks`, `convertBlock`/`convertBlocks`, `indentBlock`/`indentBlocks`, `outdentBlock`/`outdentBlocks`, `toggleTaskBlock`. Max indent 8 levels. Multi-block selection support.

**Status:** Code complete but **context menu broken**:
- Only first menu item highlights on hover
- Keyboard navigation (up/down arrows) doesn't work
- Clicking items works but corrupts undo stack

See Known Issues #4.

---

## Phase 6: Keyboard Commands ✓ COMPLETE

**Files:** `src/components/editor/blockKeymap.ts`, `blockInputRules.ts`

**Keybindings Implemented:**
- Tab/Shift+Tab: Indent/outdent blocks
- Enter: Smart splitting (paragraph splits, code block inserts newline, list item creates sibling)
- Backspace at line start: Merge with previous block or convert to paragraph

**Input Rules:**
- `- ` → bullet block
- `1. ` or `* ` → numbered block
- `# ` through `###### ` → heading levels 1-6
- `> ` → quote block
- ``` ``` ``` (with language) → code block
- `---` → divider

**Status:** All input rules and keymap commands complete. Unit tests in `blockKeymap.test.ts` and `blockInputRules.test.ts` passing.

---

## Phase 7: Block Styling ⚠️ BUGGY

**File:** `src/components/editor/Editor.module.css` (plus individual block styling)

**Block Type Rendering:**
- Headings: HTML `<h1>`-`<h6>` with proper hierarchy
- Bullets: Unicode bullet or custom CSS marker
- Numbers: CSS counter per indent level
- Tasks: Checkbox input with toggle handler ✓ (alignment OK)
- Quotes: Left border with background tint
- Code blocks: Pre-formatted font with language badge
- Dividers: Horizontal rule
- Paragraphs: Plain text with standard line-height

**Indentation:** Applied via padding-left or margin-left, scales with indent level.

**Status:** Block types styled but **bullet and number lists have vertical alignment issues**. Task list checkboxes are correctly aligned. See Known Issues #3.

---

## Phase 8: Migration ✓ COMPLETE

**Approach:** Transparent on-load conversion. No stored format change.

**Process:**
1. Load markdown file
2. Parse with old MarkdownParser (nested schema)
3. Flatten to new schema blocks
4. Save: serialize back to markdown (produces identical format)

**Compatibility:** Round-trip verified (old MD → new schema → markdown → old schema). Existing documents load without modification.

**Tasks:**
- [x] Integrate parser/serializer into Editor
- [x] Configure schema in prosemirror-config
- [x] Verify round-trip fidelity
- [x] Test with existing nested list documents
- [ ] Feature flag (optional, for gradual rollout)

**Status:** Data migration transparent and verified. Feature flagging deferred.

---

## Phase 9: Testing

### 9.1 Unit Tests ✓ COMPLETE

**Test Coverage (308 tests passing across 19 test files):**

| File | Tests | Coverage |
|------|-------|----------|
| `schema.test.ts` | Block creation and attribute handling | ✓ |
| `markdown-parser.test.ts` | Nested MD → flat blocks (23 tests) | ✓ |
| `markdown-serializer.test.ts` | Round-trip MD → blocks → MD | ✓ |
| `markdown-utils.test.ts` | Helper functions | ✓ |
| `blockCommands.test.ts` | Delete, duplicate, convert, indent/outdent (33 tests) | ✓ |
| `slashCommands.test.ts` | Block type conversion via `/` menu | ✓ |
| `dom-parser.test.ts` | HTML paste → block conversion (20 tests) | ✓ |
| `blockKeymap.test.ts` | Tab, Enter, Backspace behavior | ✓ |
| `blockInputRules.test.ts` | Auto-format detection (10 tests) | ✓ |

**Edge cases covered:**
- Deeply nested lists with mixed bullet/number types
- Task list checkbox toggling
- Code block language extraction
- Heading level limits (1-6)
- Indent level limits (0-8)

### 9.2 E2E Tests ✓ PARTIALLY COMPLETE

**Location:** `/e2e/` (project root)

**Block Editor Tests:**
| File | Tests | Status |
|------|-------|--------|
| `block-editor.spec.ts` | Input rules, drag handle presence | ✓ |
| `block-handle-visibility.spec.ts` | Handle opacity on hover, numbered list data-number, bullet alignment | ✓ |
| `block-handle-diagnostic.spec.ts` | Diagnostic tests for handle visibility and text alignment | ✓ |

**Implemented:**
- [x] Create blocks via input rules (markdown → blocks parsing)
- [x] Verify block types render with correct `data-type` attributes
- [x] Drag handles present on all blocks with proper `draggable` attribute
- [x] ARIA labels for accessibility
- [x] Handle visibility on hover (opacity transition)
- [x] Numbered list `data-number` attribute for CSS counters
- [x] Bullet text alignment

**Not Yet Implemented:**
- [ ] Drag block to reorder (blocked by Playwright DND API limitations)
- [ ] Context menu E2E actions
- [ ] Undo/redo after drag operations

**Note:** Full drag-reorder tests require Chromium DevTools Protocol extensions not available in standard Playwright. Drag logic is verified via unit tests in `blockCommands.test.ts`.

**⚠️ Current E2E tests do not fully verify functionality** — they check DOM structure but not actual interaction behavior.

---

## Known Issues (Bugs to Fix)

### Issue 1: Drag Handle Alignment 🔴 HIGH

**Symptom:** Drag handle (3-dot icon) is aligned to the left instead of centered vertically with text. The icon appears to be in the wrong container div.

**Location:** `RowHandle.tsx`, `RowHandle.module.css`, `BlockNodeView.ts`

**Suggested Fix:**
- Verify handle DOM structure matches intended layout
- Check flexbox alignment on `.block-handle-container`
- Ensure handle icon is inside the correct wrapper

---

### Issue 2: Drag-and-Drop Non-Functional 🔴 HIGH

**Symptom:** Dragging blocks doesn't work. Callbacks exist in `blockDragPlugin.ts` but don't fire correctly during drag operations.

**Location:** `blockDragPlugin.ts`, `BlockNodeView.ts`

**Possible Causes:**
- `draggable` attribute not correctly wired
- `onDragStart` not firing or not setting drag data
- ProseMirror event handling interference
- DOM structure preventing drag events from reaching handlers

**Suggested Debug Steps:**
1. Add console logs to `onDragStart`, `onDragOver`, `onDrop` handlers
2. Verify `e.dataTransfer.setData()` is called
3. Check if ProseMirror's default drag handling is interfering
4. Test with native browser drag events outside ProseMirror

---

### Issue 3: List Item Vertical Alignment 🟡 MEDIUM

**Symptom:** Numbered list numbers and bullet points are not vertically aligned with text content. Task list checkboxes are correctly aligned.

**Location:** `Editor.module.css` (`.block-row[data-type="bullet"]`, `.block-row[data-type="number"]`)

**Suggested Fix:**
- Compare CSS for task vs bullet/number blocks
- Check `::before` pseudo-element positioning
- Verify `align-items` and `line-height` values
- May need explicit vertical centering with flexbox

---

### Issue 4: Context Menu Broken 🔴 CRITICAL

**Symptoms:**
1. Only first menu item highlights on mouse hover
2. Up/down arrow keyboard navigation has no effect
3. Clicking menu item works but corrupts undo stack afterward

**Location:** `BlockContextMenu.tsx`, `RowContextMenu.tsx`, `blockCommands.ts`

**Suggested Debug Steps:**
1. **Hover issue:** Check CSS `:hover` selectors and z-index stacking
2. **Keyboard nav:** Verify `onKeyDown` handler is attached and `e.preventDefault()` called
3. **Undo corruption:** Check that commands use proper ProseMirror transactions:
   - Ensure `tr.setMeta('addToHistory', true)` or not calling with false
   - Verify single transaction per action (not multiple dispatches)
   - Check for state mutations outside transactions

---

### Issue 5: E2E Test Coverage Gap 🟡 MEDIUM

**Symptom:** Current E2E tests verify DOM structure but don't catch interaction bugs (drag, context menu, keyboard nav).

**Suggested Additions:**
- Test context menu hover states
- Test keyboard navigation in menus
- Test undo/redo after operations
- Mock or simulate drag events if possible

---

## Implementation Order

### Track A: Shared Components ⚠️ BUGGY

Phases 3.1-3.3 (RowHandle, RowContextMenu, DropIndicator) — code complete but handle alignment broken.

### Track B: Editor Block Model ⚠️ PARTIALLY WORKING

1. ✓ Phase 1: Schema
2. ✓ Phase 2: Markdown parser/serializer
3. ✓ Phase 6: Keyboard and input rules
4. ⚠️ Phase 7: Styling (list alignment issues)
5. ✓ Phase 9.1: Unit tests (308 passing)
6. ⚠️ Phase 3.4: Editor integration (handle alignment broken)
7. ⚠️ Phase 4: Drag-drop (non-functional)
8. ⚠️ Phase 5: Context menu (broken hover/keyboard/undo)
9. ✓ Phase 8: Migration (transparent on-load)
10. ⚠️ Phase 9.2: E2E tests (don't verify interaction bugs)

### Track C: Table Integration ⚠️ UI INCOMPLETE

Phase 3.5 — TableRow component code complete but inherits handle/drag bugs from shared components.

**Deferred:** Row reordering persistence pending table sorting API decision.

---

## Decisions

1. **Multi-block selection:** ✅ Yes. Topmost handle visible when selection spans multiple blocks. Dragging moves all selected blocks. Context menu actions apply to all selected.

2. **Code blocks:** ✅ Single blocks. Entire fenced code block (multi-line) is one movable unit.

3. **Cross-context drag:** ✅ Not supported for MVP. Editor ↔ table drag-drop out of scope.

4. **Touch support:** ✅ Follow-up work. Touch drag requires different UX patterns (long-press initiation).

5. **Block IDs:** ✅ Not needed. Blocks are visual/UX layer, not persisted. Markdown is storage; positions suffice for runtime.

6. **Table row reordering:** ✅ Deferred. Rows currently sorted by creation time. Manual reordering API future enhancement.

7. **Nested blocks:** ✅ Future enhancement. Container blocks (toggles, callouts, columns) out of scope for MVP. Flat `indent` model sufficient for hierarchical lists.

---

## Design Principles

1. **Separation of concerns:** UI components know nothing about ProseMirror or table records. They receive callbacks.

2. **Context-specific integration:** Each consumer provides own drag data format, context menu actions, and drop handling.

3. **Consistent appearance:** All handles use identical styling. Visual consistency reinforces mental model.

4. **Progressive enhancement:** Handles are optional UI chrome. Data model works without them (print/export, keyboard-only).

---

## References

- [ProseMirror Guide](https://prosemirror.net/docs/guide/)
- [ProseMirror NodeView](https://prosemirror.net/docs/ref/#view.NodeView)
- [Notion's Block Model](https://www.notion.so/help/what-is-a-block)
- [Lexical (Meta's block editor)](https://lexical.dev/)
