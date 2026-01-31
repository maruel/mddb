// Slash command registry defining available block types for the editor menu.

import type { EditorView } from 'prosemirror-view';
import { Selection } from 'prosemirror-state';
import { nodes } from './prosemirror-config';
import type { BlockAttrs } from './schema';
import { convertBlock } from './blockCommands';

import SubjectIcon from '@material-symbols/svg-400/outlined/subject.svg?solid';
import TitleIcon from '@material-symbols/svg-400/outlined/title.svg?solid';
import FormatListBulletedIcon from '@material-symbols/svg-400/outlined/format_list_bulleted.svg?solid';
import FormatListNumberedIcon from '@material-symbols/svg-400/outlined/format_list_numbered.svg?solid';
import ChecklistIcon from '@material-symbols/svg-400/outlined/checklist.svg?solid';
import FormatQuoteIcon from '@material-symbols/svg-400/outlined/format_quote.svg?solid';
import CodeIcon from '@material-symbols/svg-400/outlined/code.svg?solid';
import HorizontalRuleIcon from '@material-symbols/svg-400/outlined/horizontal_rule.svg?solid';
import DescriptionIcon from '@material-symbols/svg-400/outlined/description.svg?solid';

export interface SlashCommand {
  id: string;
  labelKey: string; // i18n key under slashMenu.*
  keywords: string[];
  icon: SolidSVG;
  execute: (view: EditorView, from: number, to: number) => void;
  asyncAction?: 'createSubpage'; // Async actions handled by SlashCommandMenu
}

// Helper to delete the "/" trigger text and execute a command
function deleteSlashAndExecute(view: EditorView, from: number, to: number, action: (view: EditorView) => void) {
  // Delete the slash command text first
  const tr = view.state.tr.delete(from, to);
  view.dispatch(tr);

  // Then execute the action (conversion usually)
  action(view);
}

// Helper to convert current block (where cursor is) to another type
function convertCurrentBlock(
  view: EditorView,
  type: 'paragraph' | 'heading' | 'bullet' | 'number' | 'task' | 'quote' | 'code' | 'divider',
  attrs: Partial<BlockAttrs> = {}
) {
  const { state, dispatch } = view;
  const { $from } = state.selection;

  // Find the block ancestor
  for (let d = $from.depth; d > 0; d--) {
    const node = $from.node(d);
    if (node.isBlock) {
      convertBlock($from.before(d), type, attrs)(state, dispatch);
      return;
    }
  }
}

