// Tests block rendering, live Mermaid edits, and native DOM reconciliation with code whitespace preservation.

import { it } from "node:test";
import { expect } from "@tests/expect";
import { waitFor } from "@solidjs/testing-library";
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

it("updates live Mermaid diagrams while preserving editable source", { timeout: 5000 }, async (ctx) => {
  const host = document.createElement("div");
  const view = new EditorView(host, {
    state: createEditorState(parseMarkdown("```mermaid title\nflowchart LR\nOriginal --> Diagram\n```")),
    nodeViews: { block: createBlockNodeView },
  });
  try {
    await waitFor(() => expect(host.querySelector("svg")?.textContent).toContain("Original"));
    expect(host.querySelector("pre code")?.textContent).toContain("Original --> Diagram");
    expect(host.querySelector(".mermaid-diagram")?.getAttribute("contenteditable")).toBe("false");
    ctx.mock.timers.enable({ apis: ["setTimeout"] });
    const block = view.state.doc.firstChild!;
    view.dispatch(view.state.tr.insertText("flowchart LR\nUpdated --> Diagram", 1, block.nodeSize - 1));
    ctx.mock.timers.tick(149);
    expect(host.querySelector("svg")?.textContent).toContain("Original");
    ctx.mock.timers.tick(1);
    await waitFor(() => expect(host.querySelector("svg")?.textContent).toContain("Updated"));
    expect(host.querySelector("svg")?.textContent).not.toContain("Original");
    view.dispatch(view.state.tr.insertText("invalid syntax", 1, view.state.doc.firstChild!.nodeSize - 1));
    ctx.mock.timers.tick(150);
    await waitFor(() => expect(host.querySelector("svg")).toBeNull());
    expect(host.querySelector("pre code")?.textContent).toBe("invalid syntax");
  } finally {
    view.destroy();
  }
});

it("preserves multiline code when the browser edits its DOM", async () => {
  const host = document.createElement("div");
  const view = new EditorView(host, {
    state: createEditorState(parseMarkdown("```mermaid\nflowchart LR\nOriginal --> Diagram\n```")),
    nodeViews: { block: createBlockNodeView },
  });
  try {
    await waitFor(() => expect(host.querySelector("svg")).not.toBeNull());
    const code = host.querySelector("code")!;
    code.firstChild!.textContent = "flowchart LR\nUpdated --> Diagram";
    await waitFor(() => expect(view.state.doc.firstChild!.textContent).toContain("Updated"));
    expect(view.state.doc.firstChild!.textContent).toBe("flowchart LR\nUpdated --> Diagram");
    await waitFor(() => expect(host.querySelector("svg")?.textContent).toContain("Updated"));
  } finally {
    view.destroy();
  }
});

it("reconciles native child removal in paragraph rows", async () => {
  const host = document.createElement("div");
  const view = new EditorView(host, {
    state: createEditorState(parseMarkdown("Hello\n\nWorld")),
    nodeViews: { block: createBlockNodeView },
  });
  try {
    const paragraphs = host.querySelectorAll("p");
    paragraphs[0]!.append(...paragraphs[1]!.childNodes);
    paragraphs[1]!.remove();
    await waitFor(() => expect(view.state.doc.firstChild!.textContent).toBe("HelloWorld"));
    expect(view.state.doc.child(1).textContent).toBe("");
    expect(host.querySelectorAll("p")).toHaveLength(2);
  } finally {
    view.destroy();
  }
});

it("preserves code newlines in a DOM edit spanning neighbouring blocks", async () => {
  const host = document.createElement("div");
  const view = new EditorView(host, {
    state: createEditorState(parseMarkdown("Intro\n\n```text\nline one\nline two\n```")),
    nodeViews: { block: createBlockNodeView },
  });
  try {
    host.querySelector("p")!.firstChild!.textContent = "Updated intro";
    host.querySelector("code")!.firstChild!.textContent = "line one\nline 2";
    await waitFor(() => expect(view.state.doc.firstChild!.textContent).toBe("Updated intro"));
    expect(view.state.doc.child(1).textContent).toBe("line one\nline 2");
  } finally {
    view.destroy();
  }
});
