#!/usr/bin/env python3
"""Verify e2e test data integrity after tests run."""

import sys
from pathlib import Path

DATA_DIR = Path("./data")


def parse_frontmatter(content: str) -> tuple[dict, str]:
    """Parse YAML frontmatter from markdown content."""
    if not content.startswith("---"):
        return {}, content
    end = content.find("\n---\n", 3)
    if end == -1:
        return {}, content
    fm = {}
    for line in content[4:end].split("\n"):
        if ":" in line:
            k, v = line.split(":", 1)
            fm[k.strip()] = v.strip()
    return fm, content[end + 5 :]


def scan_pages() -> list[tuple[Path, str, str]]:
    """Scan data directory, return list of (path, title, content)."""
    pages = []
    for ws_dir in DATA_DIR.iterdir():
        if not ws_dir.is_dir() or ws_dir.name.startswith("."):
            continue
        for node_dir in ws_dir.iterdir():
            if not node_dir.is_dir() or node_dir.name.startswith("."):
                continue
            index_md = node_dir / "index.md"
            if index_md.exists():
                fm, body = parse_frontmatter(index_md.read_text())
                pages.append((index_md, fm.get("title", ""), body.strip()))
    return pages


def main() -> int:
    if not DATA_DIR.exists():
        print(f"ERROR: {DATA_DIR} does not exist")
        return 1

    pages = scan_pages()
    if not pages:
        print("ERROR: No pages found")
        return 1

    errors = []

    # Check rename-on-navigation test worked
    renamed = [p for p in pages if p[1] == "RENAMED PAGE TITLE"]
    old_title = [p for p in pages if p[1] == "Page To Rename"]
    if not renamed:
        errors.append("No 'RENAMED PAGE TITLE' pages - flush on navigation broken")
    if old_title:
        errors.append(f"Found {len(old_title)} 'Page To Rename' pages - rename was lost")

    # Check rapid rename test
    final_rename = [p for p in pages if p[1] == "FINAL RENAME"]
    if not final_rename:
        errors.append("No 'FINAL RENAME' pages - rapid rename broken")

    # Check content flush test
    modified_content = [p for p in pages if "MODIFIED CONTENT" in p[2]]
    if not modified_content:
        errors.append("No pages with 'MODIFIED CONTENT' - content flush broken")

    # Check combined title+content test
    new_title = [p for p in pages if p[1] == "NEW TITLE"]
    if not new_title:
        errors.append("No 'NEW TITLE' pages - combined flush broken")

    # Check autosave control test
    updated_title = [p for p in pages if p[1] == "Updated Title"]
    if not updated_title:
        errors.append("No 'Updated Title' pages - autosave broken")

    # Report
    print(f"Scanned {len(pages)} pages in {DATA_DIR}")
    if errors:
        for e in errors:
            print(f"ERROR: {e}")
        return 1

    print("All e2e data verifications passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