export const slashCommands: SlashCommand[] = [
  {
    id: 'paragraph',
    labelKey: 'paragraph',
    keywords: ['paragraph', 'text', 'plain'],
    icon: SubjectIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'paragraph');
      });
    },
  },
  {
    id: 'heading1',
    labelKey: 'heading1',
    keywords: ['heading', 'h1', 'title', 'header'],
    icon: TitleIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'heading', { level: 1 });
      });
    },
  },
  {
    id: 'heading2',
    labelKey: 'heading2',
    keywords: ['heading', 'h2', 'subtitle', 'header'],
    icon: TitleIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'heading', { level: 2 });
      });
    },
  },
  {
    id: 'heading3',
    labelKey: 'heading3',
    keywords: ['heading', 'h3', 'header'],
    icon: TitleIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'heading', { level: 3 });
      });
    },
  },
  {
    id: 'bulletList',
    labelKey: 'bulletList',
    keywords: ['bullet', 'list', 'unordered', 'ul'],
    icon: FormatListBulletedIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'bullet');
      });
    },
  },
  {
    id: 'orderedList',
    labelKey: 'orderedList',
    keywords: ['ordered', 'list', 'numbered', 'ol'],
    icon: FormatListNumberedIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'number');
      });
    },
  },
  {
    id: 'taskList',
    labelKey: 'taskList',
    keywords: ['task', 'checkbox', 'todo', 'checklist', 'check'],
    icon: ChecklistIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'task', { checked: false });
      });
    },
  },
  {
    id: 'blockquote',
    labelKey: 'blockquote',
    keywords: ['quote', 'blockquote', 'citation'],
    icon: FormatQuoteIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'quote');
      });
    },
  },
  {
    id: 'codeBlock',
    labelKey: 'codeBlock',
    keywords: ['code', 'codeblock', 'pre', 'programming'],
    icon: CodeIcon,
    execute: (view, from, to) => {
      deleteSlashAndExecute(view, from, to, (v) => {
        convertCurrentBlock(v, 'code');
      });
    },
  },
  {
    id: 'divider',
    labelKey: 'divider',
    keywords: ['divider', 'hr', 'horizontal', 'rule', 'line'],
    icon: HorizontalRuleIcon,
    execute: (view) => {
      convertCurrentBlock(view, 'divider');

      // Insert paragraph after
      const { state } = view;
      const { $from } = state.selection;
      const blockPos = $from.before(1);
      const blockNode = state.doc.nodeAt(blockPos);

      if (blockNode) {
        const newPos = blockPos + blockNode.nodeSize;
        const tr2 = state.tr.insert(newPos, nodes.block.create({ type: 'paragraph', indent: blockNode.attrs.indent }));
        tr2.setSelection(Selection.near(tr2.doc.resolve(newPos + 1)));
        view.dispatch(tr2);
      }
    },
  },
  {
    id: 'subpage',
    labelKey: 'subpage',
    keywords: ['subpage', 'page', 'child', 'nested', 'link'],
    icon: DescriptionIcon,
    asyncAction: 'createSubpage',
    execute: () => {
      // Handled by SlashCommandMenu via asyncAction
    },
  },
];

/**
 * Check if query matches text with fuzzy character sequence matching.
 * Returns true if all characters of query appear in order in text.
 * E.g., "bl" matches "bullet", "blt" matches "bullet list".
 */
function fuzzyMatch(query: string, text: string): boolean {
  let qi = 0;
  for (let ti = 0; ti < text.length && qi < query.length; ti++) {
    if (text[ti] === query[qi]) qi++;
  }
  return qi === query.length;
}

/**
 * Score how well a query matches text.
 * Higher score = better match. Returns 0 if no match.
 * - 100: Exact match
 * - 90: Prefix match
 * - 70: Contains match
 * - 50: Fuzzy match
 */
function matchScore(query: string, text: string): number {
  const lowerText = text.toLowerCase();
  if (lowerText === query) return 100;
  if (lowerText.startsWith(query)) return 90;
  if (lowerText.includes(query)) return 70;
  if (fuzzyMatch(query, lowerText)) return 50;
  return 0;
}

type TranslateFn = (key: string) => string | undefined;

/**
 * Filter commands by query string with fuzzy matching.
 * Matches against labelKey, keywords, and display text (via translate function).
 * Results are sorted by match quality.
 */
export function filterCommands(query: string, translate?: TranslateFn): SlashCommand[] {
  if (!query) return slashCommands;

  const lowerQuery = query.toLowerCase();

  // Score each command
  const scored = slashCommands
    .map((cmd) => {
      let bestScore = 0;

      // Match on label key
      bestScore = Math.max(bestScore, matchScore(lowerQuery, cmd.labelKey));

      // Match on keywords
      for (const kw of cmd.keywords) {
        bestScore = Math.max(bestScore, matchScore(lowerQuery, kw));
      }

      // Match on display text if translate function provided
      if (translate) {
        const displayText = translate(`slashMenu.${cmd.labelKey}`);
        if (displayText) {
          bestScore = Math.max(bestScore, matchScore(lowerQuery, displayText));
        }
      }

      return { cmd, score: bestScore };
    })
    .filter((item) => item.score > 0);

  // Sort by score descending
  scored.sort((a, b) => b.score - a.score);

  return scored.map((item) => item.cmd);
}
