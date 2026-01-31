#!/usr/bin/env python3
import os
import subprocess
import sys


def is_binary(file_path):
    """Simple binary detection by checking for null bytes in the first 1024 bytes."""
    if not os.path.isfile(file_path):
        return False
    try:
        with open(file_path, "rb") as f:
            chunk = f.read(1024)
            return b"\0" in chunk
    except Exception:
        return False


def main():
    try:
        # Get all tracked files
        files = subprocess.check_output(["git", "ls-files"], text=True).splitlines()
    except subprocess.CalledProcessError as e:
        print(f"Error running git: {e}", file=sys.stderr)
        return 1

    binaries = []
    for f in files:
        if is_binary(f):
            # Exclude known binary types that are allowed if any (e.g. icons/images)
            # For mddb, the Makefile target was quite broad but typically images
            # are in data/ or frontend/src/assets
            ext = os.path.splitext(f)[1].lower()
            if ext in [".ico", ".jpg", ".gif", ".png", ".svg", ".webp"]:
                continue
            binaries.append(f)

    if binaries:
        print("Error: Binary executables found in repository:")
        for b in binaries:
            print(b)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
