// Script to generate a JSON list of all Material Symbols Outlined icon names.
// Reads filenames from @material-symbols/svg-400/outlined/, excludes -fill variants.
// Output: frontend/src/components/editor/material-symbols-names.json

import { readdirSync, writeFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const outlined = join(__dirname, '../../node_modules/@material-symbols/svg-400/outlined');
const outFile = join(__dirname, '../src/components/editor/material-symbols-names.json');

const names = readdirSync(outlined)
  .filter((f) => f.endsWith('.svg') && !f.endsWith('-fill.svg'))
  .map((f) => f.slice(0, -4)) // strip .svg
  .sort();

writeFileSync(outFile, JSON.stringify(names, null, 2) + '\n');
console.log(`Generated ${names.length} icon names → ${outFile}`);
