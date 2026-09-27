// Verifies flat blocks render as typed rows in a real ProseMirror editor view.

import { it } from "node:test";
import { expect } from "@tests/expect";
import { EditorView } from "prosemirror-view";
import { createEditorState } from "./prosemirror-config";
import { parseMarkdown } from "./markdown-parser";
import { createBlockNodeView } from "./BlockNodeView";

it("renders each paragraph as a typed block row", () => {
  const host = document.createElement("div");
  const view = new EditorView(host, {
    state: createEditorState(parseMarkdown("Line one\n\nLine two\n\nLine three")),
    nodeViews: { block: createBlockNodeView },
  });
  try {
    const rows = Array.from(host.querySelectorAll(".block-row"));
    expect(rows).toHaveLength(3);
    expect(rows.map((row) => (row as HTMLElement).dataset.type)).toEqual(["paragraph", "paragraph", "paragraph"]);
    expect(rows.map((row) => row.querySelector("p")?.textContent)).toEqual(["Line one", "Line two", "Line three"]);
  } finally {
    view.destroy();
  }
});
