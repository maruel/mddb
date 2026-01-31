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

## Phase 1: Schema Redesign

### 1.1 Define New Block Node

Replace all block-level nodes with a single `block` node type.

**File:** `src/components/editor/schema.ts` (new file)

```typescript
// Block attributes
interface BlockAttrs {
  type: 'paragraph' | 'heading' | 'bullet' | 'number' | 'task' | 'quote' | 'code' | 'divider';
  level?: number;      // heading level (1-6)
  indent: number;      // nesting depth (0+)
  checked?: boolean;   // task completion state
  language?: string;   // code block language
}
```

**Schema definition:**
```typescript
const blockSpec: NodeSpec = {
  attrs: {
    type: { default: 'paragraph' },
    level: { default: null },
    indent: { default: 0 },
    checked: { default: null },
    language: { default: null },
  },
  content: 'inline*',
  group: 'block',
  parseDOM: [...],  // See Phase 1.2
  toDOM(node) {...}, // See Phase 1.3
};
```

**Tasks:**
- [x] Create `src/components/editor/schema.ts`
- [x] Define `BlockAttrs` interface
- [x] Define `blockSpec` with all attributes
- [x] Define `dividerSpec` for horizontal rules (empty block variant)
- [x] Export new schema with marks from current config

### 1.2 DOM Parsing (HTML → ProseMirror)

Map HTML elements to block attributes for clipboard paste and initial load.

**File:** `src/components/editor/dom-parser.ts` (new file)

Implements preprocessing for pasted HTML:
- `transformPastedHTML(html)`: Walks DOM and annotates `<li>` elements with `data-type` (bullet/number) and `data-indent`
- `annotateListItems(node, indentLevel)`: Recursive function that handles nested lists and task detection
- `annotateListItem(li, listType, indentLevel)`: Annotates a single `<li>` with attributes and task class
- `extractCodeLanguage(preElement)`: Extracts code block language from class names or data attributes

**File:** `src/components/editor/domParsePlugin.ts` (new file)

Integrates preprocessing with editor:
- `domParsePlugin()`: Returns Plugin instance for future integration
- `setupPasteHandler()`: Attaches paste event handler to container
- `createHTMLTransformer()`: Returns function for ClipboardParser configuration

**Schema enhancement:** Updated code block parseDOM to extract language from `language-*`, `lang-*`, and `hljs-*` class patterns.

**Challenge solved:** `<ul>` vs `<ol>` detection requires walking the DOM tree. Solution: Preprocess pasted HTML with `transformPastedHTML()` to annotate `<li>` elements with parent list type before schema parsing.

**Tasks:**
- [x] Create `src/components/editor/dom-parser.ts` with preprocessing utilities
- [x] Create `src/components/editor/domParsePlugin.ts` with plugin integration
- [x] Implement `parseDOM` rules for all block types (already in schema.ts)
- [x] Add `transformPastedHTML` to detect `<ul>` vs `<ol>` and annotate `<li>` elements
- [x] Handle nested list indentation (count parent `<li>` depth)
- [x] Unit tests for HTML → block conversion (20 tests, all passing)

### 1.3 DOM Serialization (ProseMirror → HTML)

Already implemented in `schema.ts` during Phase 1.1. The `blockSpec.toDOM()` method renders blocks with appropriate HTML and data attributes:

- Headings → `<h1>` through `<h6>` with `data-type` and `data-indent`
- Bullet/Number lists → `<div class="block-bullet">` or `<div class="block-number">`
- Task lists → `<div class="block-task" data-checked="true/false">`
- Quotes → `<blockquote>` with data attributes
- Code blocks → `<pre data-language="...">` with language metadata
- Dividers → `<hr>` element
- Paragraphs → `<p>` with data attributes

All blocks carry `data-type` and `data-indent` attributes for drag-drop positioning and reconstruction.

**Tasks:**
- [x] Implement `toDOM` for all block types (already in schema.ts)
- [x] Add CSS classes for list-style rendering (bullets, numbers)
- [x] Ensure data attributes support drag-drop and handle positioning

---

## Phase 2: Markdown Serialization

### 2.1 Markdown Parser (MD → Blocks)

Convert markdown to flat blocks, tracking indent from nested list structure.

**File:** `src/components/editor/markdown-parser.ts` ✓ Created

**Strategy:** Parse markdown using the existing MarkdownParser (which correctly handles nested structures), then flatten the result to flat blocks by walking the tree and converting nested lists to blocks with indent attributes.

**Implementation:**
- Uses `MarkdownParser` from `prosemirror-markdown` with the nested schema (`prosemirror-config.ts`)
- Parses markdown with markdown-it (with task list and underline support)
- Recursively flattens nested structure: `flattenDocument()` → `flattenList()` → `flattenListItem()`
- Task lists detected via `checked` attribute on list items
- Code block language extracted from `params` attribute

**Tasks:**
- [x] Create `markdown-parser.ts`
- [x] Implement flattening logic for nested structures
- [x] Handle task list checkbox detection
- [x] Handle code block language extraction
- [x] Unit tests for various markdown structures (23 tests, all passing)

### 2.2 Markdown Serializer (Blocks → MD)

Reconstruct nested markdown from flat blocks by grouping consecutive list items.

**File:** `src/components/editor/markdown-serializer.ts` ✓ Created

**Strategy:**
1. Walk blocks sequentially
2. Track "open" list contexts (type + indent level) to know when to close/open lists
3. Emit list markers and indentation based on context changes
4. Maintain separate number counter per indent level to support proper numbering in nested lists

**Implementation:**
- `serializeToMarkdown()`: Main entry point, walks blocks and builds markdown lines
- Maintains `listContext` stack to track which lists are currently open
- Number counters keyed by indent to support per-level numbering
- Handles all block types: heading, paragraph, bullet, number, task, quote, code, divider
- Properly closes lists when indent decreases or type changes at same indent
- `serializeInline()`: Converts block content with marks (bold, italic, code, links, strikethrough, underline) to markdown syntax

