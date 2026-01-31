#!/usr/bin/env python3
import os
import shutil
import sys


def main():
    target = "./data-e2e"
    if os.path.exists(target):
        try:
            shutil.rmtree(target)
        except Exception as e:
            print(f"Error removing {target}: {e}", file=sys.stderr)
            return 1
    try:
        os.makedirs(target, exist_ok=True)
    except Exception as e:
        print(f"Error creating {target}: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
