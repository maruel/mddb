// Partitions bounded three-way comparisons into shared source regions and explicit overlapping alternatives.

interface SourceRegion {
  base: string;
  local: string;
  saved: string;
}
export type MergePart =
  (SourceRegion & { kind: "unchanged" | "merged"; text: string }) | (SourceRegion & { kind: "conflict" });
export interface TextMerge {
  parts: MergePart[];
  manual: boolean;
}
interface Edit {
  start: number;
  end: number;
  lines: string[];
}

// Bound both allocation and comparison work. Oversized changes remain recoverable manually.
const maxCharacters = 1_000_000;
const maxCells = 250_000;
const maxLines = 10_000;
const lines = (text: string): string[] => text.match(/[^\n]*\n|[^\n]+$/g) ?? [];

interface Span {
  start: number;
  end: number;
  otherStart: number;
  otherEnd: number;
}
interface Anchor {
  base: number;
  other: number;
}

function trim(base: string[], other: string[], span: Span): Span {
  let { start, end, otherStart, otherEnd } = span;
  while (start < end && otherStart < otherEnd && base[start] === other[otherStart]) {
    start++;
    otherStart++;
  }
  while (end > start && otherEnd > otherStart && base[end - 1] === other[otherEnd - 1]) {
    end--;
    otherEnd--;
  }
  return { start, end, otherStart, otherEnd };
}

// Unique common lines provide unambiguous partition boundaries. Keep their
// longest increasing subsequence so moves never reorder the saved source.
function anchors(base: string[], other: string[], span: Span): Anchor[] {
  const unique = (text: string[], start: number, end: number) => {
    const positions = new Map<string, number>();
    for (let i = start; i < end; i++) {
      const line = text[i];
      if (line !== undefined) positions.set(line, positions.has(line) ? -1 : i);
    }
    return positions;
  };
  const original = unique(base, span.start, span.end);
  const changed = unique(other, span.otherStart, span.otherEnd);
  const matches: Anchor[] = [];
  for (const [line, i] of original) {
    const j = changed.get(line);
    if (i >= 0 && j !== undefined && j >= 0) matches.push({ base: i, other: j });
  }
  const tails: number[] = [];
  const previous = new Int32Array(matches.length).fill(-1);
  for (let i = 0; i < matches.length; i++) {
    const match = matches[i];
    if (!match) continue;
    let left = 0,
      right = tails.length;
    while (left < right) {
      const mid = (left + right) >>> 1;
      const tail = tails[mid];
      const middle = tail === undefined ? undefined : matches[tail];
      if (middle && middle.other < match.other) left = mid + 1;
      else right = mid;
    }
    previous[i] = tails[left - 1] ?? -1;
    tails[left] = i;
  }
  const result: Anchor[] = [];
  for (let i = tails.at(-1) ?? -1; i >= 0; i = previous[i] ?? -1) {
    const match = matches[i];
    if (match) result.push(match);
  }
  return result.reverse();
}

function matrixEdits(base: string[], other: string[], span: Span): Edit[] {
  const n = span.end - span.start,
    m = span.otherEnd - span.otherStart;
  if (n === 0 || m === 0)
    return [{ start: span.start, end: span.end, lines: other.slice(span.otherStart, span.otherEnd) }];
  // Intern lines once: even long shared prefixes cost only integer comparisons in the matrix.
  const ids = new Map<string, number>();
  const encode = (line: string): number => {
    const known = ids.get(line);
    if (known !== undefined) return known;
    const id = ids.size;
    ids.set(line, id);
    return id;
  };
  const left = base.slice(span.start, span.end).map(encode);
  const right = other.slice(span.otherStart, span.otherEnd).map(encode);
  const width = m + 1;
  const lengths = new Uint32Array((n + 1) * width);
  for (let row = n - 1; row >= 0; row--)
    for (let col = m - 1; col >= 0; col--)
      lengths[row * width + col] =
        left[row] === right[col]
          ? 1 + (lengths[(row + 1) * width + col + 1] ?? 0)
          : Math.max(lengths[(row + 1) * width + col] ?? 0, lengths[row * width + col + 1] ?? 0);
  const result: Edit[] = [];
  let i = 0;
  let j = 0;
  let current: Edit | undefined;
  while (i < n || j < m) {
    if (i < n && j < m && left[i] === right[j]) {
      if (current) result.push(current);
      current = undefined;
      i++;
      j++;
    } else {
      current ??= { start: span.start + i, end: span.start + i, lines: [] };
      if (j < m && (i === n || (lengths[i * width + j + 1] ?? 0) >= (lengths[(i + 1) * width + j] ?? 0))) {
        current.lines.push(other[span.otherStart + j] ?? "");
        j++;
      } else {
        i++;
        current.end = span.start + i;
      }
    }
  }
  if (current) result.push(current);
  return result;
}

