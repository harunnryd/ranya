# Contributing to Ranya

Thanks for taking the time to contribute. We appreciate bug reports, feature ideas, and PRs.

## Code of Conduct
Please read and follow `CODE_OF_CONDUCT.md`.

## Development Setup
- Go version from `go.mod` (currently `1.24.13`).
- Python + `pip` if you want to build the docs.

## Useful Commands
```bash
go test ./...
go vet ./...
```

Format Go code before pushing:
```bash
gofmt -w ./
```

Optional vendor boundary check (especially if you touch providers):
```bash
scripts/verify_vendor_boundary.sh
```

Docs site:
```bash
pip install mkdocs-material
mkdocs serve
```

## Pull Requests
- Keep PRs focused and small when possible.
- Add or update tests where it makes sense.
- Update docs if behavior changes.
- Use Conventional Commits in PR titles and commit messages so release automation can version correctly:
  - `feat:` for new features
  - `fix:` for bug fixes
  - `feat!:` or `BREAKING CHANGE:` footer for breaking changes
- Do not edit `CHANGELOG.md` manually for normal changes; `release-please` updates it during release PRs.

## Reporting Bugs
Please include:
- Reproduction steps
- Expected vs actual behavior
- Logs or stack traces if available
- Config snippet (redact secrets)
