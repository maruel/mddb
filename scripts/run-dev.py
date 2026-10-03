#!/usr/bin/env python3
"""Run the mddb development server with a temporary config directory.

Uses 'go run' to build and run the server, writing a temporary config.toml that
holds the listen address and data directory. --fake builds with the e2e build
tag, which selects the fake OAuth providers, the raised user quota, and the
10000x rate-limit multiplier the e2e suite needs.
"""

import argparse
import os
import shutil
import signal
import subprocess
import sys
import tempfile

ROOT_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--http", default=":8080", help='HTTP listen address (default: ":8080")')
    parser.add_argument("--data-dir", default="./data", help="Content directory (default: ./data)")
    parser.add_argument(
        "--fake",
        action="store_true",
        help="Build with the e2e tag: fake OAuth providers, a raised user quota, and a 10000x rate-limit multiplier",
    )
    args = parser.parse_args()

    tmp_dir = tempfile.mkdtemp(prefix="mddb-dev-")
    try:
        config_path = os.path.join(tmp_dir, "config.toml")
        with open(config_path, "w") as f:
            f.write(f'[server]\nhttp = "{args.http}"\ndata_dir = "{args.data_dir}"\n')

        cmd = ["go", "run"]
        if args.fake:
            cmd.extend(["-tags", "e2e"])
        cmd.extend(["./backend/cmd/mddb", "-config-dir", tmp_dir])

        proc = subprocess.Popen(cmd, cwd=ROOT_DIR)

        def handle_signal(signum, frame):
            proc.terminate()

        signal.signal(signal.SIGINT, handle_signal)
        signal.signal(signal.SIGTERM, handle_signal)

        proc.wait()
        return proc.returncode
    finally:
        shutil.rmtree(tmp_dir, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
