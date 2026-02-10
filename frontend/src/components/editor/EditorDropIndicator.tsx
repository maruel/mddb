// Editor drop indicator component that renders during block drag-and-drop.
// Shows a horizontal line at the insertion point between blocks.

import { createSignal, createEffect, onCleanup, Show } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import { getDragState } from './blockDragPlugin';
import styles from './EditorDropIndicator.module.css';

export interface EditorDropIndicatorProps {
  view: EditorView | undefined;
}

/**
 * Drop indicator that shows where a dragged block will be inserted.
 * Wraps the EditorView's dispatch to track drag state changes.
 */
export function EditorDropIndicator(props: EditorDropIndicatorProps) {
  const [indicatorY, setIndicatorY] = createSignal<number | null>(null);
  let containerRef: HTMLDivElement | undefined;

  // Track when we've wrapped dispatch to avoid wrapping multiple times
  let wrappedView: EditorView | null = null;
  let originalDispatch: EditorView['dispatch'] | null = null;

  /**
   * Update indicator position based on drag state.
   */
  const updateIndicator = (view: EditorView) => {
    const dragState = getDragState(view.state);

    if (dragState.dropIndicatorY !== null && containerRef) {
      // Convert viewport Y to container-relative Y
      const containerRect = containerRef.getBoundingClientRect();
      const relativeY = dragState.dropIndicatorY - containerRect.top;
      setIndicatorY(relativeY);
    } else {
      setIndicatorY(null);
    }
  };

  // Watch for view changes and wrap dispatch
  createEffect(() => {
    const view = props.view;

    // Unwrap previous view if different
    if (wrappedView && wrappedView !== view && originalDispatch) {
      wrappedView.dispatch = originalDispatch;
      wrappedView = null;
      originalDispatch = null;
    }

    if (!view || wrappedView === view) return;

    // Store original dispatch
    const boundDispatch = view.dispatch.bind(view);
    originalDispatch = boundDispatch;
    wrappedView = view;

    // Wrap dispatch to intercept state changes
    view.dispatch = function (tr) {
      boundDispatch(tr);
      updateIndicator(view);
    };
  });

  onCleanup(() => {
    // Restore original dispatch on cleanup
    if (wrappedView && originalDispatch) {
      wrappedView.dispatch = originalDispatch;
    }
  });

  return (
    <div ref={(el) => (containerRef = el)} class={styles.container}>
      <Show when={indicatorY() !== null}>
        <div class={styles.indicator} style={{ top: `${indicatorY()}px` }} role="presentation" aria-hidden="true" />
      </Show>
    </div>
  );
}
