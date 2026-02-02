// ProseMirror NodeView implementation for flat blocks with integrated drag handles.
// Mounts SolidJS RowHandle component within vanilla JS NodeView.

import { render } from 'solid-js/web';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import { NodeSelection } from 'prosemirror-state';
import type { EditorView, NodeView, ViewMutationRecord } from 'prosemirror-view';
import { RowHandle } from '../shared/RowHandle';
import { setDragState, getSelectedBlockPositions, BLOCK_DRAG_MIME, BLOCKS_DRAG_MIME } from './blockDragPlugin';

/**
 * Custom event detail for block context menu requests.
 */
export interface BlockContextMenuDetail {
  /** Block position in document */
  pos: number;
  /** Mouse X coordinate */
  x: number;
  /** Mouse Y coordinate */
  y: number;
  /** Number of selected blocks (for multi-selection) */
  selectedCount: number;
}

/**
 * Custom event dispatched when a block's context menu is requested.
 */
export const BLOCK_CONTEXT_MENU_EVENT = 'block-context-menu';

/**
 * NodeView implementation for flat block nodes.
 * Renders a drag handle alongside block content.
 */
export class BlockNodeView implements NodeView {
  /** Outer DOM element containing handle and content */
  dom: HTMLElement;
  /** Content DOM element where ProseMirror renders inline content */
  contentDOM: HTMLElement;
  /** Cleanup function for SolidJS handle component */
  private handleDispose: (() => void) | null = null;
  /** Container element for the handle */
  private handleContainer: HTMLElement;

  constructor(
    private node: ProseMirrorNode,
    private view: EditorView,
    private getPos: () => number | undefined
  ) {
    // Create outer container
    this.dom = document.createElement('div');
    this.dom.className = 'block-row row-with-handle';
    this.updateDOMAttributes();

    // Create and mount handle container
    // Set contenteditable="false" to prevent the browser from treating drag
    // gestures on the handle as content dragging within the editor
    this.handleContainer = document.createElement('div');
    this.handleContainer.className = 'block-handle-container';
    this.handleContainer.contentEditable = 'false';
    this.dom.appendChild(this.handleContainer);

    // Mount SolidJS RowHandle component
    this.mountHandle();

    // Create content container
    const { contentDOM, wrapperDOM } = this.createContentElement();
    this.contentDOM = contentDOM;
    this.dom.appendChild(wrapperDOM || contentDOM);
  }

  /**
   * Create the content DOM element based on block type.
   */
  private createContentElement(): { contentDOM: HTMLElement; wrapperDOM?: HTMLElement } {
    const { type, level, checked, language } = this.node.attrs;
    let element: HTMLElement;

    switch (type) {
      case 'heading': {
        const tag = `h${level || 1}`;
        element = document.createElement(tag);
        break;
      }
      case 'code': {
        const pre = document.createElement('pre');
        const code = document.createElement('code');
        pre.appendChild(code);
        if (language) {
          pre.dataset.language = language;
        }
        // Specific case: pre is wrapper, code is content
        pre.className = 'block-content';
        return { contentDOM: code, wrapperDOM: pre };
      }
      case 'quote': {
        element = document.createElement('blockquote');
        break;
      }
      case 'divider': {
        element = document.createElement('hr');
        break;
      }
      case 'task': {
        element = document.createElement('div');
        element.className = 'block-task';
        element.dataset.checked = String(checked || false);
        break;
      }
      case 'bullet':
      case 'number': {
        element = document.createElement('div');
        element.className = `block-${type}`;
        if (this.node.attrs.number !== undefined && this.node.attrs.number !== null) {
          element.dataset.number = String(this.node.attrs.number);
        }
        break;
      }
      default: {
        // paragraph
        element = document.createElement('p');
        break;
      }
    }

    element.className = `${element.className || ''} block-content`.trim();
    return { contentDOM: element };
  }

  /**
   * Update DOM attributes based on node attributes.
   */
  private updateDOMAttributes(): void {
    const { type, indent, level, checked, language } = this.node.attrs;

    this.dom.dataset.type = type;
    this.dom.dataset.indent = String(indent || 0);

    if (level !== null && level !== undefined) {
      this.dom.dataset.level = String(level);
    }
    if (checked !== null && checked !== undefined) {
      this.dom.dataset.checked = String(checked);
    }
    if (language) {
      this.dom.dataset.language = language;
    }
  }

  /**
   * Mount the SolidJS RowHandle component.
   */
  private mountHandle(): void {
    const pos = this.getPos();
    const rowId = pos !== undefined ? String(pos) : '0';

    this.handleDispose = render(
      () =>
        RowHandle({
          rowId,
          onDragStart: this.handleDragStart.bind(this),
          onContextMenu: this.handleContextMenu.bind(this),
          onClick: this.handleClick.bind(this),
        }),
      this.handleContainer
    );
  }