**Edge cases handled:**
- Numbered list restart after paragraph interruption
- Mixed list types at same indent
- Code blocks preserve literal content (no prefix)
- Task lists with mixed checked/unchecked states
- Nested lists with varying indentation

**Tasks:**
- [x] Create `markdown-serializer.ts`
- [x] Implement list context tracking
- [x] Handle numbered list counter logic per indent
- [x] Handle code block fencing and language
- [x] Round-trip tests (MD → blocks → MD, all passing)

---

## Phase 3: Shared Row Handle System

The row handle must be reusable across two contexts:
1. **Editor blocks** — ProseMirror document blocks (paragraphs, lists, headings)
2. **Table rows** — Records in Table, Grid, Gallery, and Board views

Both share identical appearance and behavior but differ in their data model and drag-drop mechanics.

### 3.1 Shared Handle Component

**File:** `src/components/shared/RowHandle.tsx` (new file)
**File:** `src/components/shared/RowHandle.module.css` (new file)

A context-agnostic handle component that receives callbacks for drag and context menu.

```tsx
export interface RowHandleProps {
  /** Unique identifier for the row (block position, record ID, etc.) */
  rowId: string;
  /** Called when drag starts - consumer sets up drag data */
  onDragStart: (e: DragEvent, rowId: string) => void;
  /** Called on right-click - consumer shows context menu */
  onContextMenu: (e: MouseEvent, rowId: string) => void;
  /** Optional: called on handle click (e.g., to select row) */
  onClick?: (e: MouseEvent, rowId: string) => void;
  /** Optional: additional CSS class */
  class?: string;
}

export function RowHandle(props: RowHandleProps) {
  const handleDragStart = (e: DragEvent) => {
    e.stopPropagation();
    props.onDragStart(e, props.rowId);
  };

  const handleContextMenu = (e: MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    props.onContextMenu(e, props.rowId);
  };

  const handleClick = (e: MouseEvent) => {
    e.stopPropagation();
    props.onClick?.(e, props.rowId);
  };

  return (
    <div
      class={`${styles.handle} ${props.class || ''}`}
      draggable="true"
      onDragStart={handleDragStart}
      onContextMenu={handleContextMenu}
      onClick={handleClick}
      role="button"
      aria-label="Drag handle"
      tabIndex={-1}
    >
      <svg class={styles.icon} viewBox="0 0 10 16" fill="currentColor">
        <circle cx="3" cy="3" r="1.5" />
        <circle cx="7" cy="3" r="1.5" />
        <circle cx="3" cy="8" r="1.5" />
        <circle cx="7" cy="8" r="1.5" />
        <circle cx="3" cy="13" r="1.5" />
        <circle cx="7" cy="13" r="1.5" />
      </svg>
    </div>
  );
}
```

**CSS (RowHandle.module.css):**
```css
.handle {
  position: absolute;
  left: -24px;
  top: 50%;
  transform: translateY(-50%);
  width: 18px;
  height: 24px;
  opacity: 0;
  cursor: grab;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 3px;
  color: var(--c-text-muted);
  transition: opacity 0.1s, background 0.1s, color 0.1s;
  user-select: none;
}

.handle:hover {
  background: var(--c-bg-hover);
  color: var(--c-text);
}

.handle:active {
  cursor: grabbing;
  background: var(--c-bg-active);
}

.icon {
  width: 10px;
  height: 16px;
}

/* Parent row controls visibility via this class */
:global(.row-with-handle):hover .handle,
:global(.row-with-handle):focus-within .handle {
  opacity: 1;
}

/* Always show when dragging */
:global(.row-with-handle.dragging) .handle {
  opacity: 1;
}
```

**Tasks:**
- [x] Create `src/components/shared/RowHandle.tsx`
- [x] Create `src/components/shared/RowHandle.module.css`
- [x] Use SVG icon instead of text character for crisp rendering
- [x] Add accessibility attributes (role, aria-label)
- [x] Export from `src/components/shared/index.ts`

### 3.2 Shared Context Menu Component

**File:** `src/components/shared/RowContextMenu.tsx` (new file)
**File:** `src/components/shared/RowContextMenu.module.css` (new file)

A generic context menu that receives action definitions from the consumer.

```tsx
export interface ContextMenuAction {
  id: string;
  label: string;
  icon?: JSX.Element;
  shortcut?: string;
  disabled?: boolean;
  danger?: boolean;  // Red text for destructive actions
  separator?: boolean;  // Render separator before this item
}

export interface RowContextMenuProps {
  position: { x: number; y: number };
  actions: ContextMenuAction[];
  onAction: (actionId: string) => void;
  onClose: () => void;
}

export function RowContextMenu(props: RowContextMenuProps) {
  // Click-outside and Escape handling
  // Position adjustment to stay in viewport
  // Keyboard navigation (arrow keys)
  return (
    <Portal>
      <div
        class={styles.menu}
        style={{ left: `${props.position.x}px`, top: `${props.position.y}px` }}
        role="menu"
      >
        <For each={props.actions}>
          {(action) => (
            <>
              <Show when={action.separator}>
                <div class={styles.separator} />
              </Show>
              <button
                class={styles.item}
                classList={{ [styles.danger]: action.danger, [styles.disabled]: action.disabled }}
                onClick={() => !action.disabled && props.onAction(action.id)}
                role="menuitem"
                disabled={action.disabled}
              >
                <Show when={action.icon}>
                  <span class={styles.icon}>{action.icon}</span>
                </Show>
                <span class={styles.label}>{action.label}</span>
                <Show when={action.shortcut}>
                  <span class={styles.shortcut}>{action.shortcut}</span>
                </Show>
              </button>
            </>
          )}
        </For>
      </div>
    </Portal>
  );
}
```

**Tasks:**
- [x] Create `RowContextMenu.tsx` with action-based API
- [x] Create `RowContextMenu.module.css`
- [x] Implement click-outside to close
- [x] Implement Escape key to close
- [x] Implement keyboard navigation (↑/↓/Enter)
- [x] Viewport boundary detection for position adjustment

### 3.3 Shared Drop Indicator Component

**File:** `src/components/shared/DropIndicator.tsx` (new file)
**File:** `src/components/shared/DropIndicator.module.css` (new file)

