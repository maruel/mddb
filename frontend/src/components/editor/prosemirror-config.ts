// Configures flat block editing with plugins, keymaps, and code whitespace preservation.
// Integrates schema, plugins, and keymaps for the new block-based architecture.

import { EditorState, Plugin } from "prosemirror-state";
import { history, undo, redo } from "prosemirror-history";
import { keymap } from "prosemirror-keymap";
import { baseKeymap } from "prosemirror-commands";
import { dropCursor } from "prosemirror-dropcursor";
import { gapCursor } from "prosemirror-gapcursor";
import { DOMParser, type ParseOptions, type TagParseRule, type Node as ProseMirrorNode } from "prosemirror-model";

// Import flat block schema
import { schema } from "./schema";

// Import new block-specific plugins and rules
import { buildBlockInputRules } from "./blockInputRules";
import { buildBlockKeymap } from "./blockKeymap";
import { blockDragPlugin } from "./blockDragPlugin";
import { blockSelectionPlugin } from "./blockSelectionPlugin";
import { numberCounterPlugin } from "./numberCounterPlugin";

// Re-export schema components for use in other files
export { schema };
export { nodes, marks } from "./schema";

// ProseMirror's DOM observer supplies ruleFromNode when reparsing NodeViews.
// The runtime hook is omitted from prosemirror-model's public declarations.
interface BlockParseOptions extends ParseOptions {
  ruleFromNode?: (node: Node) => Omit<TagParseRule, "tag"> | null;
}

// Flat blocks share a node type, so ProseMirror cannot infer code whitespace
// from the schema's node type. Preserve it when reparsing a browser code edit.
class BlockDOMParser extends DOMParser {
  override parse(dom: Node, options?: BlockParseOptions): ProseMirrorNode {
    const ruleFromNode = options?.ruleFromNode;
    return super.parse(dom, {
      ...options,
      ...(options?.topNode?.attrs.type === "code" ? { preserveWhitespace: "full" as const } : {}),
      ...(ruleFromNode
        ? {
            ruleFromNode: (node: Node) => {
              const rule = ruleFromNode(node);
              return rule?.attrs?.type === "code" ? { ...rule, preserveWhitespace: "full" } : rule;
            },
          }
        : {}),
    });
  }
}

const codeWhitespacePlugin = new Plugin({
  props: { domParser: new BlockDOMParser(schema, DOMParser.fromSchema(schema).rules) },
});

/**
 * Create a new EditorState with all necessary plugins for the block editor.
 */
export function createEditorState(doc: ProseMirrorNode, extraPlugins?: Plugin[]) {
  return EditorState.create({
    doc,
    schema,
    plugins: [
      codeWhitespacePlugin,
      // Input rules for converting markdown syntax to blocks (e.g. "- " -> bullet block)
      buildBlockInputRules(),

      // Keymap for block operations (Enter, Backspace, Tab, etc.)
      buildBlockKeymap(),

      // Undo/Redo keybindings (must come before history plugin)
      keymap({ "Mod-z": undo, "Mod-y": redo, "Shift-Mod-z": redo }),

      // Standard ProseMirror keymap (basic editing)
      keymap(baseKeymap),

      // Undo/Redo history
      history(),

      // Drag and drop system
      blockDragPlugin,
      blockSelectionPlugin,
      numberCounterPlugin,

      // UI polish
      dropCursor({ color: "var(--c-control-accent)", width: 2 }),
      gapCursor(),

      // Extra plugins passed from Editor (e.g. slash menu, upload)
      ...(extraPlugins || []),
    ],
  });
}
