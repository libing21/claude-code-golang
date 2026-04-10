#!/usr/bin/env bash
set -eu

# count-go-lines: Count source lines in common code files under the given repo path (default ".")
# Usage:
#   skills/count-go-lines/run.sh [path]
#
# It scans for *.go, *.ts, *.tsx, *.js, *.jsx files, excluding vendor/node_modules, and prints per-file
# line counts plus a total at the end.

repo="${1:-.}"

tmpfile="$(mktemp)"
find "$repo" -type f \
  \( -name "*.go" -o -name "*.ts" -o -name "*.tsx" -o -name "*.js" -o -name "*.jsx" \) \
  -not -path "*/vendor/*" -not -path "*/node_modules/*" | sort > "$tmpfile"

if [ ! -s "$tmpfile" ]; then
  rm -f "$tmpfile"
  echo "No source files found under: $repo"
  exit 0
fi

echo "Counting lines under: $repo"
xargs wc -l < "$tmpfile" | awk '
  $2 == "total" { printf "TOTAL  %d\n", $1; next }
  { printf "%7d  %s\n", $1, $2 }
'
rm -f "$tmpfile"
