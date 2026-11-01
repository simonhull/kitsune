# Working on Kitsune

Kitsune is a self-hosted music server (a Navidrome replacement) with a native API,
an OpenSubsonic compatibility layer, a web UI and a terminal client. Tags are read
and written through [audiometa](https://github.com/simonhull/audiometa).

## Toolchain
- `mise install` installs the pinned Go and golangci-lint (`mise.toml`).
- `mise run check` is the full local gate, the same as CI: build, race tests, lint
  (`go mod tidy -diff`, golangci-lint with formatters, `go vet`, `go fix -diff`) and
  `govulncheck`.
- `mise run fmt` applies gofumpt and goimports. Always lint through mise: a global
  golangci-lint of another version reports different findings.

## Go style
- References: Effective Go, Go Code Review Comments, the Google Go Style Guide.
- Modern standard library first: `errors.AsType`, `iter.Seq2` for walks and queries,
  `os.Root` for file access under a library root, `sync.WaitGroup.Go`,
  `encoding/json/v2`, `log/slog`, `net/http.ServeMux` patterns, `t.Context()`,
  `testing/synctest` for anything timed.
- Errors: wrap with `fmt.Errorf("...: %w", err)`; sentinels and typed errors for
  conditions callers branch on. `err113` forbids dynamic errors (`errors.New`, or
  `fmt.Errorf` without `%w`) outside package-level sentinels; wrap a sentinel instead.
- Small packages under `internal/`, dependencies pointing one way:
  `api` → (`library`, `stream`, `auth`, `events`) → `store`. Nothing imports `api`.
  Only `internal/store` speaks SQL.
- Only `cmd/` touches `os.Stdout`/`os.Stderr` or prints; everything else takes an
  `io.Writer` or logs through `log/slog` (forbidigo enforces this).
- Accept interfaces where a consumer needs them; return concrete types.
- US spelling in identifiers, API fields and log messages.

## Lint policy
- golangci-lint runs with `default: all`. Every disabled linter in `.golangci.yml` has
  its reason next to it; do not add to that list without one.
- Every `//nolint` names the linter and gives a reason (nolintlint enforces both).
  Fix the code first.
- Tests are linted like production code; only `mnd`, `funlen`, `err113` and gosec G304
  are relaxed in `_test.go`.

## Tests
- Standard `testing` only (depguard bans testify, gomega and ginkgo). Table tests with `t.Parallel()`; `t.Context()` for contexts.
- Real dependencies over mocks: temp SQLite files, `t.TempDir()` libraries,
  `httptest` servers.
- Benchmarks use `for b.Loop()`. No performance claim without a measurement.

## Commits and docs
- Gitmoji prefix, single line, no body, no attribution trailers.
- Plans, specs and design notes never go in this repository.
