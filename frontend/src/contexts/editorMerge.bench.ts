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
