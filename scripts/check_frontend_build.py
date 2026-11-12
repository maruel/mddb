#!/usr/bin/env python3
"""Refuse to serve an embedded frontend that is older than its sources."""

import sys
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parent.parent
BUILD_MARKER = ROOT_DIR / "backend" / "frontend" / "dist" / "index.html.br"
SOURCES = ("frontend/index.html", "frontend/public", "frontend/src", "sdk/mddb/ts", "vite.config.ts")


def bundled(path: Path) -> bool:
    """Reports whether path can change the built bundle; tests and type declarations cannot."""
    return path.is_file() and ".test." not in path.name and not path.name.endswith(".d.ts")


def newest_source() -> Path:
    """Returns the most recently modified bundle input."""
    paths = []
    for source in SOURCES:
        root = ROOT_DIR / source
        paths.extend(p for p in (root.rglob("*") if root.is_dir() else [root]) if bundled(p))
    return max(paths, key=lambda p: p.stat().st_mtime)


def main() -> int:
    source = newest_source()
    if BUILD_MARKER.exists() and BUILD_MARKER.stat().st_mtime >= source.stat().st_mtime:
        return 0
    print(
        f"The embedded frontend is older than {source.relative_to(ROOT_DIR)}.\n"
        "Run `make frontend-build`, or use `make test-e2e SPEC=e2e/file.spec.ts`, which builds first.",
        file=sys.stderr,
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
