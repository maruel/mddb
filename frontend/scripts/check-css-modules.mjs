import { readFileSync, readdirSync, statSync } from 'fs';
import { dirname, join, basename, relative } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const srcDir = join(__dirname, '..', 'src');

/** Extract class names from a CSS module file */
function extractCssClasses(cssContent) {
  const classes = new Set();
  // Match class selectors: .className
  // Handles: .foo, .foo:hover, .foo::before, .foo.bar, .foo > .bar
  const classRegex = /\.([a-zA-Z_][a-zA-Z0-9_-]*)/g;
  let match;
  while ((match = classRegex.exec(cssContent)) !== null) {
    classes.add(match[1]);
  }
  return classes;
}

/** Check if a class is referenced in TSX content */
function isClassUsed(className, tsxContent) {
  // Match styles.className or styles['className'] or ${styles.className}
  const patterns = [
    new RegExp(`styles\\.${className}(?![a-zA-Z0-9_-])`, 'g'),
    new RegExp(`styles\\['${className}'\\]`, 'g'),
    new RegExp(`styles\\["${className}"\\]`, 'g'),
  ];
  return patterns.some((pattern) => pattern.test(tsxContent));
}

/** Find all files matching a pattern recursively */
function findFiles(dir, pattern) {
  const files = [];
  const entries = readdirSync(dir);

  for (const entry of entries) {
    const fullPath = join(dir, entry);
    const stat = statSync(fullPath);

    if (stat.isDirectory()) {
      files.push(...findFiles(fullPath, pattern));
    } else if (pattern.test(entry)) {
      files.push(fullPath);
    }
  }

  return files;
}

/** Find the TSX file(s) that import a CSS module */
function findTsxConsumers(cssModulePath, allTsxFiles) {
  const cssFileName = basename(cssModulePath);
  const consumers = [];

  for (const tsxFile of allTsxFiles) {
    const content = readFileSync(tsxFile, 'utf-8');
    // Check if this TSX imports the CSS module
    if (content.includes(cssFileName)) {
      consumers.push(tsxFile);
    }
  }

  return consumers;
}

function main() {
  const cssModules = findFiles(srcDir, /\.module\.css$/);
  const tsxFiles = findFiles(srcDir, /\.tsx$/);

  let hasUnused = false;
  const results = [];

  for (const cssModule of cssModules) {
    const cssContent = readFileSync(cssModule, 'utf-8');
    const classes = extractCssClasses(cssContent);
    const consumers = findTsxConsumers(cssModule, tsxFiles);

    if (consumers.length === 0) {
      results.push({
        file: relative(srcDir, cssModule),
        issue: 'No TSX file imports this CSS module',
        classes: [],
      });
      hasUnused = true;
      continue;
    }

    // Combine content from all consumers
    const combinedTsxContent = consumers
      .map((f) => readFileSync(f, 'utf-8'))
      .join('\n');

    const unusedClasses = [];
    for (const className of classes) {
      if (!isClassUsed(className, combinedTsxContent)) {
        unusedClasses.push(className);
      }
    }

    if (unusedClasses.length > 0) {
      results.push({
        file: relative(srcDir, cssModule),
        issue: 'Unused classes',
        classes: unusedClasses,
      });
      hasUnused = true;
    }
  }

  if (results.length === 0) {
    console.log('All CSS module classes are in use.');
    process.exit(0);
  }

  console.log('Unused CSS module classes found:\n');
  for (const result of results) {
    console.log(`${result.file}:`);
    if (result.classes.length === 0) {
      console.log(`  ${result.issue}`);
    } else {
      for (const cls of result.classes) {
        console.log(`  .${cls}`);
      }
    }
    console.log();
  }

  process.exit(1);
}

main();
