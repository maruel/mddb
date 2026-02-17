#!/usr/bin/env python3
"""Builds the frontend into //backend/frontend/dist/, to be embedded in the backend executable.

After the Vite build, all files are brotli-compressed at maximum quality and the
originals deleted so only .br files are embedded in the Go binary.
"""

import os
import subprocess
import sys


def brotli_compress_dist(dist_dir: str) -> None:
    """Walk dist_dir recursively, brotli --best each file, delete originals."""
    for dirpath, _, filenames in os.walk(dist_dir):
        for name in filenames:
            if name.endswith(".br"):
                continue
            filepath = os.path.join(dirpath, name)
            br_path = filepath + ".br"
            subprocess.check_call(["brotli", "--best", "--keep", filepath, "-o", br_path])
            os.remove(filepath)


def main():
    # Change directory to the root of the project
    # This script is expected to be in scripts/
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    os.chdir(root_dir)
    try:
        subprocess.check_call(["pnpm", "install", "--frozen-lockfile", "--silent"])
    except subprocess.CalledProcessError as e:
        print(f"Error installing dependencies: {e}", file=sys.stderr)
        return 1
    try:
        subprocess.check_call(["pnpm", "--silent", "build", "--logLevel", "silent"])
    except subprocess.CalledProcessError as e:
        print(f"Error building frontend: {e}", file=sys.stderr)
        return 1

    # Precompress all dist files with brotli
    dist_dir = os.path.join(root_dir, "backend", "frontend", "dist")
    try:
        brotli_compress_dist(dist_dir)
    except subprocess.CalledProcessError as e:
        print(f"Error compressing frontend assets: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
