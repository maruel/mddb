// ProseMirror plugin for highlighting invalid (broken) internal page links.

import { Plugin, PluginKey } from 'prosemirror-state';
import { Decoration, DecorationSet } from 'prosemirror-view';
import type { EditorView } from 'prosemirror-view';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import type { NodeTitleMap } from './markdown-utils';

/**
 * Pattern to extract nodeId from internal page link URLs.
 * Matches: /w/{wsId}+{slug}/{nodeId}+{slug}
 * Captures: group 1 = wsId, group 2 = nodeId
 * Note: Pattern aligned with parseNodeUrl in urls.ts
 */
export const INTERNAL_LINK_URL_PATTERN = /^\/w\/([^/+]+)(?:\+[^/]*)?\/([a-zA-Z0-9_-]+)(?:\+.*)?$/;

export interface InvalidLinkPluginState {
  decorations: DecorationSet;
  linkedNodeTitles: NodeTitleMap;
  wsId: string | undefined;
}

export const invalidLinkPluginKey = new PluginKey<InvalidLinkPluginState>('invalidLink');

/**
 * Check if a link URL is an internal page link and if the target page exists.
 * Returns true if the link is invalid (broken).
 */
function isInvalidInternalLink(href: string, linkedNodeTitles: NodeTitleMap, wsId: string | undefined): boolean {
  if (!wsId || !href) return false;

  const match = href.match(INTERNAL_LINK_URL_PATTERN);
  if (!match) {
    // Debug: log if pattern doesn't match internal-looking links
    if (href.startsWith('/w/')) {
      console.warn('[invalidLink] Pattern did not match:', href);
    }
    return false;
  }

  const linkWsId = match[1];
  const nodeId = match[2];

  // Only check links within the current workspace
  if (linkWsId !== wsId) return false;

  // Link is invalid if the node ID is not in the titles map
  if (!nodeId) return false;

  return !(nodeId in linkedNodeTitles);
}

/**
 * Build decoration set by scanning the document for invalid links.
 */
function buildDecorations(
  doc: ProseMirrorNode,
  linkedNodeTitles: NodeTitleMap,
  wsId: string | undefined
): DecorationSet {
  const decorations: Decoration[] = [];

  doc.descendants((node, pos) => {
    // Check each mark on text nodes
    if (node.isText) {
      const linkMark = node.marks.find((m) => m.type.name === 'link');
      if (linkMark && linkMark.attrs.href) {
        const href = linkMark.attrs.href as string;
        if (isInvalidInternalLink(href, linkedNodeTitles, wsId)) {
          // Add inline decoration for the entire text node with the link
          decorations.push(Decoration.inline(pos, pos + node.nodeSize, { class: 'invalid-link' }));
        }
      }
    }
    return true; // Continue traversing
  });

  return DecorationSet.create(doc, decorations);
}

/**
 * Creates a ProseMirror plugin that highlights invalid internal page links.
 * Invalid links are those pointing to pages that don't exist (not in linkedNodeTitles).
 */
export function createInvalidLinkPlugin(): Plugin<InvalidLinkPluginState> {
  return new Plugin<InvalidLinkPluginState>({
    key: invalidLinkPluginKey,

    state: {
      init() {
        return {
          decorations: DecorationSet.empty,
          linkedNodeTitles: {},
          wsId: undefined,
        };
      },

      apply(tr, pluginState, _oldState, newState) {
        // Check for meta to update linkedNodeTitles or wsId
        const meta = tr.getMeta(invalidLinkPluginKey);
        if (meta) {
          const newLinkedNodeTitles = meta.linkedNodeTitles ?? pluginState.linkedNodeTitles;
          const newWsId = meta.wsId ?? pluginState.wsId;

          // Rebuild decorations with new titles
          return {
            decorations: buildDecorations(newState.doc, newLinkedNodeTitles, newWsId),
            linkedNodeTitles: newLinkedNodeTitles,
            wsId: newWsId,
          };
        }

        // If document changed, rebuild decorations
        if (tr.docChanged) {
          return {
            ...pluginState,
            decorations: buildDecorations(newState.doc, pluginState.linkedNodeTitles, pluginState.wsId),
          };
        }

        // Map decorations through the transaction
        return {
          ...pluginState,
          decorations: pluginState.decorations.map(tr.mapping, tr.doc),
        };
      },
    },

    props: {
      decorations(state) {
        return invalidLinkPluginKey.getState(state)?.decorations ?? DecorationSet.empty;
      },
    },
  });
}

/**
 * Update the plugin's linkedNodeTitles and wsId.
 * Call this when the titles map or workspace changes.
 */
export function updateInvalidLinkState(
  view: EditorView,
  linkedNodeTitles: NodeTitleMap,
  wsId: string | undefined
): void {
  const tr = view.state.tr.setMeta(invalidLinkPluginKey, { linkedNodeTitles, wsId });
  view.dispatch(tr);
}