function edits(base: string[], other: string[]): Edit[] | null {
  const whole = trim(base, other, { start: 0, end: base.length, otherStart: 0, otherEnd: other.length });
  let spans = [whole];
  if ((whole.end - whole.start + 1) * (whole.otherEnd - whole.otherStart + 1) > maxCells) {
    const boundaries = anchors(base, other, whole);
    if (boundaries.length === 0) return null;
    spans = [];
    let start = whole.start,
      otherStart = whole.otherStart;
    for (const anchor of boundaries) {
      if (start < anchor.base || otherStart < anchor.other)
        spans.push(trim(base, other, { start, end: anchor.base, otherStart, otherEnd: anchor.other }));
      start = anchor.base + 1;
      otherStart = anchor.other + 1;
    }
    spans.push(trim(base, other, { start, end: whole.end, otherStart, otherEnd: whole.otherEnd }));
  }
  // The budget applies to all partitions together, not separately to each gap.
  let cells = 0;
  const result: Edit[] = [];
  for (const span of spans) {
    const n = span.end - span.start,
      m = span.otherEnd - span.otherStart;
    if (n === 0 && m === 0) continue;
    if (n !== 0 && m !== 0) cells += (n + 1) * (m + 1);
    if (cells > maxCells) return null;
    result.push(...matrixEdits(base, other, span));
  }
  // Separate adjacent replacements so an overlap preserves independent edits.
  return result.flatMap((edit) =>
    edit.end - edit.start === edit.lines.length && edit.lines.length > 1
      ? edit.lines.map((line, offset) => ({ start: edit.start + offset, end: edit.start + offset + 1, lines: [line] }))
      : [edit],
  );
}

function apply(base: string[], start: number, end: number, changes: Edit[]): string {
  const result: string[] = [];
  let cursor = start;
  for (const change of changes) {
    result.push(...base.slice(cursor, change.start), ...change.lines);
    cursor = change.end;
  }
  result.push(...base.slice(cursor, end));
  return result.join("");
}

function resolved(base: string, local: string, saved: string, text: string): MergePart {
  return { kind: base === local && base === saved ? "unchanged" : "merged", base, local, saved, text };
}

export function mergeText(base: string, local: string, saved: string): TextMerge {
  if (base === local && base === saved) return { parts: [resolved(base, local, saved, base)], manual: false };
  const automatic = local === saved || saved === base ? local : local === base ? saved : undefined;
  // Unambiguous resolutions stay cheap above the comparison budget. Admitted
  // documents still expose precise changed regions for review on either side.
  const fallback = (): TextMerge =>
    automatic === undefined
      ? { parts: [{ kind: "conflict", base, local, saved }], manual: true }
      : { parts: [resolved(base, local, saved, automatic)], manual: false };
  if (base.length + local.length + saved.length > maxCharacters) return fallback();
  const original = lines(base);
  const ours = lines(local);
  const theirs = lines(saved);
  if (Math.max(original.length, ours.length, theirs.length) > maxLines) return fallback();
  const localEdits = edits(original, ours);
  const savedEdits = saved === local ? localEdits : edits(original, theirs);
  if (!localEdits || !savedEdits) return fallback();
  const changes = [
    ...localEdits.map((edit) => ({ ...edit, side: "local" })),
    ...savedEdits.map((edit) => ({ ...edit, side: "saved" })),
  ].sort((a, b) => a.start - b.start || a.end - b.end);
  const parts: MergePart[] = [];
  let cursor = 0;
  let index = 0;
  while (index < changes.length) {
    const first = changes[index++];
    if (!first) break;
    const group = [first];
    let end = first.end;
    while (index < changes.length) {
      const next = changes[index];
      if (!next) break;
      if (next.start >= end && next.start !== first.start) break;
      group.push(next);
      end = Math.max(end, next.end);
      index++;
    }
    const prefix = original.slice(cursor, first.start).join("");
    if (prefix) parts.push(resolved(prefix, prefix, prefix, prefix));
    const localChanges = group.filter((change) => change.side === "local");
    const savedChanges = group.filter((change) => change.side === "saved");
    const localText = apply(original, first.start, end, localChanges);
    const savedText = apply(original, first.start, end, savedChanges);
    const baseText = original.slice(first.start, end).join("");
    if (!savedChanges.length) parts.push(resolved(baseText, localText, savedText, localText));
    else if (!localChanges.length || localText === savedText)
      parts.push(resolved(baseText, localText, savedText, savedText));
    else parts.push({ kind: "conflict", base: baseText, local: localText, saved: savedText });
    cursor = end;
  }
  const unchanged = original.slice(cursor).join("");
  if (unchanged) parts.push(resolved(unchanged, unchanged, unchanged, unchanged));
  return { parts, manual: false };
}
