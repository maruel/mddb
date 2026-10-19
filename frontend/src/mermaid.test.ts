// Tests Pele diagram compatibility, security limits, caching, and accessible insertion.
import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { renderMermaidDiagram, insertMermaidSvg } from "./mermaid";

describe("Mermaid rendering", () => {
  it("renders valid diagrams to SVG strings", async () => {
    const svg = await renderMermaidDiagram("graph TD;\nA-->B;");
    expect(svg).not.toBeNull();
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
  });

  it("returns null for invalid syntax without throwing", async () => {
    const svg = await renderMermaidDiagram("invalid diagram syntax %%%");
    expect(svg).toBeNull();
  });

  it("returns null for empty diagram text", async () => {
    expect(await renderMermaidDiagram("   ")).toBeNull();
  });

  it("caches rendered SVGs", async () => {
    const first = await renderMermaidDiagram("graph TD;\nX-->Y;");
    expect(first).not.toBeNull();

    const second = await renderMermaidDiagram("graph TD;\nX-->Y;");
    expect(second).toBe(first);
  });

  for (const [name, code] of [
    ["sequence", "sequenceDiagram\nAlice->>Bob: Hello"],
    ["class", "classDiagram\nAnimal <|-- Duck"],
    ["state", "stateDiagram-v2\n[*] --> Ready\nReady --> [*]"],
    ["architecture", "architecture-beta\nservice api(server)[API]\nservice db(database)[Database]\napi:R -- L:db"],
  ] as const) {
    it(`renders ${name} diagrams`, async () => {
      expect(await renderMermaidDiagram(code)).toContain("<svg");
    });
  }

  it("disables links, remote images, and HTML labels", async () => {
    const svg = await renderMermaidDiagram(`flowchart LR
      A["<img src=x onerror=alert(1)>"] --> B
      B@{ img: "https://example.com/diagram.png", label: "Remote image" }
      click A "https://example.com/"
    `);
    expect(svg).not.toBeNull();
    const container = document.createElement("div");
    container.innerHTML = svg ?? "";
    expect(container.querySelector("script, style, foreignObject, a, image, img, [href], [onerror]")).toBeNull();
    expect(svg).not.toContain("https://example.com");
  });

  it("falls back for oversized sources and expanded edge counts", async () => {
    expect(await renderMermaidDiagram("flowchart TD\n" + "A".repeat(50_000))).toBeNull();
    const edges = Array.from({ length: 501 }, (_, i) => `A --> B${i}`).join("\n");
    expect(await renderMermaidDiagram(`flowchart TD\n${edges}`)).toBeNull();
  });

  it("gives repeated cached diagrams independent accessible titles", async () => {
    const svg = await renderMermaidDiagram("flowchart LR\naccTitle: Pipeline\naccDescr: Build and test\nA --> B");
    const first = document.createElement("div");
    const second = document.createElement("div");
    insertMermaidSvg(first, svg!);
    insertMermaidSvg(second, svg!);
    expect(first.querySelector("title")!.id).not.toBe(second.querySelector("title")!.id);
    for (const el of [first, second]) {
      expect(el.querySelector("svg")!.getAttribute("aria-labelledby")).toBe(el.querySelector("title")!.id);
      expect(el.querySelector("svg")!.getAttribute("aria-describedby")).toBe(el.querySelector("desc")!.id);
    }
  });
});
