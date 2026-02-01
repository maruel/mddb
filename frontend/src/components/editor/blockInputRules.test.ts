import { describe, it, expect } from 'vitest';
import { EditorState, type Transaction } from 'prosemirror-state';
import { EditorView } from 'prosemirror-view';
import { schema } from './schema';
import { buildBlockInputRules } from './blockInputRules';

describe('Block Input Rules', () => {
  let view: EditorView;

  function createEditor(docContent = [schema.nodes.block!.create({ type: 'paragraph' })]) {
    const state = EditorState.create({
      doc: schema.nodes.doc!.create(null, docContent),
      plugins: [buildBlockInputRules()],
    });
    view = new EditorView(document.createElement('div'), { state });
  }

  function type(text: string) {
    // Simulate handleTextInput which inputRules plugin uses
    const handled = view.someProp(
      'handleTextInput',
      (f: (view: EditorView, from: number, to: number, text: string, deflt: () => Transaction) => boolean | void) =>
        f(view, view.state.selection.from, view.state.selection.to, text, () => ({}) as unknown as Transaction)
    );

    if (!handled) {
      view.dispatch(view.state.tr.insertText(text));
    }
  }

  it('converts "- " to bullet list', () => {
    createEditor();
    type('-');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('bullet');
    expect(block.textContent).toBe('');
  });

  it('converts "1. " to numbered list', () => {
    createEditor();
    type('1');
    type('.');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('number');
    expect(block.textContent).toBe('');
  });

  it('converts "- [ ] " to task list (unchecked)', () => {
    createEditor();
    type('-');
    type(' ');
    type('[');
    type(' ');
    type(']');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('task');
    expect(block.attrs.checked).toBe(false);
  });

  it('converts "- [x] " to task list (checked)', () => {
    createEditor();
    type('-');
    type(' ');
    type('[');
    type('x');
    type(']');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('task');
    expect(block.attrs.checked).toBe(true);
  });

  it('converts "# " to heading 1', () => {
    createEditor();
    type('#');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('heading');
    expect(block.attrs.level).toBe(1);
  });

  it('converts "### " to heading 3', () => {
    createEditor();
    type('#');
    type('#');
    type('#');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('heading');
    expect(block.attrs.level).toBe(3);
  });

  it('converts "> " to quote', () => {
    createEditor();
    type('>');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('quote');
  });

  it('converts "---" to divider', () => {
    createEditor();
    type('-');
    type('-');
    type('-');

    // The rule is /^---$/, so it triggers on the 3rd dash if content matches
    // Wait, the rule is new InputRule(/^---$/, ...)
    // But input rules trigger on text input.
    // Usually input rules trigger on space or specific char.
    // The rule logic says: if block content is "---", convert.
    // But input rules are matched against text before cursor.
    // "---" matches "---".
    // Does it trigger immediately?

    // If I type "-", content is "-", rule doesn't match.
    // Type "-", content "--".
    // Type "-", content "---".
    // Regex matches "---".
    // Prosemirror input rules usually match `match[0]` ending with the typed char.
    // If the rule is `^---$`, it matches "---".

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('divider');
  });

  it('converts "``` " to code block', () => {
    createEditor();
    type('`');
    type('`');
    type('`');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('code');
  });

  it('converts "```js " to code block with language', () => {
    createEditor();
    type('`');
    type('`');
    type('`');
    type('j');
    type('s');
    type(' ');

    const block = view.state.doc.firstChild!;
    expect(block.attrs.type).toBe('code');
    expect(block.attrs.language).toBe('js');
  });
});
