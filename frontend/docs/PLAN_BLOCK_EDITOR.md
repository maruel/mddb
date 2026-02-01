# Flat Block Editor Implementation Plan

Architectural redesign of the ProseMirror editor to use a flat block model (Notion-style), enabling uniform drag-and-drop for all content including individual list items.

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

### 3.1 Shared Handle Component ✓ COMPLETE

**Files:** `src/components/shared/RowHandle.tsx`, `RowHandle.module.css`

Context-agnostic handle component receiving callbacks for drag and context menu. Renders draggable icon with proper event handling and accessibility attributes.

**Status:** Complete. Integrated into `BlockNodeView` and table row components.

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

### 3.5 Table Integration

**File:** `src/components/table/TableRow.tsx`

Reuse `RowHandle` and `RowContextMenu` for table row drag-drop. Wire callbacks to table CRUD operations.

**Tasks:**
- [ ] Connect handle to table row drag-drop (pending: table API decision)
- [ ] Context menu actions: open record, duplicate, delete
- [ ] Visual feedback during reordering

### 3.6-3.8 Additional Phases

Complete but summarized: Multi-block selection visible indicators, visibility toggling on selection, batch operations across multiple blocks.

---

## Phase 4: Block Drag-and-Drop ✓ COMPLETE

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

**Status:** All tasks complete. Single and multi-block moves working.

---

## Phase 5: Context Menu ✓ COMPLETE

**Files:** `src/components/editor/BlockContextMenu.tsx`, `blockCommands.ts`

**Menu Component:** Floating context menu positioning at mouse cursor, with options for delete, duplicate, indent/outdent, and type conversion. Supports single and multi-block operations. Click-outside and Escape-key dismissal implemented. i18n keys added.

**Commands:** Implemented ProseMirror commands: `deleteBlock`/`deleteBlocks`, `duplicateBlock`/`duplicateBlocks`, `convertBlock`/`convertBlocks`, `indentBlock`/`indentBlocks`, `outdentBlock`/`outdentBlocks`, `toggleTaskBlock`. Max indent 8 levels. Multi-block selection support.

**Status:** All tasks complete. 33 unit tests passing for command logic.

---

## Phase 6: Keyboard Commands ✓ MOSTLY COMPLETE

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

**Status:** Core input/keyboard behaviors complete. Advanced selection and navigation refinements in progress.

---

## Phase 7: Block Styling ✓ COMPLETE

**File:** `src/components/editor/Editor.module.css` (plus individual block styling)

**Block Type Rendering:**
- Headings: HTML `<h1>`-`<h6>` with proper hierarchy
- Bullets: Unicode bullet or custom CSS marker
- Numbers: CSS counter per indent level
- Tasks: Checkbox input with toggle handler
- Quotes: Left border with background tint
- Code blocks: Pre-formatted font with language badge
- Dividers: Horizontal rule
- Paragraphs: Plain text with standard line-height

**Indentation:** Applied via padding-left or margin-left, scales with indent level.

**Status:** All block types styled and tested. Number counter plugin implemented via decorations.

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

### 9.1 Unit Tests ✓ MOSTLY COMPLETE

**Completed:**
- Schema: Block creation and attribute handling
- Markdown parser: 23 tests covering various structures
- Markdown serializer: Round-trip MD → blocks → MD
- Block commands: 33 tests (delete, duplicate, convert, indent/outdent)
- Slash commands: Block type conversion
- Floating toolbar: Block type changes
- DOM parser: HTML to block conversion

**Remaining:**
- `blockKeymap.ts`: Enter/Backspace splitting and merging
- `blockInputRules.ts`: Auto-format detection
- Edge cases with deeply nested lists and mixed content

### 9.2 E2E Tests

**File:** `e2e/block-editor.spec.ts`

**Test Scenarios (Pending):**
- Create blocks via input rules
- Drag block to reorder
- Drag list item within list
- Drag list item to different location
- Context menu actions
- Keyboard navigation and editing
- Undo/redo after drag operations

**Status:** Framework ready, tests pending implementation.

---

## Implementation Order

### Track A: Shared Components (COMPLETE)

Phases 3.1-3.3 (RowHandle, RowContextMenu, DropIndicator) — context-agnostic and reusable.

### Track B: Editor Block Model (ACTIVE)

1. ✓ Phase 1: Schema
2. ✓ Phase 2: Markdown parser/serializer
3. ✓ Phase 6: Keyboard and input rules
4. ✓ Phase 7: Styling
5. ✓ Phase 9.1: Unit tests
6. ✓ Phase 3.4: Editor integration
7. ✓ Phase 4: Drag-drop
8. ✓ Phase 5: Context menu
9. ✓ Phase 8: Migration
10. → Phase 9.2: E2E tests (PENDING)

### Track C: Table Integration (DEFERRED)

Phase 3.5 and table-specific features deferred pending API decisions for row reordering.

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