Visual indicator shown between rows during drag-drop.

```tsx
export interface DropIndicatorProps {
  /** Y position relative to container */
  y: number;
  /** Whether indicator is visible */
  visible: boolean;
  /** Optional: full width or indented */
  indent?: number;
}

export function DropIndicator(props: DropIndicatorProps) {
  return (
    <Show when={props.visible}>
      <div
        class={styles.indicator}
        style={{
          top: `${props.y}px`,
          left: `${(props.indent || 0) * 24}px`,
        }}
      />
    </Show>
  );
}
```

```css
.indicator {
  position: absolute;
  left: 0;
  right: 0;
  height: 2px;
  background: var(--c-primary);
  pointer-events: none;
  z-index: 100;
  border-radius: 1px;
}

.indicator::before,
.indicator::after {
  content: '';
  position: absolute;
  top: -3px;
  width: 8px;
  height: 8px;
  background: var(--c-primary);
  border-radius: 50%;
}

.indicator::before {
  left: -4px;
}

.indicator::after {
  right: -4px;
}
```

**Tasks:**
- [x] Create `DropIndicator.tsx`
- [x] Create `DropIndicator.module.css`
- [x] Support indent offset for nested blocks

### 3.4 Editor Integration (ProseMirror)

**File:** `src/components/editor/BlockNodeView.ts` (new file)

Integrates shared components with ProseMirror's NodeView system.

```typescript
import { RowHandle } from '../shared/RowHandle';
import { render } from 'solid-js/web';

class BlockNodeView implements NodeView {
  dom: HTMLElement;
  contentDOM: HTMLElement;
  private handleDispose: (() => void) | null = null;

  constructor(
    node: ProseMirrorNode,
    private view: EditorView,
    private getPos: () => number | undefined
  ) {
    this.dom = document.createElement('div');
    this.dom.className = 'block-row row-with-handle';

    // Mount SolidJS handle component
    const handleContainer = document.createElement('div');
    this.handleDispose = render(
      () => RowHandle({
        rowId: String(getPos()),
        onDragStart: this.handleDragStart.bind(this),
        onContextMenu: this.handleContextMenu.bind(this),
      }),
      handleContainer
    );

    this.contentDOM = document.createElement('div');
    this.contentDOM.className = 'block-content';

    this.dom.appendChild(handleContainer.firstChild!);
    this.dom.appendChild(this.contentDOM);
  }

  private handleDragStart(e: DragEvent, rowId: string) {
    const pos = this.getPos();
    if (pos === undefined) return;
    e.dataTransfer?.setData('application/x-prosemirror-block', String(pos));
    e.dataTransfer!.effectAllowed = 'move';
    this.view.dispatch(this.view.state.tr.setMeta('blockDrag', { sourcePos: pos }));
  }

  private handleContextMenu(e: MouseEvent, rowId: string) {
    const pos = this.getPos();
    if (pos === undefined) return;
    // Dispatch event or call context to show menu
    this.view.dom.dispatchEvent(new CustomEvent('block-context-menu', {
      detail: { pos, x: e.clientX, y: e.clientY },
      bubbles: true,
    }));
  }

  destroy() {
    this.handleDispose?.();
  }
}
```

**Editor context menu actions:**
```typescript
const editorBlockActions: ContextMenuAction[] = [
  { id: 'delete', label: t('editor.deleteBlock'), shortcut: '⌫', danger: true },
  { id: 'duplicate', label: t('editor.duplicateBlock'), shortcut: '⌘D' },
  { id: 'convert', label: t('editor.convertTo'), separator: true },
  // Submenu or inline options for block type conversion
];
```

**Tasks:**
- [ ] Create `BlockNodeView.ts` using shared `RowHandle`
- [ ] Mount SolidJS component within vanilla JS NodeView
- [ ] Wire up drag-start to ProseMirror plugin
- [ ] Wire up context-menu to editor state
- [ ] Implement editor-specific context menu actions

### 3.5 Table Integration

**File:** `src/components/table/TableRow.tsx` (new file or modify existing)

Integrates shared components with table view rows.

```tsx
import { RowHandle } from '../shared/RowHandle';
import { RowContextMenu } from '../shared/RowContextMenu';

interface TableRowProps {
  record: Record;
  onReorder: (recordId: string, targetIndex: number) => void;
  onDelete: (recordId: string) => void;
  onDuplicate: (recordId: string) => void;
  // ... other props
}

export function TableRow(props: TableRowProps) {
  const { t } = useI18n();
  const [menuState, setMenuState] = createSignal<{ x: number; y: number } | null>(null);

  const handleDragStart = (e: DragEvent, rowId: string) => {
    e.dataTransfer?.setData('application/x-table-record', rowId);
    e.dataTransfer!.effectAllowed = 'move';
  };

  const handleContextMenu = (e: MouseEvent, rowId: string) => {
    setMenuState({ x: e.clientX, y: e.clientY });
  };

  const tableRowActions: ContextMenuAction[] = [
    { id: 'open', label: t('table.openRecord') },
    { id: 'duplicate', label: t('table.duplicateRecord'), shortcut: '⌘D' },
    { id: 'delete', label: t('table.deleteRecord'), shortcut: '⌫', danger: true, separator: true },
  ];

  const handleAction = (actionId: string) => {
    switch (actionId) {
      case 'delete': props.onDelete(props.record.id); break;
      case 'duplicate': props.onDuplicate(props.record.id); break;
      case 'open': /* navigate to record */ break;
    }
    setMenuState(null);
  };

  return (
    <div class={`${styles.row} row-with-handle`}>
      <RowHandle
        rowId={props.record.id}
        onDragStart={handleDragStart}
        onContextMenu={handleContextMenu}
      />
      {/* Row content... */}

      <Show when={menuState()}>
        <RowContextMenu
          position={menuState()!}
          actions={tableRowActions}
          onAction={handleAction}
          onClose={() => setMenuState(null)}
        />
      </Show>
    </div>
  );
}
```

