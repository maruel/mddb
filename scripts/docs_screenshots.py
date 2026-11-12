#!/usr/bin/env python3
"""Capture mddb's documentation scenes and keep their lossless WebP baselines in e2e/screenshots/docs.

e2e/docs-screenshots.spec.ts owns the scenes; each takeScreenshot name becomes
<name>.webp. `update` renders the scenes twice against fresh e2e servers,
requires identical decoded pixels, then replaces the baselines. `check` renders
once and requires the baseline pixels. Fonts and the browser build change
pixels, so compare only captures made in the same environment. Run through
`make screenshots-update` or `make screenshots-check`, which rebuild the
embedded frontend first. Captures stay in test-results/docs-screenshots.
scripts/publish_screenshots.py copies the referenced baselines into a website.
"""

import argparse
import json
import pathlib
import re
import shutil
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
SPEC = "e2e/docs-screenshots.spec.ts"
BASELINES = ROOT / "e2e/screenshots/docs"
WORK = ROOT / "test-results/docs-screenshots"
# takeScreenshot in e2e/helpers.ts writes <index>_<name>.png in each test's output directory.
CAPTURE = re.compile(r"\d{2}_([a-z-]+)\.png")


def render(output: pathlib.Path) -> dict[str, pathlib.Path]:
    """Run the scenes against a fresh e2e server and return their PNG captures by name."""
    subprocess.run(
        ["pnpm", "exec", "playwright", "test", SPEC, "--output", str(output), "--reporter=line"],
        cwd=ROOT,
        check=True,
    )
    captures: dict[str, pathlib.Path] = {}
    for path in sorted(output.rglob("*.png")):
        match = CAPTURE.fullmatch(path.name)
        if match is None:
            continue
        name = match.group(1)
        if name in captures:
            raise RuntimeError(f"Scene name {name} is captured twice")
        captures[name] = path
    if not captures:
        raise RuntimeError(f"{SPEC} captured nothing")
    return captures


def pixels(path: pathlib.Path) -> tuple[int, int, bytes]:
    """Decode an image into its dimensions and RGBA pixels."""
    probe = subprocess.run(
        ["ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "json"]
        + [str(path)],
        check=True,
        capture_output=True,
        text=True,
    )
    stream = json.loads(probe.stdout)["streams"][0]
    raw = subprocess.run(
        ["ffmpeg", "-v", "error", "-threads", "1", "-i", str(path), "-pix_fmt", "rgba", "-f", "rawvideo", "-"],
        check=True,
        capture_output=True,
    ).stdout
    return stream["width"], stream["height"], raw


def compare(actual: dict[str, pathlib.Path], expected: dict[str, pathlib.Path], label: str) -> None:
    if actual.keys() != expected.keys():
        raise RuntimeError(f"{label}: scenes differ: {sorted(actual)} versus {sorted(expected)}")
    changed = [name for name in sorted(actual) if pixels(actual[name]) != pixels(expected[name])]
    if changed:
        raise RuntimeError(f"{label}: pixels differ in {', '.join(changed)}")


def encode(source: pathlib.Path, destination: pathlib.Path) -> None:
    """Encode a lossless WebP; one thread keeps the output bytes repeatable."""
    subprocess.run(
        ["ffmpeg", "-v", "error", "-y", "-threads", "1", "-i", str(source), "-threads", "1"]
        + ["-c:v", "libwebp", "-lossless", "1", "-compression_level", "6", "-map_metadata", "-1", str(destination)],
        check=True,
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("mode", choices=("check", "update"))
    mode = parser.parse_args().mode
    for executable in ("ffmpeg", "ffprobe", "pnpm"):
        if shutil.which(executable) is None:
            print(f"docs_screenshots: {executable} is required", file=sys.stderr)
            return 1
    try:
        first = render(WORK / "first")
        if mode == "check":
            baselines = {path.stem: path for path in BASELINES.glob("*.webp")}
            compare(first, baselines, f"{BASELINES.relative_to(ROOT)} baselines")
            print(f"{len(first)} captures match {BASELINES.relative_to(ROOT)}")
            return 0
        compare(first, render(WORK / "second"), "Two renders")
        encoded = WORK / "webp"
        shutil.rmtree(encoded, ignore_errors=True)
        encoded.mkdir()
        for name, path in first.items():
            encode(path, encoded / f"{name}.webp")
        BASELINES.mkdir(parents=True, exist_ok=True)
        names = {f"{name}.webp" for name in first}
        for stale in BASELINES.iterdir():
            if stale.name not in names:
                stale.unlink()
        for name in first:
            shutil.copyfile(encoded / f"{name}.webp", BASELINES / f"{name}.webp")
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"docs_screenshots: {error}; captures are in {WORK.relative_to(ROOT)}", file=sys.stderr)
        return 1
    print(f"Updated {len(first)} captures in {BASELINES.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
