// Lazy-loaded Pele renderer for Mermaid syntax: bounded caching and safe, accessible SVG insertion.

import DOMPurify from "dompurify";
import type * as peleApi from "pele/lazy";

type Pele = typeof peleApi;

let pelePromise: Promise<Pele> | null = null;
let nextDiagramId = 1;

const svgCache = new Map<string, string>();
const MAX_CACHE_SIZE = 100;
const MAX_SOURCE_LENGTH = 50_000;

/** Recognizes Mermaid fences with optional trailing fence metadata. */
export function isMermaidFence(info: string | null | undefined): boolean {
  return info?.trim().split(/\s+/)[0] === "mermaid";
}

function loadPele(): Promise<Pele> {
  pelePromise ??= import("pele/lazy").catch((error: unknown) => {
    pelePromise = null;
    throw error;
  });
  return pelePromise;
}

/**
 * Validates and renders a Mermaid diagram definition to an SVG string.
 * Returns null if the diagram syntax is invalid or rendering fails.
 */
export async function renderMermaidDiagram(code: string): Promise<string | null> {
  const trimmed = code.trim();
  // Bound input before Pele's lazy type detection preprocesses the source.
  if (!trimmed || trimmed.length > MAX_SOURCE_LENGTH) return null;

  const cached = svgCache.get(trimmed);
  if (cached !== undefined) return cached;

  let pele: Pele | null = null;
  try {
    pele = await loadPele();
    const { svg } = await pele.renderAsync(trimmed, {
      autoDirection: false,
      fontFamily: "system-ui",
      images: false,
      links: false,
      maxEdges: 500,
      outputLimit: 4_000_000,
      responsive: true,
    });
    // Diagram SVG uses a dedicated sanitizer. Preserve SVG styling,
    // but disallow embedded documents, remote resources, navigation, and CSS sheets.
    const safeSvg = DOMPurify.sanitize(svg, {
      USE_PROFILES: { svg: true, svgFilters: true },
      FORBID_TAGS: ["a", "foreignObject", "image", "style"],
    });

    if (svgCache.size >= MAX_CACHE_SIZE) {
      const oldest = svgCache.keys().next().value;
      if (oldest !== undefined) svgCache.delete(oldest);
    }
    svgCache.set(trimmed, safeSvg);
    return safeSvg;
  } catch (error) {
    if (!pele || !(error instanceof pele.PeleError)) console.error("Failed to render Mermaid diagram", error);
    return null;
  }
}

export function insertMermaidSvg(diagramEl: HTMLElement, svg: string): void {
  diagramEl.innerHTML = svg;
  const root = diagramEl.querySelector("svg");
  if (!root) return;
  // Cached drawings can appear several times in one document. Pele uses IDs for
  // accessible titles/descriptions; give each inserted instance its own names.
  const prefix = `mermaid_svg_${nextDiagramId++}`;
  for (const [tag, attribute] of [
    ["title", "aria-labelledby"],
    ["desc", "aria-describedby"],
  ] as const) {
    const label = root.querySelector(tag);
    if (label?.id) {
      label.id = `${prefix}_${tag}`;
      root.setAttribute(attribute, label.id);
    }
  }
}
