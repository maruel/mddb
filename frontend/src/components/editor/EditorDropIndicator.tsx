// Editor drop indicator component that renders during block drag-and-drop.
// Listens to drag state and shows insertion point via the DropIndicator.

import { createSignal, onMount, onCleanup, createEffect } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import { DropIndicator } from '../shared/DropIndicator';
import { getDragState } from './blockDragPlugin';

export interface EditorDropIndicatorProps {
  view: EditorView | undefined;
}

export function EditorDropIndicator(props: EditorDropIndicatorProps) {
  const [dropY, setDropY] = createSignal<number | null>(null);
  const [isVisible, setIsVisible] = createSignal(false);

  onMount(() => {
    const view = props.view;
    if (!view) return;

    // Store the original dispatch to wrap it
    const originalDispatchTransaction = view.dispatch.bind(view);

    // Replace dispatch to update indicator state whenever the editor state changes
    view.dispatch = function (tr) {
      // Call the original dispatch
      originalDispatchTransaction(tr);

      // Update our indicator based on new state
      const dragState = getDragState(view.state);
      if (dragState.sourcePos !== null || (dragState.selectedPositions && dragState.selectedPositions.length > 0)) {
        // We're currently dragging
        if (dragState.dropIndicatorY !== null) {
          setDropY(dragState.dropIndicatorY);
          setIsVisible(true);
        } else {
          setIsVisible(false);
        }
      } else {
        // Not dragging
        setIsVisible(false);
      }
    };

    // Cleanup: restore original dispatch
    onCleanup(() => {
      view.dispatch = originalDispatchTransaction;
    });
  });

  // Watch the view prop for changes
  createEffect(() => {
    const view = props.view;
    if (!view) {
      setIsVisible(false);
    }
  });

  // Render inside an absolutely positioned container that overlays the editor
  return (
    <div
      style={{
        position: 'absolute',
        top: '0',
        left: '0',
        right: '0',
        bottom: '0',
        'pointer-events': 'none',
        'z-index': '50',
      }}
    >
      <DropIndicator y={dropY() ?? 0} visible={isVisible()} />
    </div>
  );
}
