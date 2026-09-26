// Tests for classifying workspace SSE events against the editor's own revision.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { classifyExternalUpdate } from "./editorSync";

describe("classifyExternalUpdate", () => {
  it("ignores the echo of the editor's own write", () => {
    expect(classifyExternalUpdate(100, 100, false)).toBe("ignore");
    expect(classifyExternalUpdate(90, 100, true)).toBe("ignore");
  });

  it("reloads when a newer revision lands and nothing is unsaved", () => {
    expect(classifyExternalUpdate(101, 100, false)).toBe("reload");
    expect(classifyExternalUpdate(101, null, false)).toBe("reload");
  });

  it("reports a conflict when local edits are unsaved", () => {
    expect(classifyExternalUpdate(101, 100, true)).toBe("conflict");
    expect(classifyExternalUpdate(101, null, true)).toBe("conflict");
  });
});
