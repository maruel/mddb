#!/usr/bin/env python3
"""Builds the frontend into //backend/frontend/dist/, to be embedded in the backend executable.

After the Vite build, scripts/precompress_dist.py brotli-compresses every file at
maximum quality and deletes the originals, so only .br files are embedded in the Go
binary.
"""

import subprocess
import sys
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parent.parent
DIST_DIR = ROOT_DIR / "backend" / "frontend" / "dist"


def run(command: list[str]) -> int:
    try:
        subprocess.check_call(command, cwd=ROOT_DIR)
    except subprocess.CalledProcessError as error:
        print(f"{error.cmd[0]} failed with status {error.returncode}", file=sys.stderr)
        return 1
    return 0


def main() -> int:
    for command in (
        ["pnpm", "install", "--frozen-lockfile", "--silent"],
        ["pnpm", "--silent", "build", "--logLevel", "silent"],
    ):
        status = run(command)
        if status != 0:
            return status
    return run(
        [
            sys.executable,
            str(ROOT_DIR / "scripts" / "precompress_dist.py"),
            "--dist",
            str(DIST_DIR),
            "--delete-originals",
            "--encodings",
            "br",
        ]
    )


if __name__ == "__main__":
    sys.exit(main())
