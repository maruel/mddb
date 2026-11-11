// Tests field-level record edit rebasing, including empty and removed values.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { rebaseRecordEdit } from "./recordEdit";

describe("rebaseRecordEdit", () => {
  it("keeps unrelated acknowledged fields while applying multiple edited fields", () => {
    expect(
      rebaseRecordEdit(
        { Name: "Original", Status: "Open", Flag: true },
        { Name: "Original", Status: "Done", Flag: false },
        { Name: "First", Status: "Open", Flag: true },
      ),
    ).toEqual({ Name: "First", Status: "Done", Flag: false });
  });
  it("applies deliberate empty, null, and removed fields", () => {
    expect(
      rebaseRecordEdit({ a: "x", b: "y", c: 1 }, { a: "", b: null }, { a: "remote", b: "remote", c: 3, d: 4 }),
    ).toEqual({ a: "", b: null, d: 4 });
  });
  it("preserves a new __proto__ field without changing any prototype", () => {
    const observed = { name: "original" };
    const submitted = { name: "original", ["__proto__"]: { polluted: true } };
    const latest = { name: "saved" };
    const result = rebaseRecordEdit(observed, submitted, latest);
    expect(Object.hasOwn(result, "__proto__")).toBe(true);
    expect(JSON.stringify(result)).toBe('{"name":"saved","__proto__":{"polluted":true}}');
    expect(Object.getPrototypeOf(result)).toBe(Object.prototype);
    expect(Object.getPrototypeOf(latest)).toBe(Object.prototype);
    expect(Object.hasOwn(Object.prototype, "polluted")).toBe(false);
    expect(observed).toEqual({ name: "original" });
    expect(latest).toEqual({ name: "saved" });
  });
  it("treats equivalent nested values as unchanged regardless of key order", () => {
    expect(rebaseRecordEdit({ a: { x: 1, y: [2] } }, { a: { y: [2], x: 1 } }, { a: { x: 3 } })).toEqual({
      a: { x: 3 },
    });
  });
});
