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
  expect(merged.parts).toContainEqual(expect.objectContaining({ local: "ours\n", saved: "theirs\n" }));
  expect(merged.parts).toContainEqual(expect.objectContaining({ text: "A\n" }));
  expect(merged.parts).toContainEqual(expect.objectContaining({ text: "D\n" }));
});
it("handles insertions at the same location as an explicit overlap", () => {
  expect(mergeText("a\n", "one\na\n", "two\na\n").parts).toContainEqual(
    expect.objectContaining({ local: "one\n", saved: "two\n" }),
  );
});
it("accepts identical edits and one unchanged side", () => {
  expect(mergeText("before", "after", "after").parts).toEqual([
    { kind: "merged", base: "before", local: "after", saved: "after", text: "after" },
  ]);
  expect(mergeText("before", "before", "after").parts).toEqual([
    { kind: "merged", base: "before", local: "before", saved: "after", text: "after" },
  ]);
});
it("bounds pathological comparisons with a manual alternative", () => {
  const base = Array.from({ length: 1000 }, (_, i) => `base ${i}\n`).join("");
  expect(mergeText(base, base.replaceAll("base", "local"), base.replaceAll("base", "saved")).manual).toBe(true);
});

it("merges separated sparse changes in a long document without manual recovery", () => {
  const original = Array.from({ length: 5000 }, (_, i) => `paragraph ${i} 🌳\n`);
  const ours = [...original];
  const theirs = [...original];
  ours[5] = "local first\n";
  ours[4900] = "local last\n";
  theirs[20] = "saved first\n";
  theirs[4800] = "saved last\n";
  const merged = mergeText(original.join(""), ours.join(""), theirs.join(""));
  const expected = [...ours];
  expected[20] = theirs[20]!;
  expected[4800] = theirs[4800]!;
  expect(merged.manual).toBe(false);
  expect(merged.parts.every((part) => "text" in part)).toBe(true);
  expect(merged.parts.map((part) => ("text" in part ? part.text : "CONFLICT")).join("")).toBe(expected.join(""));
});

it("retains exact source regions through sparse overlaps and moved unique lines", () => {
  const original = Array.from({ length: 3000 }, (_, i) => `unique ${i}\r\n`);
  const ours = [...original],
    theirs = [...original];
  ours[5] = "our overlap 🌳\n";
  theirs[5] = "their overlap 🌳\n";
  ours[2900] = "distant local\n";
  theirs.splice(1000, 0, theirs.splice(1500, 1)[0]!);
  const inputs = { base: original.join(""), local: ours.join(""), saved: theirs.join("") };
  const merged = mergeText(inputs.base, inputs.local, inputs.saved);
  expect(merged.manual).toBe(false);
  for (const side of ["base", "local", "saved"] as const)
    expect(merged.parts.map((part) => part[side]).join("")).toBe(inputs[side]);
  expect(merged.parts.filter((part) => part.kind === "conflict")).toEqual([
    { kind: "conflict", base: "unique 5\r\n", local: "our overlap 🌳\n", saved: "their overlap 🌳\n" },
  ]);
});
it("applies one comparison budget across every partition", () => {
  const block = Array.from({ length: 400 }, (_, i) => `base ${i}\n`).join("");
  const base = `${block}unique separator\n${block}`;
  const merged = mergeText(base, base.replaceAll("base", "local"), base.replaceAll("base", "saved"));
  expect(merged.manual).toBe(true);
});
it("falls back deterministically when repeated lines provide no unambiguous partition", () => {
  const base = "same 🌳\n".repeat(3000);
  const local = `local first\n${base}local last\n`,
    saved = `saved first\n${base}saved last\n`;
  expect(mergeText(base, local, saved)).toEqual({ manual: true, parts: [{ kind: "conflict", base, local, saved }] });
});
it("keeps source reconstruction exact for short insertions, deletions, repeats, and empty text", () => {
  const samples = ["", "one\n", "one\none\n", "two\none\n", "🌳\r\nlast", "last"];
  for (const base of samples)
    for (const local of samples)
      for (const saved of samples) {
        const merged = mergeText(base, local, saved);
        for (const side of ["base", "local", "saved"] as const)
          expect(merged.parts.map((part) => part[side]).join("")).toBe({ base, local, saved }[side]);
      }
});

it("partitions admitted one-sided changes and preserves trivial resolution above budgets", () => {
  const merged = mergeText("before\nkeep\n", "before\nkeep\n", "after\nkeep\n");
  expect(merged.parts).toEqual([
    { kind: "merged", base: "before\n", local: "before\n", saved: "after\n", text: "after\n" },
    { kind: "unchanged", base: "keep\n", local: "keep\n", saved: "keep\n", text: "keep\n" },
  ]);
  const large = "content\n".repeat(100_000);
  expect(mergeText(large, large, `${large}saved`)).toEqual({
    manual: false,
    parts: [{ kind: "merged", base: large, local: large, saved: `${large}saved`, text: `${large}saved` }],
  });
});
