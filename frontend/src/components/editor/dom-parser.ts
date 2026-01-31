// DOM parsing utilities for converting pasted/loaded HTML to flat block format.
// Handles ul/ol detection, indentation tracking, and task list recognition.

/**
 * Preprocesses pasted HTML to annotate list items with their list type (bullet/number)
 * and nesting depth, enabling the schema's parseDOM to convert them to flat blocks.
 *
 * ProseMirror doesn't preserve the parent ul/ol context when parsing individual li elements,
 * so we walk the DOM tree beforehand and annotate each li with data-type and data-indent.
 *
 * Example transformation:
 * ```html
 * <!-- Before -->
 * <ul>
 *   <li>Item 1</li>
 *   <li>
 *     Item 2
 *     <ol>
 *       <li>Nested ordered</li>
 *     </ol>
 *   </li>
 * </ul>
 *
 * <!-- After -->
 * <ul>
 *   <li data-type="bullet" data-indent="0">Item 1</li>
 *   <li data-type="bullet" data-indent="0">
 *     Item 2
 *     <ol>
 *       <li data-type="number" data-indent="1">Nested ordered</li>
 *     </ol>
 *   </li>
 * </ul>
 * ```
 */
export function transformPastedHTML(html: string): string {
  const parser = new DOMParser();
  const doc = parser.parseFromString(html, 'text/html');
  const body = doc.body;

  // Walk the DOM and annotate list items
  annotateListItems(body);

  return body.innerHTML;
}

/**
 * Recursively walks a DOM tree and annotates all li elements with data-type and data-indent.
 * Handles nested lists of different types (ul, ol) and task list items.
 */
function annotateListItems(node: Node, indentLevel: number = 0): void {
  if (!(node instanceof Element)) return;

  // Determine the list type from this element
  const isOrderedList = node.tagName === 'OL';
  const isBulletList = node.tagName === 'UL';

  if (isBulletList || isOrderedList) {
    const listType = isOrderedList ? 'number' : 'bullet';

    // Annotate all direct li children
    Array.from(node.children).forEach((child) => {
      if (child.tagName === 'LI') {
        annotateListItem(child, listType, indentLevel);
      }
    });
  } else {
    // Recursively process children
    Array.from(node.children).forEach((child) => {
      annotateListItems(child, indentLevel);
    });
  }
}

/**
 * Annotates a single li element and its nested lists.
 * - Sets data-type to "bullet" or "number"
 * - Sets data-indent to the nesting level
 * - Detects task list items and sets data-checked attribute
 * - Recursively processes nested lists
 */
function annotateListItem(li: Element, listType: 'bullet' | 'number', indentLevel: number): void {
  li.setAttribute('data-type', listType);
  li.setAttribute('data-indent', String(indentLevel));

  // Check if this is a task list item
  // Task list items have a checkbox: [ ] or [x] at the start
  const firstChild = li.firstChild;
  if (firstChild && firstChild.nodeType === Node.TEXT_NODE) {
    const text = firstChild.textContent || '';
    const match = text.match(/^\s*\[([ xX])\]\s*/);
    if (match) {
      // This is a task list item
      li.classList.add('task-list-item');
      const checkedChar = match[1];
      if (checkedChar) {
        li.setAttribute('data-checked', checkedChar.toLowerCase() === 'x' ? 'true' : 'false');
      }
    }
  }

  // Recursively annotate nested lists
  Array.from(li.children).forEach((child) => {
    if (child.tagName === 'UL' || child.tagName === 'OL') {
      annotateListItems(child, indentLevel + 1);
    }
  });
}

/**
 * Detects and extracts code block language from a pre element's classes.
 * Searches for common language class patterns: language-xyz, lang-xyz, hljs-xyz
 *
 * Example:
 * ```html
 * <pre class="language-typescript"><code>...</code></pre>
 * ```
 * Returns: "typescript"
 */
export function extractCodeLanguage(preElement: HTMLElement): string | null {
  const classes = Array.from(preElement.classList);

  for (const cls of classes) {
    // Check for language-* pattern
    if (cls.startsWith('language-')) {
      return cls.slice(9); // Remove 'language-' prefix
    }
    // Check for lang-* pattern
    if (cls.startsWith('lang-')) {
      return cls.slice(5); // Remove 'lang-' prefix
    }
    // Check for hljs-* pattern
    if (cls.startsWith('hljs-')) {
      return cls.slice(5); // Remove 'hljs-' prefix
    }
  }

  // Check data-language attribute
  const dataLang = preElement.getAttribute('data-language');
  if (dataLang) return dataLang;

  return null;
}
