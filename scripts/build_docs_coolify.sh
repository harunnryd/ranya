#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${1:-$ROOT_DIR/.coolify/docs}"

echo "Building docs into: $OUT_DIR"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

python3 -m mkdocs build -f "$ROOT_DIR/mkdocs.yml" -d "$OUT_DIR/en"
python3 -m mkdocs build -f "$ROOT_DIR/mkdocs.id.yml" -d "$OUT_DIR/id"
python3 -m mkdocs build -f "$ROOT_DIR/mkdocs.ja.yml" -d "$OUT_DIR/ja"

cat >"$OUT_DIR/index.html" <<'EOF'
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta http-equiv="refresh" content="0;url=/en/" />
    <meta name="viewport" content="width=device-width,initial-scale=1" />
    <title>Ranya Docs</title>
  </head>
  <body>
    <p>Redirecting to <a href="/en/">/en/</a> ...</p>
  </body>
</html>
EOF

echo "Done. Output:"
echo "  - $OUT_DIR/en"
echo "  - $OUT_DIR/id"
echo "  - $OUT_DIR/ja"
