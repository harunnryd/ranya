#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-harunnryd/ranya}"
API_URL="${API_URL:-https://api.github.com}"

resolve_token() {
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    printf '%s' "$GITHUB_TOKEN"
    return
  fi

  if [ -n "${GH_TOKEN:-}" ]; then
    printf '%s' "$GH_TOKEN"
    return
  fi

  local creds
  creds="$(printf 'protocol=https\nhost=github.com\n\n' | git credential fill 2>/dev/null || true)"
  printf '%s' "$creds" | awk -F= '/^password=/{print $2; exit}'
}

TOKEN="$(resolve_token)"
if [ -z "$TOKEN" ]; then
  echo "No GitHub token found. Set GITHUB_TOKEN or configure git credentials."
  exit 1
fi

github_api() {
  local method="$1"
  local path="$2"
  local data="${3:-}"
  if [ -n "$data" ]; then
    curl -sS -X "$method" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Accept: application/vnd.github+json" \
      "$API_URL$path" \
      -d "$data"
  else
    curl -sS -X "$method" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Accept: application/vnd.github+json" \
      "$API_URL$path"
  fi
}

ensure_label() {
  local name="$1"
  local color="$2"
  local description="$3"
  local payload
  payload="$(jq -nc --arg name "$name" --arg color "$color" --arg description "$description" \
    '{name:$name,color:$color,description:$description}')"
  local resp
  resp="$(github_api POST "/repos/$REPO/labels" "$payload")"
  local created
  created="$(printf '%s' "$resp" | jq -r '.name // empty')"
  if [ -n "$created" ]; then
    echo "Label created: $created"
    return
  fi
  local message
  message="$(printf '%s' "$resp" | jq -r '.message // empty')"
  if [ "$message" = "Validation Failed" ]; then
    echo "Label exists: $name"
    return
  fi
  if [ -n "$message" ]; then
    echo "Label error ($name): $message"
  fi
}

get_existing_titles() {
  github_api GET "/repos/$REPO/issues?state=all&per_page=100" | jq -r '.[].title'
}

EXISTING_TITLES="$(get_existing_titles)"

create_issue() {
  local title="$1"
  local labels_csv="$2"
  local body
  body="$(cat)"

  if printf '%s\n' "$EXISTING_TITLES" | grep -Fxq "$title"; then
    echo "Skip (already exists): $title"
    return
  fi

  local labels_json
  labels_json="$(printf '%s' "$labels_csv" | jq -R 'split(",") | map(gsub("^\\s+|\\s+$";"")) | map(select(length > 0))')"

  local payload
  payload="$(jq -nc --arg title "$title" --arg body "$body" --argjson labels "$labels_json" \
    '{title:$title,body:$body,labels:$labels}')"

  local resp
  resp="$(github_api POST "/repos/$REPO/issues" "$payload")"

  local issue_url issue_number
  issue_url="$(printf '%s' "$resp" | jq -r '.html_url // empty')"
  issue_number="$(printf '%s' "$resp" | jq -r '.number // empty')"
  if [ -n "$issue_url" ] && [ -n "$issue_number" ]; then
    echo "Created #$issue_number: $issue_url"
    return
  fi

  local message
  message="$(printf '%s' "$resp" | jq -r '.message // empty')"
  echo "Create issue failed ($title): ${message:-unknown error}"
}

ensure_label "good first issue" "7057ff" "Good for first-time contributors"
ensure_label "help wanted" "008672" "Extra attention is needed"

create_issue "good first issue: Add Makefile targets for local test and docs workflows" "good first issue, enhancement" <<'BODY'
## Summary
Add a `Makefile` with high-signal targets for common contributor workflows.

## Why this is needed
Current commands are split across README, CI workflow, and docs compose flow. A thin wrapper reduces onboarding friction and command drift.

## Proposed Targets
- `make docs-build` -> build EN/ID/JA docs.
- `make docs-serve` -> run EN docs preview.
- `make docs-serve-id` -> run ID docs preview.
- `make docs-serve-ja` -> run JA docs preview.
- `make test` -> `go test ./...`
- `make vet` -> `go vet ./...`
- `make docs-compose-up` -> `docker compose -f docker-compose.docs.yml up --build -d docs`

## Acceptance Criteria
- `Makefile` exists at repo root.
- Commands run successfully on a clean clone.
- `README.md` has a short "Useful make targets" section.

## Pointers
- `README.md`
- `.github/workflows/ci.yml`
- `docker-compose.docs.yml`
- `mkdocs.yml`, `mkdocs.id.yml`, `mkdocs.ja.yml`
BODY

