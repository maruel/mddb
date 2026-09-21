// Installs jsdom globals, the Solid TSX transform, and asset stubs before tests run.
// Loaded via `node --test --import ./frontend/tests/setup-dom.ts` so every test-file child
// process gets a DOM before module evaluation, like vitest's environment: "jsdom".
import { readFileSync } from "node:fs";
import { registerHooks } from "node:module";
import { fileURLToPath } from "node:url";
import babel from "@babel/core";
import solidPreset from "babel-preset-solid";
import typescriptPreset from "@babel/preset-typescript";
import { JSDOM } from "jsdom";
import { afterEach } from "node:test";

// Load the assertion facade FIRST, before the jsdom globals are installed below: chai 6
// subclasses the global `Event` at module-evaluation time for its plugin events, so it must
// capture Node's native Event. Everything evaluated after the globals block (components,
// tests) sees jsdom's Event, matching vitest's jsdom environment.
await import("@tests/expect");

const dom = new JSDOM('<!doctype html><html><body id="root"></body></html>', {
  url: "http://localhost:3000/",
  pretendToBeVisual: true,
});

const globals = dom.window as unknown as Record<string, unknown>;
const globalTarget = globalThis as unknown as Record<string, unknown>;
for (const key of [
  "document",
  "HTMLElement",
  "HTMLInputElement",
  "HTMLSelectElement",
  "HTMLTextAreaElement",
  "HTMLHeadElement",
  "HTMLBodyElement",
  "HTMLHtmlElement",
  "HTMLAnchorElement",
  "HTMLButtonElement",
  "HTMLIFrameElement",
  "SVGElement",
  "Element",
  "Node",
  "DOMParser",
  "XMLSerializer",
  "NodeFilter",
  "Event",
  "EventTarget",
  "CustomEvent",
  "KeyboardEvent",
  "MouseEvent",
  "getComputedStyle",
  "requestAnimationFrame",
  "cancelAnimationFrame",
  "localStorage",
  "sessionStorage",
]) {
  const value = globals[key];
  if (value !== undefined) globalTarget[key] = value;
}
globalTarget["window"] = dom.window;

// Delegate the timer family from the jsdom window to globalThis so node:test's fake
// timers (which replace the globals) govern `window.setTimeout` calls too. Late-bound:
// reads the current global at call time so fake-timer swaps are honored.
for (const timer of [
  "setTimeout",
  "setInterval",
  "clearTimeout",
  "clearInterval",
  "setImmediate",
  "clearImmediate",
  "queueMicrotask",
]) {
  if (globalTarget[timer] !== undefined) {
    Object.defineProperty(dom.window, timer, {
      configurable: true,
      value: (...args: unknown[]) => {
        const fn = (globalThis as unknown as Record<string, ((...args: unknown[]) => unknown) | undefined>)[timer];
        return fn?.(...args);
      },
    });
  }
}

// jsdom does not implement window.matchMedia; stub it so components that call
// it (e.g. for touch-device detection) don't throw in tests.
const matchMediaStub = (query: string) => ({
  matches: false,
  media: query,
  onchange: null,
  addListener: () => {},
  removeListener: () => {},
  addEventListener: () => {},
  removeEventListener: () => {},
  dispatchEvent: () => false,
});
Object.defineProperty(dom.window, "matchMedia", { configurable: true, value: matchMediaStub });
globalTarget["matchMedia"] = matchMediaStub;

Object.defineProperty(globalTarget, "navigator", { value: dom.window.navigator, configurable: true });

// jsdom does not implement window scrolling, and components scroll on mount;
// stub it as a no-op.
globalTarget["scrollTo"] = () => {};
dom.window.scrollTo = () => {};

// Now that the DOM exists, register jest-dom matchers (imports @testing-library/dom, whose
// `screen` binds document.body at module-evaluation time) and the testing-library cleanup hook.
const { registerDomMatchers } = await import("@tests/expect");
await registerDomMatchers();
const { cleanup } = await import("@solidjs/testing-library");
afterEach(cleanup);

// CSS modules, SVG components (?solid via vite-solid-svg), and ?url assets are
// build-time transforms the node runner does not perform; stub them here.
const STUB_SUFFIXES = [".css", ".png", ".jpg", ".jpeg", ".woff", ".woff2"];

const isStubbedSpecifier = (specifier: string): boolean => {
  const path = specifier.split("?")[0] ?? specifier;
  if (specifier.includes("?url") || specifier.includes("?solid")) return true;
  return STUB_SUFFIXES.some((suffix) => path.endsWith(suffix));
};

// Solid TSX must go through babel-preset-solid (the vite-plugin-solid transform);
// esbuild cannot compile Solid's JSX, and solid-js ships no jsx-runtime functions.
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (isStubbedSpecifier(specifier)) {
      return { url: `test-asset:${specifier}`, shortCircuit: true };
    }
    return nextResolve(specifier, context);
  },
  load(url, context, nextLoad) {
    if (url.startsWith("test-asset:")) {
      const specifier = url.slice("test-asset:".length);
      if (specifier.includes("?solid")) {
        // Solid components expect a component function returning a DOM node; forward the
        // props (data-testid, class, ...) as attributes so queries keep working.
        return {
          format: "module",
          shortCircuit: true,
          source:
            `export default (props) => {\n` +
            `  const el = globalThis.document.createElement("span");\n` +
            `  for (const key of Object.keys(props)) {\n` +
            `    if (key !== "children") el.setAttribute(key, String(props[key]));\n` +
            `  }\n` +
            `  return el;\n` +
            `};`,
        };
      }
      if (specifier.includes("?url")) {
        return { format: "module", shortCircuit: true, source: `export default ${JSON.stringify(specifier)};` };
      }
      if (specifier.endsWith(".css")) {
        // CSS modules: class-name lookups resolve to the key itself, like vitest's default.
        return {
          format: "module",
          shortCircuit: true,
          source: `const styles = new Proxy({}, { get: (_target, key) => String(key) }); export default styles;`,
        };
      }
      // Images and fonts: a stable URL string.
      return { format: "module", shortCircuit: true, source: `export default ${JSON.stringify(specifier)};` };
    }
    if (/\.tsx$/.test(url.split("?")[0] ?? url)) {
      const filePath = fileURLToPath(url.split("?")[0] ?? url);
      const result = babel.transformSync(readFileSync(filePath, "utf8"), {
        filename: filePath,
        sourceType: "module",
        presets: [
          [solidPreset, { moduleName: "solid-js/web", generate: "dom", hydratable: false }],
          [typescriptPreset, { isTSX: true, allExtensions: true }],
        ],
      });
      return { format: "module", shortCircuit: true, source: result?.code ?? "" };
    }
    return nextLoad(url, context);
  },
});
