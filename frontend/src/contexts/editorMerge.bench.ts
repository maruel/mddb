// Measures bounded conflict comparison work for ordinary, maximal, and oversized document edits.

import { bench } from "@tests/bench";
import { mergeText } from "./editorMerge";

const base = Array.from({ length: 498 }, (_, i) => `line ${i}\n`).join("");
const local = base.replaceAll("line", "local");
const saved = base.replaceAll("line", "saved");
bench("merges separated edits in a 498-line page", () => {
  mergeText(base, base.replace("line 0\n", "local first\n"), base.replace("line 497\n", "saved last\n"));
});
bench("compares two complete rewrites at the diff cell limit", () => {
  mergeText(base, local, saved);
});
const oversized = "content\n".repeat(100_000);
bench("offers manual recovery above the character budget", () => {
  mergeText(oversized, `${oversized}local`, `${oversized}saved`);
});

const long = Array.from({ length: 5000 }, (_, i) => `paragraph ${i} 🌳\n`).join("");
const sparseLocal = long.replace("paragraph 5 🌳\n", "local first\n").replace("paragraph 4900 🌳\n", "local last\n");
const sparseSaved = long.replace("paragraph 20 🌳\n", "saved first\n").replace("paragraph 4800 🌳\n", "saved last\n");
bench("compares sparse changes across a 5000-line document", () => {
  mergeText(long, sparseLocal, sparseSaved);
});
const repeated = "same paragraph 🌳\n".repeat(5000);
bench("bounds ambiguous repeated lines with distant changes", () => {
  mergeText(repeated, `local start\n${repeated}local end\n`, `saved start\n${repeated}saved end\n`);
});
const dense = Array.from({ length: 1000 }, (_, i) => `base ${i}\n`).join("");
bench("bounds dense rewrites above the comparison budget", () => {
  mergeText(dense, dense.replaceAll("base", "local"), dense.replaceAll("base", "saved"));
});
