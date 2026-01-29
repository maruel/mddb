// ProseMirror plugin for drag-and-drop file upload support.

import { Plugin, PluginKey } from 'prosemirror-state';
import type { EditorView } from 'prosemirror-view';

export interface DropUploadState {
  isDragging: boolean;
}

export const dropUploadKey = new PluginKey<DropUploadState>('dropUpload');

export interface DropUploadOptions {
  /** Callback when files are dropped. Returns position in document. */
  onFileDrop: (files: File[], pos: number) => void;
}

/**
 * Creates a ProseMirror plugin that handles drag-and-drop file uploads.
 * Sets isDragging state during drag for CSS styling.
 */
export function createDropUploadPlugin(options: DropUploadOptions): Plugin<DropUploadState> {
  return new Plugin<DropUploadState>({
    key: dropUploadKey,

    state: {
      init() {
        return { isDragging: false };
      },
      apply(tr, pluginState) {
        const meta = tr.getMeta(dropUploadKey);
        if (meta !== undefined) {
          return { isDragging: meta.isDragging };
        }
        return pluginState;
      },
    },

    props: {
      attributes(state): { [name: string]: string } {
        const pluginState = dropUploadKey.getState(state);
        if (pluginState?.isDragging) {
          return { class: 'drop-target-active' };
        }
        return {};
      },

      handleDOMEvents: {
        dragenter(view: EditorView, event: DragEvent) {
          // Only handle if dragging files
          if (!event.dataTransfer?.types.includes('Files')) {
            return false;
          }
          event.preventDefault();
          setDragging(view, true);
          return false;
        },

        dragover(_view: EditorView, event: DragEvent) {
          // Only handle if dragging files
          if (!event.dataTransfer?.types.includes('Files')) {
            return false;
          }
          event.preventDefault();
          // Set drop effect
          if (event.dataTransfer) {
            event.dataTransfer.dropEffect = 'copy';
          }
          return false;
        },

        dragleave(view: EditorView, event: DragEvent) {
          // Check if we're leaving the editor entirely
          const target = event.relatedTarget as Node | null;
          const editorDom = view.dom;
          if (!target || !editorDom.contains(target)) {
            setDragging(view, false);
          }
          return false;
        },

        drop(view: EditorView, event: DragEvent) {
          setDragging(view, false);

          const files = event.dataTransfer?.files;
          if (!files || files.length === 0) {
            return false;
          }

          // Check if any files match our supported types
          const validFiles: File[] = [];
          for (const file of files) {
            if (isValidFileType(file.type)) {
              validFiles.push(file);
            }
          }

          if (validFiles.length === 0) {
            return false;
          }

          event.preventDefault();

          // Get drop position in document
          const pos = view.posAtCoords({ left: event.clientX, top: event.clientY });
          if (!pos) {
            // Fallback to end of document
            options.onFileDrop(validFiles, view.state.doc.content.size);
            return true;
          }

          options.onFileDrop(validFiles, pos.pos);
          return true;
        },
      },
    },
  });
}

function setDragging(view: EditorView, isDragging: boolean): void {
  const currentState = dropUploadKey.getState(view.state);
  if (currentState?.isDragging !== isDragging) {
    view.dispatch(view.state.tr.setMeta(dropUploadKey, { isDragging }));
  }
}

function isValidFileType(mimeType: string): boolean {
  return (
    mimeType === 'image/png' ||
    mimeType === 'image/jpeg' ||
    mimeType === 'image/gif' ||
    mimeType === 'image/webp' ||
    mimeType === 'image/svg+xml' ||
    mimeType === 'image/avif' ||
    mimeType === 'application/pdf'
  );
}
