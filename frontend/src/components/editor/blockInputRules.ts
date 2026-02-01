// Input rules for flat block editor.
// Convert markdown-style syntax to block attributes.

import { inputRules, InputRule } from 'prosemirror-inputrules';
import { smartQuotes, ellipsis, emDash } from 'prosemirror-inputrules';
import type { EditorState } from 'prosemirror-state';

/**
 * Helper to create a block type conversion input rule.
 * Deletes the trigger text and updates the current block's attributes.
 */
function blockTypeRule(pattern: RegExp, getAttrs: (match: RegExpMatchArray) => Record<string, unknown>): InputRule {
  return new InputRule(pattern, (state: EditorState, match, start, end) => {
    const $start = state.doc.resolve(start);

    // Only apply at the start of a block
    if ($start.parentOffset !== 0) return null;

    const blockPos = $start.before(1);
    const attrs = getAttrs(match);

    return state.tr.delete(start, end).setNodeMarkup(blockPos, undefined, attrs);
  });
}

/**
 * Build input rules for flat block editor.
 */
export function buildBlockInputRules() {
  return inputRules({
    rules: [
      // Bullet list: - or * at start followed by space
      blockTypeRule(/^[-*]\s$/, () => ({
        type: 'bullet',
        indent: 0,
        level: null,
        checked: null,
        language: null,
      })),

      // Numbered list: 1. at start (any number)
      blockTypeRule(/^(\d+)\.\s$/, () => ({
        type: 'number',
        indent: 0,
        level: null,
        checked: null,
        language: null,
      })),

      // Task list (unchecked): - [ ] at start
      blockTypeRule(/^[-*]\s*\[\s\]\s$/, () => ({
        type: 'task',
        indent: 0,
        checked: false,
        level: null,
        language: null,
      })),

      // Task list (unchecked): [ ] at start (fallback if bullet rule consumed dash)
      blockTypeRule(/^\[\s\]\s$/, () => ({
        type: 'task',
        indent: 0,
        checked: false,
        level: null,
        language: null,
      })),

      // Task list (checked): - [x] or - [X] at start
      blockTypeRule(/^[-*]\s*\[[xX]\]\s$/, () => ({
        type: 'task',
        indent: 0,
        checked: true,
        level: null,
        language: null,
      })),

      // Task list (checked): [x] or [X] at start (fallback)
      blockTypeRule(/^\[[xX]\]\s$/, () => ({
        type: 'task',
        indent: 0,
        checked: true,
        level: null,
        language: null,
      })),

      // Heading 1: # at start
      blockTypeRule(/^#\s$/, () => ({
        type: 'heading',
        level: 1,
        indent: 0,
        checked: null,
        language: null,
      })),

      // Heading 2: ## at start
      blockTypeRule(/^##\s$/, () => ({
        type: 'heading',
        level: 2,
        indent: 0,
        checked: null,
        language: null,
      })),

      // Heading 3: ### at start
      blockTypeRule(/^###\s$/, () => ({
        type: 'heading',
        level: 3,
        indent: 0,
        checked: null,
        language: null,
      })),

      // Heading 4: #### at start
      blockTypeRule(/^####\s$/, () => ({
        type: 'heading',
        level: 4,
        indent: 0,
        checked: null,
        language: null,
      })),

      // Heading 5: ##### at start
      blockTypeRule(/^#####\s$/, () => ({
        type: 'heading',
        level: 5,
        indent: 0,
        checked: null,
        language: null,
      })),

      // Heading 6: ###### at start
      blockTypeRule(/^######\s$/, () => ({
        type: 'heading',
        level: 6,
        indent: 0,
        checked: null,
        language: null,
      })),

      // Blockquote: > at start
      blockTypeRule(/^>\s$/, () => ({
        type: 'quote',
        indent: 0,
        level: null,
        checked: null,
        language: null,
      })),

      // Code block: ``` with optional language at start followed by space
      new InputRule(/^```(\w*)\s$/, (state: EditorState, match, start, end) => {
        const $start = state.doc.resolve(start);

        // Only apply in an empty block at the start
        if ($start.parentOffset !== 0) return null;

        const blockPos = $start.before(1);

        // Let's just trust the regex for now.
        // If we require space, the user types space.

        return state.tr.delete(start, end).setNodeMarkup(blockPos, undefined, {
          type: 'code',
          language: match[1] || null,
          indent: 0,
          level: null,
          checked: null,
        });
      }),

      // Divider: --- at start of empty block (exactly three dashes)
      // Note: emDash rule might convert -- to — (em dash) before this rule triggers on the 3rd dash.
      // So we match either --- or —-
      new InputRule(/^(---|—-|—)$/, (state: EditorState, match, start, end) => {
        const $start = state.doc.resolve(start);
        const blockPos = $start.before(1);
        const block = state.doc.nodeAt(blockPos);

        // Only convert if the block only contains "---" (minus the last char which is being typed)
        // match[0] is "---" (len 3) or "—-" (len 2).
        // content in doc is "--" (len 2) or "—" (len 1).
        // So doc content length should be match[0].length - 1.
        if (block && block.textContent.length === match[0].length - 1) {
          return state.tr.delete(start, end).setNodeMarkup(blockPos, undefined, {
            type: 'divider',
            indent: 0,
            level: null,
            checked: null,
            language: null,
          });
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
