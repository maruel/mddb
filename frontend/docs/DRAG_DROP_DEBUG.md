# Block Drag-and-Drop Debug Notes

## Problem
Drag operations from block handles are being lost - click + movement fails to initiate drag.

## Architecture

### Components
- `BlockNodeView.ts` - ProseMirror NodeView, creates drag handle via `RowHandle`
- `RowHandle.tsx` - SolidJS component with `draggable="true"`
- `blockDragPlugin.ts` - ProseMirror plugin handling dragover/drop/dragend events
- `EditorDropIndicator.tsx` - Visual drop indicator

### Event Flow (Expected)
1. User mousedown on RowHandle
2. User moves mouse (drag threshold)
3. Browser fires `dragstart` on RowHandle
4. `RowHandle.onDragStart` sets dataTransfer MIME types
5. `blockDragPlugin.dragstart` handler returns true to prevent ProseMirror's built-in handler
6. User drags, `dragover` events update drop indicator
7. User releases, `drop` event moves blocks

### Root Cause (Found)

The handle DOM element was getting **detached from the document** between the `dragstart` target phase and the bubble phase reaching ProseMirror's listener on `.ProseMirror`. This caused:

1. ProseMirror's `eventBelongsToView()` to return `false` (parent chain hit `null`)
2. The `blockDragPlugin.dragstart` handler never fired
3. The browser aborted the native drag (no `dragover`/`drop`/`dragend` events followed)

Two separate mechanisms caused the detachment:

#### Issue 1: ProseMirror's mousedown handler re-renders NodeViews

ProseMirror's `handlers.mousedown` calls `forceDOMFlush()` → `endComposition()` → `view.domObserver.forceFlush()` which can trigger `view.updateState(view.state)`, destroying and recreating NodeViews. This detaches the handle between mousedown and dragstart.

**Fix**: Added `stopEvent()` to `BlockNodeView` that returns `true` for `mousedown`/`mouseup`/`pointerdown`/`pointerup` events originating from the handle container. This prevents ProseMirror from processing mouse events on the drag handle. The handle's own SolidJS event handlers (onDragStart, onClick, onContextMenu) are unaffected since they fire on the element directly, not through ProseMirror's dispatch.

#### Issue 2: DOM mutation during dragstart triggers MutationObserver flush

`this.dom.classList.add('dragging')` in `handleDragStart` triggers a DOM attribute mutation. ProseMirror's `ignoreMutation` didn't cover attribute mutations on `this.dom` (only mutations in `handleContainer`). When the browser fires a `selectionchange` during drag initiation, ProseMirror's `DOMObserver.flush()` picks up the pending mutation and re-renders.

**Fix**: Removed direct DOM class manipulation from `handleDragStart`. Also added `ignoreMutation` coverage for attribute mutations on `this.dom` as a safety net.

#### Issue 3 (historical): ProseMirror clears dataTransfer
ProseMirror's built-in `handlers.dragstart` (prosemirror-view/src/input.ts) calls `event.dataTransfer.clearData()` which erases our custom MIME types.

**Fix**: Plugin returns `true` from `dragstart` to prevent default handler. This works correctly once Issue 1 and 2 are fixed (so the plugin handler actually fires).

### Key Insight: eventBelongsToView

ProseMirror's event dispatch checks `eventBelongsToView()` before calling any plugin handlers:

```javascript
function eventBelongsToView(view, event) {
    if (!event.bubbles) return true;
    if (event.defaultPrevented) return false;
    for (let node = event.target; node != view.dom; node = node.parentNode)
        if (!node || node.nodeType == 11 ||
            (node.pmViewDesc && node.pmViewDesc.stopEvent(event)))
            return false;
    return true;
}
```

If any node in the parent chain from `event.target` to `view.dom` is `null` (element detached from document), the event is silently ignored. No plugin handlers fire, no built-in handlers fire. This makes DOM stability during event propagation critical for custom NodeViews.

### Testing

- **Synthetic drag** (e2e tests using `dispatchEvent`): Works because the entire drag sequence is scripted — no real browser drag initiation, no mousedown processing, no MutationObserver flush between phases.
- **Real browser drag** (user interaction): Fails without the fixes because trusted events trigger browser-level selection changes and ProseMirror's mousedown handler, both of which can cause NodeView re-rendering.

This is why the e2e tests passed while real drag was broken.
