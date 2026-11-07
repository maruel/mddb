// Tests for classifying workspace SSE events against the editor's own revision.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { classifyExternalUpdate } from "./editorSync";

describe("classifyExternalUpdate", () => {
  it("distinguishes content changes within one timestamp without advancing an unseen draft baseline", () => {
    expect(classifyExternalUpdate(100, 100, false, "new", "old")).toBe("reload");
    expect(classifyExternalUpdate(100, 100, true, "new", "old")).toBe("conflict");
    expect(classifyExternalUpdate(101, 100, true, "same", "same")).toBe("ignore");
  });
  it("ignores the echo of the editor's own write", () => {
    expect(classifyExternalUpdate(100, 100, false, null, null)).toBe("ignore");
    expect(classifyExternalUpdate(90, 100, true, null, null)).toBe("ignore");
  });

  it("reloads when a newer revision lands and nothing is unsaved", () => {
    expect(classifyExternalUpdate(101, 100, false, null, null)).toBe("reload");
    expect(classifyExternalUpdate(101, null, false, null, null)).toBe("reload");
  });

  it("reports a conflict when local edits are unsaved", () => {
    expect(classifyExternalUpdate(101, 100, true, null, null)).toBe("conflict");
    expect(classifyExternalUpdate(101, null, true, null, null)).toBe("conflict");
  });
});