create_issue "good first issue: Add provider capability matrix page (EN/ID/JA)" "good first issue, documentation" <<'BODY'
## Summary
Add a dedicated docs page that compares provider capabilities for STT, TTS, LLM, and transport.

## Why this is needed
Provider docs are descriptive but not comparative. New users need one view to choose tradeoffs quickly.

## Scope
- New page under docs for EN/ID/JA.
- Columns: streaming support, language/voice override support, retry/circuit-breaker behavior, local testing path, known caveats.
- Link this page from `start-here` and `providers`.

## Acceptance Criteria
- A visible "Provider Capability Matrix" page exists in nav.
- Table is present in all 3 locales.
- Existing docs links include this page.

## Pointers
- `docs/en/providers.md`
- `docs/id/providers.md`
- `docs/ja/providers.md`
BODY

create_issue "good first issue: Add Coolify docs deployment quickstart page (EN/ID/JA)" "good first issue, documentation" <<'BODY'
## Summary
Document a minimal end-to-end Coolify setup using `docker-compose.docs.yml`.

## Why this is needed
Users regularly fail setup on compose path and host-port binding assumptions in Coolify UI.

## Scope
- Add a new quickstart page with field-by-field mapping for Coolify UI.
- Include common failure modes and fixes:
  - compose file path not loaded,
  - fixed port collision,
  - stale deploy cache.

## Acceptance Criteria
- New page in EN/ID/JA docs nav.
- README links to this page.
- Includes troubleshooting checklist.

## Pointers
- `docker-compose.docs.yml`
- `README.md`
BODY

create_issue "help wanted: Harden Twilio signature verification docs and proxy URL tests" "help wanted, enhancement, documentation" <<'BODY'
## Summary
Improve reliability guidance and test coverage for existing Twilio signature verification.

## Why this is needed
Signature verification already exists in transport handlers, but reverse-proxy/public URL mismatch can cause false negatives in production.

## Scope
- Add tests for `requestURL` reconstruction with:
  - `public_url` configured
  - `X-Forwarded-Proto` set
  - host/port variations
- Add docs section for signature verification behavior and deployment checklist.

## Acceptance Criteria
- New tests cover URL variants affecting signature verification.
- Docs include practical checklist to avoid false negatives.
- Existing Twilio transport tests stay green.

## Pointers
- `pkg/transports/twilio/transport.go`
- `pkg/transports/twilio/transport_test.go`
BODY

create_issue "help wanted: Add OpenTelemetry exporter for pipeline spans and metrics" "help wanted, enhancement" <<'BODY'
## Summary
Add OpenTelemetry instrumentation for pipeline processing stages and request lifecycle.

## Why this is needed
Current observability is internal (timeline/cost/latency observers) and lacks OTel export integration.

## Scope
- Add spans for key pipeline stages.
- Add stage-level latency metrics.
- Add minimal exporter configuration documentation.

## Acceptance Criteria
- OTel instrumentation can be enabled via config.
- Measurable spans and metrics are emitted.
- Documentation includes setup and sample dashboard queries.

## Pointers
- `pkg/ranya/engine.go`
- observability modules and docs
BODY

create_issue "help wanted: Add integration tests for router and language metadata flow" "help wanted, enhancement" <<'BODY'
## Summary
Create integration tests that validate routing decisions and language switching under realistic frame sequences.

## Why this is needed
Routing + language are core paths and currently under-covered for integration-level metadata propagation tests.

## Scope
- Add test fixtures for multi-agent/multi-language scenarios.
- Assert emitted frames include expected metadata and ordering.
- Cover fallback path when language detector confidence is low.

## Acceptance Criteria
- New integration tests run in CI.
- Tests fail on routing regressions and pass on expected flow.
- Docs mention what these tests protect.

## Pointers
- `docs/en/task-3-routing.md`
- routing and language logic under `pkg/ranya`
BODY

create_issue "help wanted: Add preflight doctor command for config and env validation" "help wanted, enhancement" <<'BODY'
## Summary
Introduce a preflight command (`doctor` or equivalent) to validate config and env before runtime.

## Why
Startup failures due to missing env values are avoidable and expensive in production.

## Scope
- Validate config schema and required fields.
- Validate required env vars for selected providers.
- Return machine-readable non-zero exit codes on failure.

## Acceptance Criteria
- Command can run against example configs.
- Errors point to exact missing/invalid keys.
- README includes a short preflight usage snippet.

## Pointers
- `examples/hvac/config.yaml`
- config loader paths in `pkg/ranya`
BODY
