// Benchmarks markdown parsing and serialization round-trips on a realistic document.

import { bench } from "@tests/bench";

import { parseMarkdown } from "./markdown-parser";
import { serializeToMarkdown } from "./markdown-serializer";

// Exercises headings, lists, task blocks, fenced code, tables, links, and
// inline formatting in proportions a long page actually contains.
const document = [
  "# Release planning",
  "",
  "Draft the plan for the next release, then circulate it for review.",
  "",
  "## Checklist",
  "",
  "- [x] Collect feedback from the beta channel",
  "- [ ] Triage open regressions",
  "- [ ] Update the migration guide",
  "",
  "## Migration notes",
  "",
  "1. Back up the database before upgrading",
  "2. Run the schema migration with `--dry-run` first",
  "3. Verify the **counts** match the *expected* totals",
  "",
  "```sql",
  "SELECT id, title FROM pages WHERE modified > NOW() - INTERVAL '7 days';",
  "UPDATE pages SET archived = true WHERE id IN (1, 2, 3);",
  "```",
  "",
  "| Stage | Owner | Status |",
  "| ----- | ----- | ------ |",
  "| Backup | Ada | done |",
  "| Migrate | Grace | in progress |",
  "| Verify | Linus | blocked |",
  "",
  "See the [runbook](md://page/runbook-details) and the [tracker](https://example.com/issues) for details.",
  "",
  "> Rollbacks are only safe before the first write lands.",
  "",
  "### Follow-ups",
  "",
  "- Notify the #release channel",
  "  - Include the rollback window",
  "- Schedule the postmortem",
].join("\n");

bench("parses a realistic page document", () => {
  const doc = parseMarkdown(document);
  if (doc.childCount < 10) {
    throw new Error(`expected many blocks, parsed ${doc.childCount}`);
  }
});

bench("serializes a parsed page document", () => {
  const markdown = serializeToMarkdown(parseMarkdown(document));
  if (!markdown.includes("- [x] Collect feedback")) {
    throw new Error("expected task markup to survive the round-trip");
  }
});

bench("round-trips a page document", () => {
  const markdown = serializeToMarkdown(parseMarkdown(document));
  const again = serializeToMarkdown(parseMarkdown(markdown));
  if (markdown !== again) {
    throw new Error("round-trip is not stable");
  }
});
