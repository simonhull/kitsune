# Working on Kitsune

Kitsune is a self-hosted music server (a Navidrome replacement) with a native API,
an OpenSubsonic compatibility layer, a web UI and a terminal client. Tags are read
and written through [audiometa](https://github.com/simonhull/audiometa).

## Toolchain
- `mise install` installs the pinned Go and golangci-lint (`mise.toml`).
- `mise run check` is the full local gate, the same as CI: build (`CGO_ENABLED=0`),
  race tests, lint (`go mod tidy -diff`, golangci-lint with formatters, `go vet`,
  `go fix -diff`), the no-C-interop check and `govulncheck`.
- **Pure Go, no C interop.** No cgo in Kitsune or in any dependency, test-only ones
  included: the binary is static (`CGO_ENABLED=0`) for scratch images and plain
  `GOARCH` cross-builds. `scripts/nocgo.sh` (`mise run nocgo`, in `check` and CI) fails
  if any non-standard package in the build graph, Kitsune's own included, has cgo, C
  sources or `.syso` objects. Linters can't do this: Go silently skips cgo files when
  `CGO_ENABLED=0`. Pure-Go assembly is fine. The race detector is toolchain, not a
  dependency, and stays.
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

## API
- `api/openapi.yaml` is the contract; there is no code generator. Handlers live in
  `internal/server`, wire types in `api`, and clients are written by hand.
- Every route is one entry in `internal/server`'s route table; its pattern is the
  spec's method and path verbatim. `TestRoutesMatchContract` enforces this.
- API tests go through `openapitest` (strict request/response validation). A full
  test run fails if any declared operation × status is never exercised.
- Errors are RFC 9457 problems via `writeProblem`; never `http.Error`.
- Non-public routes run behind `authenticate`: `Authorization: Bearer` wins over the
  `kitsune_session` cookie; cookie-authenticated unsafe methods must send
  `X-Kitsune-Request: 1`. Handlers read the caller with `currentUser`.
- Problem `type`s come from the status (`urn:kitsune:problem:<kind>`); write problems only
  through `writeProblem`.
- Responses carry `api` types built field by field (`toAPIUser`); never marshal a
  `store` type, so a new store column can't leak.
- Nothing the server writes (bodies, headers, logs) contains a password or token, except
  login's response and `Set-Cookie`.

## Store
- `internal/store` is the only package that speaks SQL. Writes go through the
  single-connection writer (`_txlock=immediate`), reads through the `query_only` pool.
- Until the first release the schema is one file, `internal/store/schema.sql`, edited in
  place. Editing it changes the schema hash, and an existing development database is then
  refused: delete it. Tables are `STRICT`; times are Unix ms UTC.
- Fixed queries live in `internal/store/queries/*.sql` and are generated into
  `internal/store/sqlcdb` (committed, never edited) by `mise run generate`. That builds
  sqlc cgo-free from `tools/go.mod`; never use the release binary, which links C.
- Dynamic queries (browse filters and sorts) are built in Go over allow-listed columns,
  with bound parameters only. Never interpolate request input into SQL.

## Auth
- `internal/auth` owns credentials. Passwords are Argon2id in PHC format; the store only
  ever sees the encoded hash. Session tokens (`kss_…`) are shown to the client once and
  stored as SHA-256 hashes. No password, token or hash appears in an error, log or response.
- Tests hash with cheap parameters (`auth.Params{Time: 1, MemoryKiB: 64, Threads: 1}`);
  only benchmarks use `auth.DefaultParams()`.
- Changing `DefaultParams` is safe: hashes carry their parameters, and logins rehash.

## Operations
- `cmd/kitsune-server` is the only place that reads the environment, signals or flags;
  `internal/server` takes everything through `server.Config`.
- Logs are `log/slog` JSON. Log `r.URL.Path`, never the full URL or query: OpenSubsonic
  clients send credentials in the query string.
- `Config.Ready` backs `/readyz`; new readiness conditions (the scanner) join it there.

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