  /**
   * Handle drag start event from the handle.
   */
  private handleDragStart(e: DragEvent, _rowId: string): void {
    const pos = this.getPos();
    if (pos === undefined) return;

    // Check if this block is part of a multi-selection
    const selectedPositions = getSelectedBlockPositions(this.view.state);
    const isMultiSelection = selectedPositions.length > 1 && selectedPositions.includes(pos);

    if (isMultiSelection) {
      // Multi-block drag: serialize all selected positions
      e.dataTransfer?.setData(BLOCKS_DRAG_MIME, JSON.stringify(selectedPositions));
      if (e.dataTransfer) {
        e.dataTransfer.effectAllowed = 'move';
      }
      this.view.dispatch(
        setDragState(this.view.state.tr, {
          sourcePos: pos,
          selectedPositions,
        })
      );
    } else {
      // Single block drag
      e.dataTransfer?.setData(BLOCK_DRAG_MIME, String(pos));
      if (e.dataTransfer) {
        e.dataTransfer.effectAllowed = 'move';
      }
      this.view.dispatch(
        setDragState(this.view.state.tr, {
          sourcePos: pos,
          selectedPositions: null,
        })
      );
    }

    // Add dragging class for visual feedback
    this.dom.classList.add('dragging');
  }

  /**
   * Handle context menu request from the handle.
   */
  private handleContextMenu(e: MouseEvent, _rowId: string): void {
    const pos = this.getPos();
    if (pos === undefined) return;

    // Check multi-selection
    const selectedPositions = getSelectedBlockPositions(this.view.state);
    const selectedCount =
      selectedPositions.length > 1 && selectedPositions.includes(pos) ? selectedPositions.length : 1;

    // Dispatch custom event for parent to handle
    const detail: BlockContextMenuDetail = {
      pos,
      x: e.clientX,
      y: e.clientY,
      selectedCount,
    };

    this.view.dom.dispatchEvent(
      new CustomEvent(BLOCK_CONTEXT_MENU_EVENT, {
        detail,
        bubbles: true,
      })
    );
  }

  /**
   * Handle click on the handle (select the block).
   */
  private handleClick(e: MouseEvent, _rowId: string): void {
    const pos = this.getPos();
    if (pos === undefined) return;

    // Select the entire block when clicking the handle
    // This enables multi-selection when holding shift
    const { state, dispatch } = this.view;
    const node = state.doc.nodeAt(pos);
    if (!node) return;

    // Create a node selection spanning the block
    if (e.shiftKey) {
      // Extend selection - handled by ProseMirror's default shift-click
      return;
    }

    // Select this block
    const selection = NodeSelection.create(state.doc, pos);
    dispatch(state.tr.setSelection(selection));
  }

  /**
   * Called when the node is updated. Returns false if NodeView should be rebuilt.
   */
  update(node: ProseMirrorNode): boolean {
    // Only handle same-type nodes
    if (node.type !== this.node.type) {
      return false;
    }

    // Check if block type attribute changed (requires rebuild)
    if (node.attrs.type !== this.node.attrs.type) {
      return false;
    }

    this.node = node;
    this.updateDOMAttributes();

    // Update task checkbox state
    if (node.attrs.type === 'task') {
      const taskContent = this.dom.querySelector('.block-task');
      if (taskContent) {
        (taskContent as HTMLElement).dataset.checked = String(node.attrs.checked || false);
      }
    }

    // Update number attribute
    if (node.attrs.type === 'number') {
      // For number type, contentDOM is the block-number div
      if (node.attrs.number !== undefined && node.attrs.number !== null) {
        this.contentDOM.dataset.number = String(node.attrs.number);
      }
    }

    return true;
  }

  /**
   * Called when the selection changes. Update handle visibility for multi-selection.
   */
  selectNode(): void {
    this.dom.classList.add('selected');
  }

  deselectNode(): void {
    this.dom.classList.remove('selected');
  }

  /**
   * Cleanup when the NodeView is destroyed.
   */
  destroy(): void {
    // Remove dragging class
    this.dom.classList.remove('dragging');

    // Dispose SolidJS component
    if (this.handleDispose) {
      this.handleDispose();
      this.handleDispose = null;
    }
  }

  /**
   * Stop ProseMirror from handling some mutations.
   */
  ignoreMutation(mutation: ViewMutationRecord): boolean {
    // Ignore mutations to the handle container
    if (mutation.type !== 'selection' && this.handleContainer.contains(mutation.target as Node)) {
      return true;
    }
    return false;
  }
}

/**
 * Factory function to create BlockNodeView instances.
 * Use this with EditorView's nodeViews option.
 */
export function createBlockNodeView(
  node: ProseMirrorNode,
  view: EditorView,
  getPos: () => number | undefined
): BlockNodeView {
  return new BlockNodeView(node, view, getPos);
}
