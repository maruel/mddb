// Exercises toolbar formatting and list transitions with a real ProseMirror state in jsdom.

import { afterEach, describe, it } from "node:test";
import { expect } from "@tests/expect";
import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { EditorView } from "prosemirror-view";
import { TextSelection } from "prosemirror-state";
import { createEditorState } from "./prosemirror-config";
import { parseMarkdown } from "./markdown-parser";
import { serializeToMarkdown } from "./markdown-serializer";
import EditorToolbar, { type FormatState } from "./EditorToolbar";

const inactiveFormat: FormatState = {
  isBold: false,
  isItalic: false,
  isUnderline: false,
  isStrikethrough: false,
  isCode: false,
  headingLevel: null,
  isBulletList: false,
  isOrderedList: false,
  isTaskList: false,
  isBlockquote: false,
  isCodeBlock: false,
};

let view: EditorView | null = null;

afterEach(() => {
  cleanup();
  view?.destroy();
  view = null;
});

function mountToolbar(markdown: string, formatState: Partial<FormatState>, selectAll: boolean): EditorView {
  const doc = parseMarkdown(markdown);
  let state = createEditorState(doc);
  if (selectAll) {
    state = state.apply(state.tr.setSelection(TextSelection.create(doc, 1, doc.content.size - 1)));
  }
  view = new EditorView(document.createElement("div"), { state });
  render(() => (
    <EditorToolbar
      view={view ?? undefined}
      formatState={{ ...inactiveFormat, ...formatState }}
      position={{ top: 0, bottom: 20, left: 100 }}
    />
  ));
  return view;
}

describe("EditorToolbar", () => {
  const transitions = [
    { from: "paragraph", markdown: "First\n\nSecond\n\nThird", button: "Task List", to: "task" },
    { from: "paragraph", markdown: "First\n\nSecond\n\nThird", button: "Bullet List", to: "bullet" },
    { from: "paragraph", markdown: "First\n\nSecond\n\nThird", button: "Numbered List", to: "number" },
    { from: "bullet", markdown: "- First\n- Second\n- Third", button: "Numbered List", to: "number" },
    { from: "number", markdown: "1. First\n2. Second\n3. Third", button: "Bullet List", to: "bullet" },
    { from: "number", markdown: "1. First\n2. Second\n3. Third", button: "Task List", to: "task" },
    { from: "task", markdown: "- [ ] First\n- [ ] Second\n- [ ] Third", button: "Numbered List", to: "number" },
  ] as const;

  for (const { from, markdown, button, to } of transitions) {
    it(`converts all selected ${from} blocks to ${to} blocks`, () => {
      const editor = mountToolbar(markdown, {}, true);
      fireEvent.click(screen.getByTitle(button));
      expect(Array.from(editor.state.doc.content.content, (block) => block.attrs.type)).toEqual([to, to, to]);
      expect(Array.from(editor.state.doc.content.content, (block) => block.textContent)).toEqual([
        "First",
        "Second",
        "Third",
      ]);
      expect(serializeToMarkdown(editor.state.doc)).not.toBe("");
    });
  }

  const toggles = [
    { markdown: "- First\n- Second", button: "Bullet List", active: { isBulletList: true } },
    { markdown: "1. First\n2. Second", button: "Numbered List", active: { isOrderedList: true } },
    { markdown: "- [ ] First\n- [x] Second", button: "Task List", active: { isTaskList: true } },
  ] as const;

  for (const { markdown, button, active } of toggles) {
    it(`toggles ${button} off for the selected blocks`, () => {
      const editor = mountToolbar(markdown, active, true);
      expect(screen.getByTitle(button).className).toMatch(/isActive/);
      for (const other of ["Bullet List", "Numbered List", "Task List"]) {
        if (other !== button) expect(screen.getByTitle(other).className).not.toMatch(/isActive/);
      }
      fireEvent.click(screen.getByTitle(button));
      expect(Array.from(editor.state.doc.content.content, (block) => block.attrs.type)).toEqual([
        "paragraph",
        "paragraph",
      ]);
      expect(serializeToMarkdown(editor.state.doc)).toBe("First\n\nSecond");
    });
  }

  it("formats the current block when there is no multi-block selection", () => {
    const editor = mountToolbar("One", {}, false);
    fireEvent.click(screen.getByTitle("Heading 2"));
    expect(editor.state.doc.firstChild?.attrs).toMatchObject({ type: "heading", level: 2 });
    expect(serializeToMarkdown(editor.state.doc)).toBe("## One");
  });

  it("keeps the same blocks selected through consecutive list conversions", () => {
    const doc = parseMarkdown("First\n\nSecond\n\nThird");
    const state = createEditorState(doc);
    view = new EditorView(document.createElement("div"), {
      state: state.apply(state.tr.setSelection(TextSelection.create(doc, 1, doc.content.size - 1))),
    });
    const [formatState, setFormatState] = createSignal(inactiveFormat);
    render(() => (
      <EditorToolbar
        view={view ?? undefined}
        formatState={formatState()}
        position={{ top: 0, bottom: 20, left: 100 }}
      />
    ));

    for (const [button, type, active] of [
      ["Bullet List", "bullet", { isBulletList: true }],
      ["Numbered List", "number", { isOrderedList: true }],
      ["Task List", "task", { isTaskList: true }],
    ] as const) {
      fireEvent.click(screen.getByTitle(button));
      expect(Array.from(view.state.doc.content.content, (block) => block.attrs.type)).toEqual([type, type, type]);
      expect(view.state.selection.from).toBe(1);
      expect(view.state.selection.to).toBe(view.state.doc.content.size - 1);
      setFormatState({ ...inactiveFormat, ...active });
      expect(screen.getByTitle(button).className).toMatch(/isActive/);
    }
  });

  it("converts only the selected middle blocks", () => {
    const doc = parseMarkdown("First\n\nSecond\n\nThird\n\nFourth");
    const from = doc.child(0).nodeSize + 1;
    const to = doc.child(0).nodeSize + doc.child(1).nodeSize + doc.child(2).nodeSize - 1;
    const state = createEditorState(doc);
    view = new EditorView(document.createElement("div"), {
      state: state.apply(state.tr.setSelection(TextSelection.create(doc, from, to))),
    });
    render(() => (
      <EditorToolbar
        view={view ?? undefined}
        formatState={inactiveFormat}
        position={{ top: 0, bottom: 20, left: 100 }}
      />
    ));

    fireEvent.click(screen.getByTitle("Bullet List"));
    expect(Array.from(view.state.doc.content.content, (block) => block.attrs.type)).toEqual([
      "paragraph",
      "bullet",
      "bullet",
      "paragraph",
    ]);
    expect(serializeToMarkdown(view.state.doc)).toBe("First\n\n- Second\n- Third\n\nFourth");
  });
});