**Tasks:**
- [ ] Create `TableRow.tsx` wrapper using shared `RowHandle`
- [ ] Define table-specific context menu actions
- [ ] Implement record drag-drop reordering
- [ ] Apply to TableTable, TableGrid, TableGallery, TableBoard views

### 3.6 Layout Adjustments

Add left margin to containers that use row handles.

**Shared CSS pattern (add to each view's module.css):**
```css
/* Container needs left padding for handles */
.container {
  position: relative;
  padding-left: 28px;  /* Space for handle (24px) + gap (4px) */
}

/* For print/export, hide handles */
@media print {
  :global(.row-with-handle) :global(.handle) {
    display: none;
  }
  .container {
    padding-left: 0;
  }
}
```

**Tasks:**
- [ ] Add left padding to editor container
- [ ] Add left padding to table view containers
- [ ] Add print media query to hide handles
- [ ] Test with various viewport widths

### 3.7 Component Summary

| Component | Location | Purpose |
|-----------|----------|---------|
| `RowHandle` | `shared/RowHandle.tsx` | Drag handle UI, context-agnostic |
| `RowContextMenu` | `shared/RowContextMenu.tsx` | Action menu UI, receives action list |
| `DropIndicator` | `shared/DropIndicator.tsx` | Visual drop target line |
| `BlockNodeView` | `editor/BlockNodeView.ts` | ProseMirror integration |
| `TableRow` | `table/TableRow.tsx` | Table view integration |

### 3.8 Multi-Block Selection Behavior

When the user's selection spans multiple blocks, special handle behavior applies:

**Requirements:**
1. Only the **topmost block's handle** is visible; other selected blocks hide their handles
2. Dragging the visible handle moves **all selected blocks** as a unit
3. Context menu actions apply to **all selected blocks** (e.g., delete all, convert all)
4. Visual feedback shows which blocks are part of the selection

**Implementation:**

```typescript
// In editor, track which blocks are in current selection
function getSelectedBlockPositions(state: EditorState): number[] {
  const { from, to } = state.selection;
  const positions: number[] = [];

  state.doc.nodesBetween(from, to, (node, pos) => {
    if (node.isBlock && pos >= 0) {
      positions.push(pos);
    }
    return true;  // Continue traversal
  });

  return positions;
}

// BlockNodeView checks if it should show handle
class BlockNodeView implements NodeView {
  update(node: ProseMirrorNode) {
    const selectedPositions = getSelectedBlockPositions(this.view.state);
    const myPos = this.getPos();

    if (selectedPositions.length > 1) {
      // Multi-block selection: only first block shows handle
      const isFirst = selectedPositions[0] === myPos;
      this.handle.style.display = isFirst ? '' : 'none';
      this.dom.classList.toggle('in-selection', selectedPositions.includes(myPos!));
    } else {
      // Single block or no selection: show handle normally
      this.handle.style.display = '';
      this.dom.classList.remove('in-selection');
    }
    return true;
  }
}
```

**CSS for selection feedback:**
```css
/* Highlight selected blocks */
:global(.block-row.in-selection) {
  background: var(--c-selection-bg);
  border-radius: 3px;
}

/* Hide handles on non-first selected blocks */
:global(.block-row.in-selection:not(:first-child)) .handle {
  display: none !important;
}
```

**Drag behavior for multi-selection:**
```typescript
handleDragStart(e: DragEvent, rowId: string) {
  const selectedPositions = getSelectedBlockPositions(this.view.state);

  if (selectedPositions.length > 1) {
    // Serialize all selected blocks
    const blocks = selectedPositions.map(pos => this.view.state.doc.nodeAt(pos));
    e.dataTransfer?.setData('application/x-prosemirror-blocks',
      JSON.stringify(selectedPositions));
  } else {
    // Single block
    e.dataTransfer?.setData('application/x-prosemirror-block', rowId);
  }
  e.dataTransfer!.effectAllowed = 'move';
}
```

**Context menu for multi-selection:**
```typescript
const getEditorBlockActions = (selectedCount: number): ContextMenuAction[] => [
  {
    id: 'delete',
    label: selectedCount > 1
      ? t('editor.deleteBlocks', { count: selectedCount })
      : t('editor.deleteBlock'),
    shortcut: '⌫',
    danger: true
  },
  {
    id: 'duplicate',
    label: selectedCount > 1
      ? t('editor.duplicateBlocks', { count: selectedCount })
      : t('editor.duplicateBlock'),
    shortcut: '⌘D'
  },
  // Convert only shown for single selection (mixed types don't convert well)
  ...(selectedCount === 1 ? [
    { id: 'convert', label: t('editor.convertTo'), separator: true }
  ] : []),
];
```

**Tasks:**
- [ ] Implement `getSelectedBlockPositions()` helper
- [ ] Update `BlockNodeView.update()` to show/hide handles based on selection
- [ ] Add `.in-selection` CSS class for visual feedback
- [ ] Update drag handler to serialize multiple block positions
- [ ] Update drop handler to move multiple blocks
- [ ] Update context menu to show count and apply actions to all
- [ ] Add i18n keys for plural forms (`deleteBlocks`, `duplicateBlocks`)

---

## Phase 4: Drag-and-Drop System

### 4.1 Drag State Management

**File:** `src/components/editor/blockDragPlugin.ts` (new file)

Track drag source and show drop indicators.

```typescript
interface DragState {
  sourcePos: number | null;      // Block being dragged
  dropTarget: number | null;     // Position to drop (between blocks)
  dropIndicatorY: number | null; // Visual indicator position
}

const blockDragPlugin = new Plugin<DragState>({
  state: {
    init: () => ({ sourcePos: null, dropTarget: null, dropIndicatorY: null }),
    apply(tr, state) {
      const meta = tr.getMeta(blockDragPlugin);
      if (meta) return { ...state, ...meta };
      return state;
    },
  },
  props: {
    decorations(state) {
      const { dropIndicatorY } = this.getState(state);
      if (dropIndicatorY === null) return null;
      // Return line decoration at dropIndicatorY
    },
  },
});
```

**Tasks:**
- [x] Create `blockDragPlugin.ts`
- [x] Define drag state interface
- [x] Implement state transitions
- [x] Add drop indicator decoration

### 4.2 Drag Event Handlers

Handle dragstart, dragover, and drop on the editor.

```typescript
// In BlockNodeView or plugin
onDragStart(e: DragEvent, view: EditorView, getPos: () => number | undefined) {
  const pos = getPos();
  if (pos === undefined) return;

  // Set drag data
  e.dataTransfer?.setData('application/x-prosemirror-block', String(pos));
  e.dataTransfer!.effectAllowed = 'move';

  // Update plugin state
  view.dispatch(view.state.tr.setMeta(blockDragPlugin, { sourcePos: pos }));
}

onDragOver(e: DragEvent, view: EditorView) {
  e.preventDefault();
  const coords = { left: e.clientX, top: e.clientY };
  const pos = view.posAtCoords(coords);
  if (!pos) return;

  // Find nearest block boundary
  const $pos = view.state.doc.resolve(pos.pos);
  const blockStart = $pos.before(1);
  const blockEnd = $pos.after(1);

  // Determine if dropping above or below
  const rect = view.coordsAtPos(blockStart);
  const dropAbove = e.clientY < rect.top + (rect.bottom - rect.top) / 2;
  const dropPos = dropAbove ? blockStart : blockEnd;

  view.dispatch(view.state.tr.setMeta(blockDragPlugin, {
    dropTarget: dropPos,
    dropIndicatorY: dropAbove ? rect.top : rect.bottom,
  }));
}

onDrop(e: DragEvent, view: EditorView) {
  e.preventDefault();
  const sourcePos = parseInt(e.dataTransfer?.getData('application/x-prosemirror-block') || '', 10);
  const { dropTarget } = blockDragPlugin.getState(view.state);

  if (isNaN(sourcePos) || dropTarget === null) return;

  // Execute move transaction
  const tr = view.state.tr;
  const node = view.state.doc.nodeAt(sourcePos);
  if (!node) return;

  // Delete from source, insert at target (adjust positions as needed)
  // ... transaction logic

  view.dispatch(tr.setMeta(blockDragPlugin, { sourcePos: null, dropTarget: null, dropIndicatorY: null }));
}
```

**Tasks:**
- [x] Implement `onDragStart` with data transfer
- [x] Implement `onDragOver` with position calculation
- [x] Implement `onDrop` with move transaction
- [x] Handle position adjustment when source is before target
- [x] Clear drag state on dragend/dragleave

### 4.3 Drop Indicator Styling

**File:** `src/components/editor/Editor.module.css` (modify)

```css
.dropIndicator {
  position: absolute;
  left: 0;
  right: 0;
  height: 2px;
  background: var(--c-primary);
  pointer-events: none;
  z-index: 10;
}
```

**Tasks:**
- [ ] Style drop indicator line
- [ ] Animate indicator appearance
- [ ] Ensure indicator doesn't interfere with content

---

## Phase 5: Context Menu

### 5.1 Menu Component

**File:** `src/components/editor/BlockContextMenu.tsx` (new file)
**File:** `src/components/editor/BlockContextMenu.module.css` (new file)

```tsx
interface BlockContextMenuProps {
  position: { x: number; y: number };
  blockPos: number;
  onClose: () => void;
  onAction: (action: BlockAction) => void;
}

type BlockAction =
  | { type: 'delete' }
  | { type: 'duplicate' }
  | { type: 'convert'; to: BlockType }
  | { type: 'indent' }
  | { type: 'outdent' };

export function BlockContextMenu(props: BlockContextMenuProps) {
  const { t } = useI18n();

  return (
    <Portal>
      <div
        class={styles.menu}
        style={{ left: `${props.position.x}px`, top: `${props.position.y}px` }}
      >
        <button onClick={() => props.onAction({ type: 'delete' })}>
          {t('editor.deleteBlock')}
        </button>
        <button onClick={() => props.onAction({ type: 'duplicate' })}>
          {t('editor.duplicateBlock')}
        </button>
        <div class={styles.separator} />
        <span class={styles.label}>{t('editor.convertTo')}</span>
        <button onClick={() => props.onAction({ type: 'convert', to: 'paragraph' })}>
          {t('editor.paragraph')}
        </button>
        <button onClick={() => props.onAction({ type: 'convert', to: 'heading' })}>
          {t('editor.heading')}
        </button>
        {/* ... other block types */}
      </div>
    </Portal>
  );
}
```

**Tasks:**
- [ ] Create `BlockContextMenu.tsx`
- [ ] Create `BlockContextMenu.module.css`
- [ ] Add i18n keys for all menu items
- [ ] Implement click-outside to close
- [ ] Implement Escape key to close

### 5.2 Menu Actions

Implement ProseMirror commands for each action.

**File:** `src/components/editor/blockCommands.ts` (new file)

```typescript
export function deleteBlock(pos: number): Command {
  return (state, dispatch) => {
    const $pos = state.doc.resolve(pos);
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    if (dispatch) {
      dispatch(state.tr.delete(pos, pos + node.nodeSize));
    }
    return true;
  };
}

export function duplicateBlock(pos: number): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    if (dispatch) {
      dispatch(state.tr.insert(pos + node.nodeSize, node.copy(node.content)));
    }
    return true;
  };
}

export function convertBlock(pos: number, toType: BlockType, attrs?: Partial<BlockAttrs>): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    if (dispatch) {
      dispatch(state.tr.setNodeMarkup(pos, undefined, { ...node.attrs, type: toType, ...attrs }));
    }
    return true;
  };
}

export function indentBlock(pos: number): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    const currentIndent = node.attrs.indent || 0;
    if (currentIndent >= 8) return false;  // Max indent

    if (dispatch) {
      dispatch(state.tr.setNodeMarkup(pos, undefined, { ...node.attrs, indent: currentIndent + 1 }));
    }
    return true;
  };
}

export function outdentBlock(pos: number): Command {
  return (state, dispatch) => {
    const node = state.doc.nodeAt(pos);
    if (!node) return false;

    const currentIndent = node.attrs.indent || 0;
    if (currentIndent <= 0) return false;

    if (dispatch) {
      dispatch(state.tr.setNodeMarkup(pos, undefined, { ...node.attrs, indent: currentIndent - 1 }));
    }
    return true;
  };
}
```

**Tasks:**
- [x] Create `blockCommands.ts`
- [x] Implement `deleteBlock`
- [x] Implement `duplicateBlock`
- [x] Implement `convertBlock`
- [x] Implement `indentBlock` / `outdentBlock`
- [ ] Unit tests for each command

---

## Phase 6: Keyboard Commands

### 6.1 List Behavior Reimplementation

Since lists are now flat blocks, standard ProseMirror list commands won't work. Implement custom behavior.

**File:** `src/components/editor/blockKeymap.ts` (new file)

```typescript
export function buildBlockKeymap() {
  return keymap({
    // Tab: Increase indent (for list-like blocks)
    'Tab': (state, dispatch) => {
      const { $from } = state.selection;
      const node = state.doc.nodeAt($from.before(1));
      if (!node) return false;

      const { type, indent } = node.attrs;
      if (!['bullet', 'number', 'task'].includes(type)) return false;
      if (indent >= 8) return false;

      if (dispatch) {
        dispatch(state.tr.setNodeMarkup($from.before(1), undefined, { ...node.attrs, indent: indent + 1 }));
      }
      return true;
    },

    // Shift-Tab: Decrease indent
    'Shift-Tab': (state, dispatch) => {
      const { $from } = state.selection;
      const node = state.doc.nodeAt($from.before(1));
      if (!node) return false;

      const { indent } = node.attrs;
      if (indent <= 0) return false;

      if (dispatch) {
        dispatch(state.tr.setNodeMarkup($from.before(1), undefined, { ...node.attrs, indent: indent - 1 }));
      }
      return true;
    },

    // Enter: Create new block of same type (for lists)
    'Enter': (state, dispatch) => {
      const { $from, empty } = state.selection;
      if (!empty) return false;

      const node = state.doc.nodeAt($from.before(1));
      if (!node) return false;

      const { type, indent } = node.attrs;

      // If in a list-type block
      if (['bullet', 'number', 'task'].includes(type)) {
        // If block is empty, convert to paragraph and outdent
        if (node.content.size === 0) {
          if (dispatch) {
            const newAttrs = indent > 0
              ? { type, indent: indent - 1 }
              : { type: 'paragraph', indent: 0 };
            dispatch(state.tr.setNodeMarkup($from.before(1), undefined, newAttrs));
          }
          return true;
        }

        // Split block and create new list item
        if (dispatch) {
          const tr = state.tr;
          const newBlock = schema.nodes.block.create(
            { type, indent, checked: type === 'task' ? false : null },
            null
          );
          // Split at cursor, insert new block after
          const splitPos = $from.pos;
          const afterContent = node.cut($from.parentOffset);
          tr.delete(splitPos, $from.after(1) - 1);  // Delete content after cursor
          tr.insert($from.after(1), newBlock);
          if (afterContent.content.size > 0) {
            tr.insert($from.after(1) + 1, afterContent.content);
          }
          tr.setSelection(TextSelection.create(tr.doc, $from.after(1) + 1));
          dispatch(tr);
        }
        return true;
      }

      return false;  // Let default Enter handling take over
    },

    // Backspace at start: Merge with previous or convert to paragraph
    'Backspace': (state, dispatch) => {
      const { $from, empty } = state.selection;
      if (!empty || $from.parentOffset !== 0) return false;

      const node = state.doc.nodeAt($from.before(1));
      if (!node) return false;

      const { type, indent } = node.attrs;

      // If indented, outdent first
      if (indent > 0) {
        if (dispatch) {
          dispatch(state.tr.setNodeMarkup($from.before(1), undefined, { ...node.attrs, indent: indent - 1 }));
        }
        return true;
      }

      // If list type, convert to paragraph
      if (['bullet', 'number', 'task', 'quote'].includes(type)) {
        if (dispatch) {
          dispatch(state.tr.setNodeMarkup($from.before(1), undefined, { type: 'paragraph', indent: 0 }));
        }
        return true;
      }

      return false;  // Let default backspace handle merging
    },
  });
}
```

**Tasks:**
- [x] Create `blockKeymap.ts`
- [x] Implement Tab/Shift-Tab for indent
- [x] Implement Enter for block splitting
- [x] Implement Backspace for outdent/convert
- [ ] Test all keyboard interactions

### 6.2 Input Rules Update

Update input rules to set block attributes instead of wrapping in containers.

**File:** `src/components/editor/blockInputRules.ts` (new file)

```typescript
export function buildBlockInputRules() {
  return inputRules({
    rules: [
      // Bullet list: - at start
      new InputRule(/^\s*[-*]\s$/, (state, match, start, end) => {
        return state.tr
          .delete(start, end)
          .setNodeMarkup(state.doc.resolve(start).before(1), undefined, { type: 'bullet', indent: 0 });
      }),

      // Numbered list: 1. at start
      new InputRule(/^\s*(\d+)\.\s$/, (state, match, start, end) => {
        return state.tr
          .delete(start, end)
          .setNodeMarkup(state.doc.resolve(start).before(1), undefined, { type: 'number', indent: 0 });
      }),

      // Task list: - [ ] or - [x] at start
      new InputRule(/^\s*[-*]\s+\[([ xX])\]\s$/, (state, match, start, end) => {
        const checked = match[1]?.toLowerCase() === 'x';
        return state.tr
          .delete(start, end)
          .setNodeMarkup(state.doc.resolve(start).before(1), undefined, { type: 'task', indent: 0, checked });
      }),

      // Headings: # ## ### etc
      ...([1, 2, 3, 4, 5, 6].map(level =>
        new InputRule(new RegExp(`^#{${level}}\\s$`), (state, match, start, end) => {
          return state.tr
            .delete(start, end)
            .setNodeMarkup(state.doc.resolve(start).before(1), undefined, { type: 'heading', level, indent: 0 });
        })
      )),

      // Blockquote: > at start
      new InputRule(/^\s*>\s$/, (state, match, start, end) => {
        return state.tr
          .delete(start, end)
          .setNodeMarkup(state.doc.resolve(start).before(1), undefined, { type: 'quote', indent: 0 });
      }),

      // Code block: ``` at start
      new InputRule(/^```(\w*)$/, (state, match, start, end) => {
        return state.tr
          .delete(start, end)
          .setNodeMarkup(state.doc.resolve(start).before(1), undefined, { type: 'code', language: match[1] || null, indent: 0 });
      }),

      // Divider: --- at start of empty block
      new InputRule(/^---$/, (state, match, start, end) => {
        const $start = state.doc.resolve(start);
        const block = state.doc.nodeAt($start.before(1));
        if (block && block.content.size === 3) {  // Only "---"
          return state.tr
            .delete(start, end)
            .setNodeMarkup($start.before(1), undefined, { type: 'divider', indent: 0 });
        }
        return null;
      }),

      // Smart typography
      ...smartQuotes,
      ellipsis,
      emDash,
    ],
  });
}
```

**Tasks:**
- [x] Create `blockInputRules.ts`
- [x] Implement all input rules for block type conversion
- [ ] Test each input rule
- [ ] Ensure rules don't conflict

---

## Phase 7: Styling

### 7.1 Block Type Styling

Update CSS to render list markers, indentation, and block-specific styles.

**File:** `src/components/editor/Editor.module.css` (modify)

```css
/* Block row container */
.prosemirrorEditor :global(.block-row) {
  position: relative;
  display: flex;
  align-items: flex-start;
}

/* Indentation */
.prosemirrorEditor :global([data-indent="1"]) { padding-left: 24px; }
.prosemirrorEditor :global([data-indent="2"]) { padding-left: 48px; }
.prosemirrorEditor :global([data-indent="3"]) { padding-left: 72px; }
.prosemirrorEditor :global([data-indent="4"]) { padding-left: 96px; }
.prosemirrorEditor :global([data-indent="5"]) { padding-left: 120px; }
.prosemirrorEditor :global([data-indent="6"]) { padding-left: 144px; }
.prosemirrorEditor :global([data-indent="7"]) { padding-left: 168px; }
.prosemirrorEditor :global([data-indent="8"]) { padding-left: 192px; }

/* Bullet list marker */
.prosemirrorEditor :global(.block-bullet)::before {
  content: '•';
  position: absolute;
  left: calc(var(--indent-offset, 0px) - 16px);
  color: var(--c-text-muted);
}

/* Numbered list - requires counter management via CSS or JS */
.prosemirrorEditor :global(.block-number)::before {
  content: attr(data-number) '.';
  position: absolute;
  left: calc(var(--indent-offset, 0px) - 24px);
  color: var(--c-text-muted);
  min-width: 20px;
  text-align: right;
}

/* Task list checkbox */
.prosemirrorEditor :global(.block-task) {
  position: relative;
}

.prosemirrorEditor :global(.block-task)::before {
  content: '';
  position: absolute;
  left: calc(var(--indent-offset, 0px) - 20px);
  top: 4px;
  width: 14px;
  height: 14px;
  border: 1.5px solid var(--c-border);
  border-radius: 3px;
  background: var(--c-bg);
}

.prosemirrorEditor :global(.block-task[data-checked="true"])::before {
  background: var(--c-primary);
  border-color: var(--c-primary);
}

.prosemirrorEditor :global(.block-task[data-checked="true"])::after {
  content: '✓';
  position: absolute;
  left: calc(var(--indent-offset, 0px) - 18px);
  top: 2px;
  font-size: 12px;
  color: white;
}
```

**Note:** Numbered list counters are complex with CSS alone when blocks can be reordered. Consider computing `data-number` attribute in a ProseMirror plugin that runs on every transaction.

**Tasks:**
- [ ] Add indentation CSS
- [ ] Add bullet marker styling
- [ ] Add numbered list marker styling (with counter plugin)
- [ ] Add task checkbox styling
- [ ] Add quote styling (left border)
- [ ] Test all block types at various indent levels

### 7.2 Number Counter Plugin

Compute sequential numbers for numbered list blocks.

```typescript
const numberCounterPlugin = new Plugin({
  appendTransaction(transactions, oldState, newState) {
    // Only recompute if document changed
    if (!transactions.some(tr => tr.docChanged)) return null;

    const tr = newState.tr;
    let needsUpdate = false;
    let counters: Map<number, number> = new Map();  // indent → current count

    newState.doc.forEach((node, pos) => {
      if (node.attrs.type === 'number') {
        const indent = node.attrs.indent || 0;
        const count = (counters.get(indent) || 0) + 1;
        counters.set(indent, count);

        // Reset deeper counters
        for (const [i] of counters) {
          if (i > indent) counters.delete(i);
        }

        // Update data-number attribute if changed
        // (This would require storing number in attrs or using decorations)
      } else {
        // Non-number block resets counters at and below this indent
        const indent = node.attrs.indent || 0;
        for (const [i] of counters) {
          if (i >= indent) counters.delete(i);
        }
      }
    });

    return needsUpdate ? tr : null;
  },
});
```

**Alternative:** Use decorations to add `data-number` without modifying document.

**Tasks:**
- [ ] Implement number counter logic
- [ ] Choose between attrs or decorations approach
- [ ] Test number sequences after reordering

---

## Phase 8: Migration

### 8.1 Data Migration Strategy

Existing documents use nested list structure. Need transparent migration.

**Approach:** Migrate on load, no stored format change.

1. **Load:** Parse markdown → old schema → convert to flat blocks
2. **Save:** Flat blocks → serialize to markdown (produces same markdown)

Since markdown is the storage format and serialization produces valid markdown, no data migration is needed. The new parser/serializer handles the transformation.

**Tasks:**
- [ ] Verify round-trip compatibility (old MD → new schema → MD → old schema → same)
- [ ] Test with existing documents
- [ ] Handle edge cases (deeply nested lists, mixed content)

### 8.2 Feature Flags

Optionally gate new editor behind feature flag during development.

```typescript
// In Editor.tsx
const useBlockEditor = () => {
  // Check localStorage or user preference
  return localStorage.getItem('editor.useBlockEditor') === 'true';
};
```

**Tasks:**
- [ ] Add feature flag for gradual rollout
- [ ] Add setting to toggle between editors
- [ ] Remove flag after stabilization

---

## Phase 9: Testing

### 9.1 Unit Tests

**Files:**
- `src/components/editor/schema.test.ts`
- `src/components/editor/markdown-parser.test.ts`
- `src/components/editor/markdown-serializer.test.ts`
- `src/components/editor/blockCommands.test.ts`
- `src/components/editor/blockKeymap.test.ts`

**Test cases:**
- Schema: Block creation with all attribute combinations
- Parser: Various markdown structures → correct flat blocks
- Serializer: Flat blocks → correct markdown
- Round-trip: MD → blocks → MD produces identical output
- Commands: Delete, duplicate, convert, indent/outdent
- Keymap: Tab, Shift-Tab, Enter, Backspace behaviors

**Tasks:**
- [ ] Unit tests for schema
- [ ] Unit tests for markdown parser
- [ ] Unit tests for markdown serializer
- [ ] Round-trip tests
- [ ] Command tests
- [ ] Keymap tests

### 9.2 E2E Tests

**File:** `e2e/block-editor.spec.ts` (new file)

**Test scenarios:**
- Create blocks via input rules (type `- `, `1. `, `# `, etc.)
- Drag block to reorder
- Drag list item within list
- Drag list item to different location
- Context menu actions
- Keyboard navigation and editing
- Undo/redo after drag operations

**Tasks:**
- [ ] E2E test for block creation
- [ ] E2E test for drag-drop reordering
- [ ] E2E test for context menu
- [ ] E2E test for keyboard shortcuts

---

## Implementation Order

Recommended sequence for incremental delivery:

### Track A: Shared Components (can start immediately)

These components are context-agnostic and can be built and tested in isolation.

1. **Phase 3.1-3.3 (Shared UI)** — `RowHandle`, `RowContextMenu`, `DropIndicator`
2. **Storybook/Visual tests** — Verify appearance and interactions in isolation

### Track B: Editor Block Model (depends on Track A for Phase 3.4)

1. **Phase 1 (Schema)** — Foundation, no visible changes yet
2. **Phase 2 (Markdown)** — Parser/serializer, editor still works
3. **Phase 6 (Keyboard)** — Input rules and keymap, editing works
4. **Phase 7 (Styling)** — Visual rendering correct
5. **Phase 9.1 (Unit Tests)** — Verify core logic
6. **Phase 3.4 (Editor Integration)** — Wire shared components to ProseMirror
7. **Phase 3.8 (Multi-Block Selection)** — Handle visibility and batch operations
8. **Phase 4 (Drag-Drop)** — Core feature complete (single and multi-block)
9. **Phase 5 (Context Menu)** — Editor-specific actions (with multi-block support)
10. **Phase 8 (Migration)** — Ensure compatibility
11. **Phase 9.2 (E2E Tests)** — Full integration verification

### Track C: Table Integration (depends on Track A)

Can proceed in parallel with Track B once shared components exist.

1. **Phase 3.5 (Table Integration)** — Wire shared components to table views
2. **Table drag-drop** — Implement row reordering (requires API decision)
3. **Table context menu** — Record-specific actions (open, duplicate, delete)
4. **E2E tests** — Table row drag-drop verification

### Suggested Parallelization

```
Week 1: Track A (shared components)
Week 2: Track B steps 1-4 (schema, markdown, keyboard, styling)
        Track C step 1 (table integration)
Week 3: Track B steps 5-7 (tests, editor integration, drag-drop)
        Track C steps 2-3 (table drag-drop, context menu)
Week 4: Track B steps 8-10 (context menu, migration, E2E)
        Track C step 4 (table E2E)
```

---

## Decisions

1. **Multi-block selection:** ✅ Yes. When selection spans multiple blocks, only the **topmost handle** is visible. Dragging it moves all selected blocks. Context menu actions apply to all selected blocks. See Phase 3.8.

2. **Code blocks:** ✅ Code blocks are **single blocks**. A fenced code block (even with many lines) is one block that can be dragged as a unit. The flat model applies uniformly.

3. **Cross-context drag:** ✅ **Not supported** for MVP. Dragging between editor and table views is out of scope.

4. **Touch support:** ✅ **Follow-up work**, not MVP. Touch drag-drop requires different UX patterns (long-press to initiate, etc.).

5. **Block IDs:** ✅ **Not needed.** Blocks are a visual/UX layer, not persisted. Markdown is the storage format; ProseMirror positions suffice for runtime operations.

6. **Table row reordering:** ✅ **Deferred.** Rows sorted by creation time for now. Manual reordering API is a future enhancement.

7. **Nested blocks:** ✅ **Future enhancement.** Container blocks (toggles, callouts, columns) that hold child blocks are out of scope for MVP. The flat model with `indent` attribute handles hierarchical lists. Nested containers can be added later as separate node types if needed.

---

## Design Principles

The shared component architecture follows these principles:

1. **Separation of concerns:** UI components (`RowHandle`, `RowContextMenu`, `DropIndicator`) know nothing about ProseMirror or table records. They receive callbacks and render UI.

2. **Context-specific integration:** Each consumer (editor, table views) provides its own:
   - Drag data format (`application/x-prosemirror-block` vs `application/x-table-record`)
   - Context menu actions
   - Drop handling logic

3. **Consistent appearance:** All handles use identical styling via shared CSS. Visual consistency across editor and tables reinforces the mental model.

4. **Progressive enhancement:** Handles are optional UI chrome. The underlying data model (blocks, records) works without them. This allows:
   - Print/export without handles
   - Keyboard-only navigation
   - Future: different handle styles per context

---

## References

- [ProseMirror Guide](https://prosemirror.net/docs/guide/)
- [ProseMirror NodeView](https://prosemirror.net/docs/ref/#view.NodeView)
- [Notion's Block Model](https://www.notion.so/help/what-is-a-block)
- [Lexical (Meta's block editor)](https://lexical.dev/)
