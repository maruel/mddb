// Tests conservative three-way merging, overlapping changes, and bounded manual recovery.

import { it } from "node:test";
import { expect } from "@tests/expect";
import { mergeText } from "./editorMerge";

it("merges independent edits and preserves line endings", () => {
  const merged = mergeText("one\ntwo\nthree\n", "ONE\ntwo\nthree\n", "one\ntwo\nTHREE\n");
  expect(merged.parts.map((part) => ("text" in part ? part.text : "CONFLICT")).join("")).toBe("ONE\ntwo\nTHREE\n");
});
it("retains overlapping alternatives surrounded by independently merged edits", () => {
  const merged = mergeText("a\nb\nc\nd\n", "A\nours\nc\nd\n", "a\ntheirs\nc\nD\n");
  expect(merged.parts).toContainEqual({ local: "ours\n", saved: "theirs\n" });
  expect(merged.parts).toContainEqual({ text: "A\n" });
  expect(merged.parts.at(-2)).toEqual({ text: "D\n" });
});
it("handles insertions at the same location as an explicit overlap", () => {
  expect(mergeText("a\n", "one\na\n", "two\na\n").parts).toContainEqual({ local: "one\n", saved: "two\n" });
});
it("accepts identical edits and one unchanged side", () => {
  expect(mergeText("before", "after", "after").parts).toEqual([{ text: "after" }]);
  expect(mergeText("before", "before", "after").parts).toEqual([{ text: "after" }]);
});
it("bounds pathological comparisons with a manual alternative", () => {
  const base = Array.from({ length: 1000 }, (_, i) => `base ${i}\n`).join("");
  expect(mergeText(base, base.replaceAll("base", "local"), base.replaceAll("base", "saved")).manual).toBe(true);
});
