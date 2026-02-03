// ProseMirror NodeView implementation for flat blocks with integrated drag handles.

import { render } from 'solid-js/web';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import type { EditorView, NodeView, ViewMutationRecord } from 'prosemirror-view';
import { RowHandle } from '../shared/RowHandle';
import { setDragState, getSelectedBlockPositions, BLOCK_DRAG_MIME, BLOCKS_DRAG_MIME } from './blockDragPlugin';

/**
 * Custom event detail for block context menu requests.
 */
export interface BlockContextMenuDetail {
  pos: number;
  x: number;
  y: number;
  selectedCount: number;
}

/**
 * Custom event dispatched when a block's context menu is requested.
 */
export const BLOCK_CONTEXT_MENU_EVENT = 'block-context-menu';

/**
 * NodeView implementation for flat block nodes.
 */
export class BlockNodeView implements NodeView {
  dom: HTMLElement;
  contentDOM: HTMLElement;
  private handleDispose: (() => void) | null = null;
  private handleContainer: HTMLElement;

  constructor(
    private node: ProseMirrorNode,
    private view: EditorView,
    private getPos: () => number | undefined
  ) {
    this.dom = document.createElement('div');
    this.dom.className = 'block-row row-with-handle';
    this.updateDOMAttributes();

    this.handleContainer = document.createElement('div');
    this.handleContainer.className = 'block-handle-container';
    this.handleContainer.contentEditable = 'false';
    this.dom.appendChild(this.handleContainer);

    this.mountHandle();

    const { contentDOM, wrapperDOM } = this.createContentElement();
    this.contentDOM = contentDOM;
    this.dom.appendChild(wrapperDOM || contentDOM);
  }

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
        element = document.createElement('p');
        break;
      }
    }

    element.className = `${element.className || ''} block-content`.trim();
    return { contentDOM: element };
  }

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

  private handleDragStart(e: DragEvent, _rowId: string): void {
    const pos = this.getPos();
    if (pos === undefined) return;

    const selectedPositions = getSelectedBlockPositions(this.view.state);
    const isMultiSelection = selectedPositions.length > 1 && selectedPositions.includes(pos);

    // Set dataTransfer synchronously (required for drag)
    if (e.dataTransfer) {
      if (isMultiSelection) {
        e.dataTransfer.setData(BLOCKS_DRAG_MIME, JSON.stringify(selectedPositions));
      } else {
        e.dataTransfer.setData(BLOCK_DRAG_MIME, String(pos));
      }
      e.dataTransfer.effectAllowed = 'move';

      // Set drag image
      const blockRect = this.dom.getBoundingClientRect();
      const offsetX = e.clientX - blockRect.left;
      const offsetY = e.clientY - blockRect.top;
      e.dataTransfer.setDragImage(this.dom, offsetX, offsetY);
    }

    this.dom.classList.add('dragging');

    // Defer state update to avoid interfering with native drag initialization
    setTimeout(() => {
      this.view.dispatch(
        setDragState(this.view.state.tr, {
          sourcePos: pos,
          selectedPositions: isMultiSelection ? selectedPositions : null,
        })
      );
    }, 0);
  }

  private handleContextMenu(e: MouseEvent, _rowId: string): void {
    const pos = this.getPos();
    if (pos === undefined) return;

    const selectedPositions = getSelectedBlockPositions(this.view.state);
    const selectedCount =
      selectedPositions.length > 1 && selectedPositions.includes(pos) ? selectedPositions.length : 1;

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

  private handleClick(e: MouseEvent, rowId: string): void {
    this.handleContextMenu(e, rowId);
  }

  update(node: ProseMirrorNode): boolean {
    if (node.type !== this.node.type) {
      return false;
    }

    if (node.attrs.type !== this.node.attrs.type) {
      return false;
    }

    this.node = node;
    this.updateDOMAttributes();

    if (node.attrs.type === 'task') {
      const taskContent = this.dom.querySelector('.block-task');
      if (taskContent) {
        (taskContent as HTMLElement).dataset.checked = String(node.attrs.checked || false);
      }
    }

    if (node.attrs.type === 'number') {
      if (node.attrs.number !== undefined && node.attrs.number !== null) {
        this.contentDOM.dataset.number = String(node.attrs.number);
      }
    }

    return true;
  }

  selectNode(): void {
    this.dom.classList.add('selected');
  }

  deselectNode(): void {
    this.dom.classList.remove('selected');
  }

  destroy(): void {
    this.dom.classList.remove('dragging');

    if (this.handleDispose) {
      this.handleDispose();
      this.handleDispose = null;
    }
  }

  ignoreMutation(mutation: ViewMutationRecord): boolean {
    if (mutation.type !== 'selection' && this.handleContainer.contains(mutation.target as Node)) {
      return true;
    }
    return false;
  }
}

export function createBlockNodeView(
  node: ProseMirrorNode,
  view: EditorView,
  getPos: () => number | undefined
): BlockNodeView {
  return new BlockNodeView(node, view, getPos);
}
