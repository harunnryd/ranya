#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

pattern='github.com/(deepgram|twilio|elevenlabs|openai)|github.com/harunnryd/ranya/pkg/providers/|github.com/harunnryd/ranya/pkg/transports/twilio'

matches="$(rg -n "$pattern" pkg \
  --glob '!pkg/providers/**' \
  --glob '!pkg/transports/**' \
  --glob '!**/*_test.go' \
  || true)"

if [[ -n "$matches" ]]; then
  echo "Vendor boundary violation detected (core package imports vendor-specific code):"
  echo "$matches"
  exit 1
fi

echo "Vendor boundary check passed."
