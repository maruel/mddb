#!/bin/bash
# Deploy Playwright report to gh-pages branch and prepare for GitHub Pages upload.
# Required environment variables: GITHUB_SHA, GITHUB_WORKSPACE, GITHUB_ENV, PAGES_URL, GIT_REMOTE_URL
set -eu

SHORT_SHA="${GITHUB_SHA:0:7}"
DEPLOY_DIR=$(mktemp -d)
cd "$DEPLOY_DIR"

git init
git remote add origin "$GIT_REMOTE_URL"

# Fetch existing reports if branch exists
if git ls-remote --exit-code origin gh-pages >/dev/null 2>&1; then
  git fetch --depth=1 origin gh-pages
  git checkout gh-pages
fi

# Add new report
mkdir -p reports
rm -rf "reports/${SHORT_SHA}"
cp -r "${GITHUB_WORKSPACE}/playwright-report" "reports/${SHORT_SHA}"
date -u '+%Y-%m-%d %H:%M' > "reports/${SHORT_SHA}/.timestamp"
rm -f reports/latest
ln -s "${SHORT_SHA}" reports/latest

# Delete old reports, keeping only the 20 most recent
MAX_REPORTS=20
cd reports
for dir in */; do
  dir="${dir%/}"
  [ "$dir" = "latest" ] && continue
  ts=$(cat "${dir}/.timestamp" 2>/dev/null || echo "0000-00-00 00:00")
  echo "$ts $dir"
done | sort -r | tail -n +$((MAX_REPORTS + 1)) | while read -r _ _ old_dir; do
  echo "Deleting old report: $old_dir"
  rm -rf "$old_dir"
done
cd ..

# Generate index.html with links to all reports
cat > index.html <<'HEADER'
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Playwright Reports</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 800px; margin: 2rem auto; padding: 0 1rem; }
    h1 { border-bottom: 1px solid #eee; padding-bottom: 0.5rem; }
    a { color: #0366d6; text-decoration: none; }
    a:hover { text-decoration: underline; }
    table { border-collapse: collapse; width: 100%; margin-top: 1rem; }
    th, td { border: 1px solid #ddd; padding: 0.5rem; text-align: left; }
    th { background: #f6f8fa; }
    .latest { font-size: 1.2rem; margin: 1rem 0; }
  </style>
</head>
<body>
  <h1>Playwright Reports</h1>
  <p class="latest"><a href="reports/latest/">Latest Report</a></p>
  <table>
    <tr><th>Commit</th><th>Generated (UTC)</th><th>Report</th></tr>
HEADER

for dir in reports/*/; do
  dir=$(basename "$dir")
  [ "$dir" = "latest" ] && continue
  ts=$(cat "reports/${dir}/.timestamp" 2>/dev/null || echo "unknown")
  printf '%s\t<tr><td><code>%s</code></td><td>%s</td><td><a href="reports/%s/">View</a></td></tr>\n' "$ts" "$dir" "$ts" "$dir"
done | sort -r | cut -f2- >> index.html

echo '</table></body></html>' >> index.html

# Commit and push
git add -A
git -c user.name="github-actions[bot]" -c user.email="github-actions[bot]@users.noreply.github.com" \
  commit -m "Playwright report for ${SHORT_SHA}" || exit 0
git push origin HEAD:gh-pages

# Export deploy directory for pages upload
echo "DEPLOY_DIR=$DEPLOY_DIR" >> "$GITHUB_ENV"
