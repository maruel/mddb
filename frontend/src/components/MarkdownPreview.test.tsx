// Tests sanitized Markdown, asset URLs, diagram disclosure, and asynchronous preview replacement.
import { describe, it, beforeEach, afterEach } from "node:test";
import { expect, vi } from "@tests/expect";
import { createSignal } from "solid-js";
import { render, cleanup, waitFor } from "@solidjs/testing-library";
import Preview from "./MarkdownPreview";
import { I18nProvider } from "../i18n";
import type { ComponentProps } from "solid-js";

function MarkdownPreview(props: ComponentProps<typeof Preview>) {
  return (
    <I18nProvider>
      <Preview {...props} />
    </I18nProvider>
  );
}
import type { AssetUrlMap } from "../contexts/EditorContext";

// Mock CSS module
afterEach(() => {
  cleanup();
});

describe("MarkdownPreview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders markdown content as HTML", () => {
    const { container } = render(() => <MarkdownPreview content="# Hello World" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview).toBeTruthy();
    expect(preview?.innerHTML).toContain("<h1>");
    expect(preview?.innerHTML).toContain("Hello World");
  });

  it("renders paragraphs correctly", () => {
    const { container } = render(() => <MarkdownPreview content="This is a paragraph." />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain("<p>");
    expect(preview?.innerHTML).toContain("This is a paragraph.");
  });

  it("renders links correctly", () => {
    const { container } = render(() => <MarkdownPreview content="[Link Text](https://example.com)" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain("<a");
    expect(preview?.innerHTML).toContain('href="https://example.com"');
    expect(preview?.innerHTML).toContain("Link Text");
  });

  it("renders code blocks correctly", () => {
    const { container } = render(() => <MarkdownPreview content="```\ncode block\n```" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain("<code>");
    expect(preview?.innerHTML).toContain("code block");
  });

  it("renders inline code correctly", () => {
    const { container } = render(() => <MarkdownPreview content="Use `inline code` here" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain("<code>");
    expect(preview?.innerHTML).toContain("inline code");
  });

  it("renders lists correctly", () => {
    const { container } = render(() => <MarkdownPreview content="- Item 1\n- Item 2\n- Item 3" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain("<ul>");
    expect(preview?.innerHTML).toContain("<li>");
    expect(preview?.innerHTML).toContain("Item 1");
  });

  it("renders external image URLs unchanged", () => {
    const { container } = render(() => <MarkdownPreview content="![Alt](https://example.com/image.png)" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('src="https://example.com/image.png"');
  });

  it("rewrites local filenames to signed URLs when assetUrls provided", () => {
    const assetUrls: AssetUrlMap = {
      "image.png": "https://signed.example.com/image.png?sig=abc",
    };
    const { container } = render(() => <MarkdownPreview content="![Alt](image.png)" assetUrls={assetUrls} />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('src="https://signed.example.com/image.png?sig=abc"');
  });

  it("keeps filename if not in asset map", () => {
    const assetUrls: AssetUrlMap = {
      "other.png": "https://signed.example.com/other.png?sig=abc",
    };
    const { container } = render(() => <MarkdownPreview content="![Alt](unknown.png)" assetUrls={assetUrls} />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('src="unknown.png"');
  });

  it("keeps filename if assetUrls is empty", () => {
    const { container } = render(() => <MarkdownPreview content="![Alt](image.png)" assetUrls={{}} />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('src="image.png"');
  });

  it("does not rewrite external URLs even with assetUrls", () => {
    const assetUrls: AssetUrlMap = {
      "image.png": "https://signed.example.com/image.png?sig=abc",
    };
    const { container } = render(() => (
      <MarkdownPreview content="![Alt](https://cdn.example.com/image.png)" assetUrls={assetUrls} />
    ));

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('src="https://cdn.example.com/image.png"');
  });

  it("does not rewrite absolute paths even with assetUrls", () => {
    const assetUrls: AssetUrlMap = {
      "image.png": "https://signed.example.com/image.png?sig=abc",
    };
    const { container } = render(() => (
      <MarkdownPreview content="![Alt](/some/path/image.png)" assetUrls={assetUrls} />
    ));

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('src="/some/path/image.png"');
  });

  it("renders empty content correctly", () => {
    const { container } = render(() => <MarkdownPreview content="" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview).toBeTruthy();
    expect(preview?.innerHTML.trim()).toBe("");
  });

  it("has correct accessibility attributes", () => {
    const { container } = render(() => <MarkdownPreview content="Test" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.getAttribute("role")).toBe("region");
    expect(preview?.getAttribute("aria-label")).toBe("Markdown preview");
  });

  it("renders bold and italic text", () => {
    const { container } = render(() => <MarkdownPreview content="**bold** and *italic* text" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain("<strong>bold</strong>");
    expect(preview?.innerHTML).toContain("<em>italic</em>");
  });

  it("renders blockquotes", () => {
    const { container } = render(() => <MarkdownPreview content="> This is a quote" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain("<blockquote>");
    expect(preview?.innerHTML).toContain("This is a quote");
  });
});

it("renders diagrams with keyboard accessible source disclosure and invalid-source fallback", async () => {
  const [content, setContent] = createSignal("```mermaid title\nflowchart LR\nA --> B\n```");
  const { container } = render(() => <MarkdownPreview content={content()} />);
  await waitFor(() => expect(container.querySelector("svg")).not.toBeNull());
  expect(container.querySelector("details")!.open).toBe(false);
  container.querySelector("summary")!.click();
  expect(container.querySelector("details")!.open).toBe(true);
  expect(container.querySelector("code")!.textContent).toContain("A --> B");
  setContent("```mermaid\ninvalid diagram syntax %%%\n```");
  await waitFor(() => expect(container.querySelector("code")!.textContent).toContain("invalid diagram"));
  expect(container.querySelector("details")!.open).toBe(true);
  expect(container.querySelector("svg")).toBeNull();
});

it("does not insert an obsolete diagram after content is replaced", async () => {
  const [content, setContent] = createSignal("```mermaid\nflowchart LR\nObsolete --> Diagram\n```");
  const { container } = render(() => <MarkdownPreview content={content()} />);
  const oldDiagram = container.querySelector("[data-mermaid-diagram]");
  setContent("Replacement text");
  await new Promise<void>((resolve) => setTimeout(resolve, 0));
  expect(oldDiagram?.innerHTML).toBe("");
  expect(container.textContent).toBe("Replacement text\n");
  expect(container.querySelector("svg")).toBeNull();
});

it("preserves sanitized inline SVG and MathML in history content", () => {
  const { container } = render(() => (
    <MarkdownPreview content={'<svg onload="alert(1)"><circle r="4" /></svg><math><mi>x</mi></math>'} />
  ));
  expect(container.querySelector("svg circle")).not.toBeNull();
  expect(container.querySelector("math mi")?.textContent).toBe("x");
  expect(container.querySelector("[onload]")).toBeNull();
});
