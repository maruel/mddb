import { Schema } from 'prosemirror-model';
import { schema as baseSchema } from 'prosemirror-markdown';
import { addListNodes } from 'prosemirror-schema-list';

// Extend base schema nodes with list nodes and custom list_item attributes
let nodes = addListNodes(baseSchema.spec.nodes, 'paragraph block*', 'block');

// Update list_item to support 'checked' attribute for task lists
const listItemNode = nodes.get('list_item');
if (listItemNode) {
  nodes = nodes.update('list_item', {
    ...listItemNode,
    attrs: {
      ...listItemNode.attrs,
      checked: { default: null }, // null = bullet/number, boolean = task
    },
  });
}

// Add underline and strikethrough marks if missing
let marks = baseSchema.spec.marks;

if (!marks.get('underline')) {
  marks = marks.addToEnd('underline', {
    parseDOM: [{ tag: 'u' }, { style: 'text-decoration=underline' }],
    toDOM() {
      return ['u', 0];
    },
  });
}

if (!marks.get('strikethrough')) {
  marks = marks.addToEnd('strikethrough', {
    parseDOM: [{ tag: 's' }, { tag: 'del' }, { style: 'text-decoration=line-through' }],
    toDOM() {
      return ['s', 0];
    },
  });
}

// Rename 'strike' to 'strikethrough' if it exists differently or ensure naming consistency
// prosemirror-markdown usually has 'em', 'strong', 'link', 'code'.
// We ensure 'underline' and 'strikethrough' are present.

export const nestedSchema = new Schema({
  nodes,
  marks,
});
