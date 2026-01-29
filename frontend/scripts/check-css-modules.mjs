import { readFileSync, readdirSync, statSync } from 'fs';
import { dirname, join, basename, relative } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const srcDir = join(__dirname, '..', 'src');

/** Extract class names from a CSS module file */
function extractCssClasses(cssContent) {
  const classes = new Set();

  // Remove :global() blocks first - classes inside these are external/library classes
  // that aren't meant to be referenced via styles.className
  const contentWithoutGlobals = cssContent.replace(/:global\([^)]*\)/g, '');

  // Match class selectors: .className
  // Handles: .foo, .foo:hover, .foo::before, .foo.bar, .foo > .bar
  const classRegex = /\.([a-zA-Z_][a-zA-Z0-9_-]*)/g;
  let match;
  while ((match = classRegex.exec(contentWithoutGlobals)) !== null) {
    classes.add(match[1]);
  }
  return classes;
}

/** Extract the import name used for a CSS module in a TSX file */
function extractImportName(tsxContent, cssFileName) {
  // Match: import styles from './Foo.module.css' or import myStyles from './Foo.module.css'
  const importRegex = new RegExp(
    `import\\s+([a-zA-Z_][a-zA-Z0-9_]*)\\s+from\\s+['"][^'"]*${cssFileName.replace('.', '\\.')}['"]`,
    'g'
  );
  const match = importRegex.exec(tsxContent);
  return match ? match[1] : 'styles';
}

/** Check if a class is referenced in TSX content with given import name */
function isClassUsed(className, tsxContent, importName) {
  // Match importName.className or importName['className'] or ${importName.className}
  const patterns = [
    new RegExp(`${importName}\\.${className}(?![a-zA-Z0-9_-])`, 'g'),
    new RegExp(`${importName}\\['${className}'\\]`, 'g'),
    new RegExp(`${importName}\\["${className}"\\]`, 'g'),
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
    const cssFileName = basename(cssModule);
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

    // Check each consumer with its specific import name
    const unusedClasses = new Set(classes);
    for (const consumerPath of consumers) {
      const tsxContent = readFileSync(consumerPath, 'utf-8');
      const importName = extractImportName(tsxContent, cssFileName);

      for (const className of unusedClasses) {
        if (isClassUsed(className, tsxContent, importName)) {
          unusedClasses.delete(className);
        }
      }
    }

    if (unusedClasses.size > 0) {
      results.push({
        file: relative(srcDir, cssModule),
        issue: 'Unused classes',
        classes: Array.from(unusedClasses),
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
