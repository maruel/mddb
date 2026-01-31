#!/usr/bin/env python3
"""Builds the frontend into //backend/frontend/dist/, to be embedded in the backend executable."""

import os
import subprocess
import sys


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
    return 0


if __name__ == "__main__":
    sys.exit(main())
