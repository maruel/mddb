// ProseMirror plugin for detecting "/" slash commands and tracking menu state.

import { Plugin, PluginKey } from 'prosemirror-state';
import type { EditorView } from 'prosemirror-view';

export interface SlashMenuState {
  active: boolean;
  query: string;
  triggerPos: number;
  position: { top: number; left: number };
}

export const slashMenuKey = new PluginKey<SlashMenuState>('slashMenu');

const initialState: SlashMenuState = {
  active: false,
  query: '',
  triggerPos: 0,
  position: { top: 0, left: 0 },
};

/**
 * Creates a ProseMirror plugin that detects "/" at line start or after whitespace
 * and triggers the slash command menu.
 */
export function createSlashCommandPlugin(onStateChange: (state: SlashMenuState) => void): Plugin<SlashMenuState> {
  return new Plugin<SlashMenuState>({
    key: slashMenuKey,

    state: {
      init() {
        return { ...initialState };
      },

      apply(tr, pluginState, _oldState, newState) {
        // Check if there's a meta flag to close the menu
        const meta = tr.getMeta(slashMenuKey);
        if (meta?.close) {
          if (pluginState.active) {
            const newPluginState = { ...initialState };
            // Schedule state change callback
            setTimeout(() => onStateChange(newPluginState), 0);
            return newPluginState;
          }
          return pluginState;
        }

        // Only process on document changes or selection changes
        if (!tr.docChanged && !tr.selectionSet) {
          return pluginState;
        }

        const { selection } = newState;
        const { $from } = selection;

        // Only trigger in empty selections (cursor position)
        if (!selection.empty) {
          if (pluginState.active) {
            const newPluginState = { ...initialState };
            setTimeout(() => onStateChange(newPluginState), 0);
            return newPluginState;
          }
          return pluginState;
        }

        // Get text from start of text block to cursor
        // In flat block schema, blocks are top-level and content is directly inside
        const textBlockStart = $from.start($from.depth);

        // Ensure we are inside a textblock (not identifying the doc or non-textblock)
        if (!$from.parent.isTextblock) return pluginState;

        const textToCursor = newState.doc.textBetween(textBlockStart, $from.pos, '', '');

        // Match "/" at line start or after whitespace, followed by optional query
        const slashMatch = textToCursor.match(/(^|\s)\/([^\s]*)$/);

        if (slashMatch) {
          const query = slashMatch[2] || '';
          const slashOffset = textToCursor.length - slashMatch[0].length + (slashMatch[1] ? 1 : 0);
          const triggerPos = textBlockStart + slashOffset;

          const newPluginState: SlashMenuState = {
            active: true,
            query,
            triggerPos,
            position: pluginState.position, // Preserve position until view.update calculates it
          };

          // Only notify on query changes if already active (position update comes from view.update)
          if (pluginState.active && query !== pluginState.query) {
            setTimeout(() => onStateChange(newPluginState), 0);
          }
          // If becoming active, view.update will call onStateChange with correct position
          return newPluginState;
        }

        // No match - close menu if it was open
        if (pluginState.active) {
          const newPluginState = { ...initialState };
          setTimeout(() => onStateChange(newPluginState), 0);
          return newPluginState;
        }

        return pluginState;
      },
    },

    view(_editorView: EditorView) {
      return {
        update(view: EditorView) {
          const pluginState = slashMenuKey.getState(view.state);
          if (pluginState?.active) {
            // Calculate menu position based on trigger position
            try {
              const coords = view.coordsAtPos(pluginState.triggerPos);
              const newTop = coords.bottom + 4;
              const newLeft = coords.left;

              // Only update position if we get valid coordinates (not 0,0)
              // This can happen during rapid typing or when position is off-screen
              const hasValidPosition = newTop > 0 || newLeft > 0;
              const position = hasValidPosition ? { top: newTop, left: newLeft } : pluginState.position; // Keep existing position if new one is invalid

              const newState: SlashMenuState = {
                ...pluginState,
                position,
              };
              onStateChange(newState);
            } catch {
              // Position calculation may fail during rapid updates
              // Still notify state change to update query filtering
              onStateChange(pluginState);
            }
          }
        },
      };
    },

    props: {
      handleKeyDown(view, event) {
        const pluginState = slashMenuKey.getState(view.state);
        if (!pluginState?.active) return false;

        // Intercept these keys to prevent ProseMirror from handling them
        // The menu component's document listener handles the actual navigation
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp' || event.key === 'Enter' || event.key === 'Tab') {
          return true; // Tell ProseMirror we handled it
        }

        if (event.key === 'Escape') {
          closeSlashMenu(view);
          return true;
        }

        return false;
      },
    },
  });
}

/**
 * Close the slash menu programmatically.
 */
export function closeSlashMenu(view: EditorView): void {
  view.dispatch(view.state.tr.setMeta(slashMenuKey, { close: true }));
}
