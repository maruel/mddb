// Computes bounded line-based three-way merges while retaining explicit overlapping alternatives.

export type MergePart = { text: string } | { local: string; saved: string };
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

function edits(base: string[], other: string[]): Edit[] | null {
  let start = 0;
  while (start < base.length && start < other.length && base[start] === other[start]) start++;
  let end = base.length;
  let otherEnd = other.length;
  while (end > start && otherEnd > start && base[end - 1] === other[otherEnd - 1]) {
    end--;
    otherEnd--;
  }
  const n = end - start;
  const m = otherEnd - start;
  if ((n + 1) * (m + 1) > maxCells) return null;
  // Intern lines once: even long shared prefixes cost only integer comparisons in the matrix.
  const ids = new Map<string, number>();
  const encode = (line: string): number => {
    const known = ids.get(line);
    if (known !== undefined) return known;
    const id = ids.size;
    ids.set(line, id);
    return id;
  };
  const left = base.slice(start, end).map(encode);
  const right = other.slice(start, otherEnd).map(encode);
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
      current ??= { start: start + i, end: start + i, lines: [] };
      if (j < m && (i === n || (lengths[i * width + j + 1] ?? 0) >= (lengths[(i + 1) * width + j] ?? 0))) {
        current.lines.push(other[start + j] ?? "");
        j++;
      } else {
        i++;
        current.end = start + i;
      }
    }
  }
  if (current) result.push(current);
  // Separate adjacent one-line replacements so choosing an overlap keeps nearby independent edits.
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

export function mergeText(base: string, local: string, saved: string): TextMerge {
  if (local === saved || saved === base) return { parts: [{ text: local }], manual: false };
  if (local === base) return { parts: [{ text: saved }], manual: false };
  const fallback = (): TextMerge => ({ parts: [{ local, saved }], manual: true });
  if (base.length + local.length + saved.length > maxCharacters) return fallback();
  const original = lines(base);
  const ours = lines(local);
  const theirs = lines(saved);
  if (Math.max(original.length, ours.length, theirs.length) > maxLines) return fallback();
  const localEdits = edits(original, ours);
  const savedEdits = edits(original, theirs);
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
    parts.push({ text: original.slice(cursor, first.start).join("") });
    const localChanges = group.filter((change) => change.side === "local");
    const savedChanges = group.filter((change) => change.side === "saved");
    const localText = apply(original, first.start, end, localChanges);
    const savedText = apply(original, first.start, end, savedChanges);
    if (!savedChanges.length) parts.push({ text: localText });
    else if (!localChanges.length || localText === savedText) parts.push({ text: savedText });
    else parts.push({ local: localText, saved: savedText });
    cursor = end;
  }
  parts.push({ text: original.slice(cursor).join("") });
  return { parts, manual: false };
}
