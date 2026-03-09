// Script to generate a classified emoji list from unicode-emoji-json.
// Reads all emoji groups from the Unicode standard and outputs a compact JSON
// with group names, emoji characters, and names (for search) for the icon picker.
// Output: frontend/src/components/editor/emoji-groups.json

import { existsSync, readFileSync, writeFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const pkgData = join(__dirname, '../../node_modules/unicode-emoji-json/data-by-group.json');
const outFile = join(__dirname, '../src/components/editor/emoji-groups.json');

const raw = JSON.parse(readFileSync(pkgData, 'utf8'));

// Compact format: { g: groupName, e: [{ c: char, n: name }] }
// The data already contains only canonical (base) emojis — no skin-tone variants.
const groups = raw.map((g) => ({
  g: g.name,
  e: g.emojis.map((em) => ({ c: em.emoji, n: em.name })),
}));

const content = JSON.stringify(groups, null, 2) + '\n';
const existing = existsSync(outFile) ? readFileSync(outFile, 'utf8') : null;
if (existing !== content) writeFileSync(outFile, content);
