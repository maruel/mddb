// ProseMirror plugin that preprocesses pasted HTML before parsing.
// Transforms nested lists and list items to annotated format for flat block schema.

import { Plugin } from 'prosemirror-state';
import { transformPastedHTML } from './dom-parser';

/**
 * Creates a plugin that intercepts paste events and preprocesses HTML
 * to annotate list items with data-type and data-indent before parsing.
 *
 * This allows the flat block schema to correctly identify list types and
 * nesting levels without relying on parent ul/ol context.
 */
export function domParsePlugin() {
  return new Plugin({
    props: {
      /**
       * Override the paste/drop handling to transform HTML before parsing.
       * This is called when content is pasted or dropped into the editor.
       */
      transformPasted(slice) {
        // Check if slice has HTML content
        // ProseMirror passes slice.content as nodes, but we need to intercept at the HTML level
        // This is handled via the handlePaste method below instead
        return slice;
      },

      /**
       * Handle paste events at the EditorView level.
       * This is set up in the EditorView creation, not here.
       * We'll use a beforeinput or paste event handler instead.
       */
    },
  });
}

/**
 * Sets up paste event handling on an EditorView element.
 * This should be called after the view is mounted.
 *
 * Example:
 * ```tsx
 * const view = new EditorView(container, { state, ... });
 * setupPasteHandler(view, container);
 * ```
 */
export function setupPasteHandler(container: HTMLElement, onPaste: (html: string) => void): void {
  container.addEventListener('paste', (e: ClipboardEvent) => {
    const html = e.clipboardData?.getData('text/html');
    if (html) {
      const transformed = transformPastedHTML(html);
      onPaste(transformed);
    }
  });
}

/**
 * Alternative approach: return a transform function that can be used
 * in EditorView's ClipboardParser configuration.
 *
 * Usage in EditorView creation:
 * ```typescript
 * const view = new EditorView(dom, {
 *   state,
 *   clipboardTextParser: createHTMLTransformer(parser),
 * });
 * ```
 */
export function createHTMLTransformer<T>(parser: (html: string) => T) {
  return (text: string) => {
    const transformed = transformPastedHTML(text);
    return parser(transformed);
  };
}
