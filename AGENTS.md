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
  `X-Kitsune-Request: 1`. Handlers read the caller with `currentUser`. A route with `admin: true`
  also runs behind `requireAdmin`.
- Problem `type`s come from the status (`urn:kitsune:problem:<kind>`); write problems only
  through `writeProblem`.
- The whole API is served under `/api/v1` (`servers` in the contract; the paths in it and in the
  route table don't carry the prefix). `internal/server` strips it after the base path.
- Browse lists page by keyset (`GET /albums`: `sort` `added` or `name`, opaque `cursor`), never
  `OFFSET`. The two sorts are fixed sqlc queries, one per index, with identical SELECT lists; they are
  not built dynamically, which supersedes the dynamic-sorts rule in the Store section for these two.
  This also supersedes the spec's `next_cursor` and limit 500: the API says `nextCursor` and caps
  `limit` at 200.
- Tag edits are one route, `POST /albums/{id}/edits`, and `GET /albums/{id}/tags` reads what an editor
  shows, from the files. The validator is the album's `etag` field in the body, a hash of
  `ulid:size:mtime_ms` per track, and not a response header. Clients send it back verbatim as
  `If-Match`; comparison is strong and happens under the library lock.
- A canceled query fails with `sqlite3: interrupted`, not `context.Canceled`: check
  `r.Context().Err()` before logging an error from a handler a client can abandon.
- Responses carry `api` types built field by field (`toAPIUser`); never marshal a
  `store` type, so a new store column can't leak.
- Nothing the server writes (bodies, headers, logs) contains a password or token, except
  login's response and `Set-Cookie`.

## Store
- `internal/store` is the only package that speaks SQL. Writes go through the
  single-connection writer (`_txlock=immediate`), reads through the `query_only` pool.
- The schema is the numbered files in `internal/store/migrations/`, listed with their sha256 in
  `manifest.txt`; a test checks one against the other. `internal/store/schema.snapshot` is the
  resulting schema, generated by `mise run generate`: read it to see the schema, and review its diff
  to see what a migration did. Tables are `STRICT`; times are Unix ms UTC.
- Only `serve` migrates (`Options.Migrate`), at startup, one transaction per file with foreign keys
  off; `users add` refuses a database with migrations pending, since a server may be running on it.
  A failed step stops startup and names the file: that step is undone and earlier ones stay.
- Before migrating a database with data, `serve` writes `kitsune.db.v<from>.<UTC time>.bak` beside it,
  mode 0600. It wants about 3x the database free, and a failed backup stops the upgrade with nothing
  changed. After a success, the newest backup of each of the last 3 from-versions stays. To restore:
  stop the server, delete `kitsune.db-wal` and `kitsune.db-shm`, move the backup over `kitsune.db`,
  and run a binary at that version. A stale `-wal` beside a restored file reads as
  `database disk image is malformed`.
- To change the schema, add `NNNN_name.sql` with the next number and its `manifest.txt` line
  (`sha256sum`), then run `mise run generate` and review the snapshot's diff. The rules:
  - Never edit a shipped file. No transaction control or `PRAGMA` in a file: the runner owns both,
    and a test rejects them.
  - Rebuild a table in this order: create new, copy, drop old, rename new. List columns explicitly,
    including `id`, and never reorder rows: ids are what tags and FTS rows hang off, and
    `foreign_key_check` can't see them move. Recreate indexes and triggers. Renaming the old table
    first repoints its children at a name about to vanish.
  - `ADD COLUMN` can't take `NOT NULL` without a default, a non-constant default, `UNIQUE`, or a
    `REFERENCES` with a non-NULL default: those need a rebuild.
  - Keep FTS rowids. No `UPDATE ... FROM`: sqlc can't read it; use a subquery. Values derived in Go
    (FTS text, ULIDs) can't come from a migration: they need a design of their own, in the plan that
    needs them. State any backfilled time in the file: `Options.Now` doesn't reach SQL.
  - Rows `seedLibrary` writes must stay valid at every version; `TestPrefixes` starts from each one.
- Binaries from before migrations call a migrated database stale and say to delete it. Don't. Point
  old worktrees at their own data dirs.
- Fixed queries live in `internal/store/queries/*.sql` and are generated into
  `internal/store/sqlcdb` (committed, never edited) by `mise run generate`. That builds
  sqlc cgo-free from `tools/go.mod`; never use the release binary, which links C.
- Dynamic queries (browse filters; the album list's two sorts are fixed queries, see API) are built in Go over allow-listed columns,
  with bound parameters only. Never interpolate request input into SQL.
- Albums have a public ULID and a display `artist`. Their `sort_name` column is `COLLATE NOCASE`:
  the collation must be on the column, because one on a row value never seeks. `LiveTrackFacts` uses
  `INDEXED BY tracks_live`, since a partial index's WHERE column doesn't count as covered; a test
  fails if the index is renamed.
- Artists and albums are found by an in-memory name index (`names.go`), not by FTS, which can't
  take a typo or say which artist matched. A search reads the index without waiting; `RefreshNames`
  builds a new one and swaps it in. `Open` and the end of every scan call it, and so must anything
  else that adds, removes or renames an artist or album. Various Artists stays out of it.
- Artists have a public ULID too, assigned by the scanner and kept across updates. Their row `id` is
  internal: it is reused after a delete.
- An album belongs to its album artists; with none, to the first artist of its tracks, which grouping
  makes the same on all of them, unless it is a compilation. `queries/artists.sql` spells this out,
  and every query about owners repeats it. Various Artists is a real row that no reader shows:
  `isVarious` decides, by match key or MBID.
- Library writes go through `WriteBatch`, one transaction per batch. The store never
  normalizes names or decides identity: callers pass match keys, group keys and ULIDs.
- FTS rows are written in the same transaction as their track and through `foldSearch`;
  a search query must fold the same way.
- Nothing reaches `MATCH` except a `SearchQuery`, and `ParseSearch` is the only way to make one. Its
  caps are what stop a long query taking down a read connection; never hand-build an FTS expression.
  The `tracks_fts` table is contentless: it can rank, but can't say which column matched.

## Auth
- `internal/auth` owns credentials. Passwords are Argon2id in PHC format; the store only
  ever sees the encoded hash. Session tokens (`kss_…`) are shown to the client once and
  stored as SHA-256 hashes. No password, token or hash appears in an error, log or response.
- Tests hash with cheap parameters (`auth.Params{Time: 1, MemoryKiB: 64, Threads: 1}`);
  only benchmarks use `auth.DefaultParams()`.
- Changing `DefaultParams` is safe: hashes carry their parameters, and logins rehash.

## Library
- `internal/library` turns audiometa files into store writes. Identity keys always go
  through `MatchKey` (trim, collapse spaces, NFC, case fold); no punctuation stripping,
  no splitting of names.
- Raw tag names live only in `keys.go`'s candidate tables, verified per format against
  real files written by audiometa, in `writekeys.go`'s field table, which maps each field to its
  keys per format, its mirrors and its dependents, and in `rawtags.go`'s allow rule for raw edits.
  Add a key there, never inline. A key the table owns in any format can't be edited raw: a scan reads
  candidates in every format, so a raw tag could otherwise move a track to another album.
- audiometa reports tags as stored; display fallbacks (title from file name, sort name
  from name, disc folders) are Kitsune's and live here.
- The scanner is the only library writer: one goroutine resolves identity and writes. Tag edits go
  through it too (`Scanner.EditAlbum`), holding the library lock.
  Identity order: row at the path, then a single digest match among missing tracks,
  then same relative path, then base name, then a new ULID.
- An outage must never look like deletions, because a missing track is eventually
  deleted with its users' data. An unreadable or empty root, a mass disappearance, an
  unreadable directory and an empty `roots` all leave tracks alone. Removed roots count
  only when every configured root was walked and had audio. Deletes hold while a missing
  track may be behind an outage.
- Genres are derived, not stored: `genres.go` splits and groups the raw tags into `library.Genres`, in
  memory, refreshed at start and after every scan and edit. `genreSeparators` alone decides what
  divides a value, so a rule change needs a restart and no migration. Synonyms stay apart: the tag
  editor fixes those. Lists and album pages carry the result as `displayGenres`; an album's raw
  `genres` stay as tagged, for the editor.
- Unreadable files are retried every scan (`retrySize`); corrupted and unsupported ones
  wait for the file to change.
- One library lock covers a whole scan, walk included, and a whole edit. An edit that meets it
  gets `ErrBusy`; a scan that meets an edit waits. Readers never take it.
- `Scanner.Claim` reserves the one scan without waiting, so a refusal is known before anything starts;
  `Scan` is a claim that runs at once. A scan a handler starts runs as the app's background work
  (`app.startScan`), so shutdown cancels it and waits. A scheduled scan that meets a running one retries
  once, when that one ends. `POST /admin/library/scan` answers 202 or 409 from the claim.
- `Scanner.Progress` is an immutable snapshot swapped atomically, which only the claim's holder
  changes. `GET /library` carries it, and the reason a scan failed goes to administrators only.
- No library root reaches a client. `GET /admin/library/issues` sends paths relative to the root and
  passes messages through `StripRoot`; `GET /library` gives everyone counts by kind.
- Scans remove audiometa's temp files (`isWriteTemp`), and only with `library.tag_writes` on: off,
  nothing under a root ever changes.

## Artwork
- `internal/artwork` renders covers during the scan and knows nothing of the store or the
  scanner. Renditions are JPEGs at `<data_dir>/artwork/<hh>/<hash>-<size>.jpg`, named by the
  SHA-256 of the source image, so a request only ever reads a file and a file never changes.
  The ladder is 128/512/1024 on the longest side and never upscales; `Path` returns the largest
  rung at or below the request. A zero-size file counts as missing.
- The scanner finds one cover per album per scan (`covers` in `internal/library/cover.go`): the cover
  of the album's first track, by disc, track number and path, that has one. A track looks unless one
  before it has found a cover, and the writer drops a cover that arrives after a better one, so
  worker order never decides. Art never fails
  a file: bad images become warnings, cache failures are logged. Folder art is read through
  `os.Root`, never through `fs.FS` (it rejects non-UTF-8 names).
- A cover's accent and ground colors come from the encoded 128 rendition, so a fresh render and
  an idempotent `Put` agree.

## Operations
- `cmd/kitsune-server` is the only place that reads the environment, signals or flags;
  `internal/server` takes everything through `server.Config`.
- Logs are `log/slog` JSON. Log `r.URL.Path`, never the full URL or query: OpenSubsonic
  clients send credentials in the query string.
- `Config.Ready` backs `/readyz`; new readiness conditions join it there (today: the first scan has finished, then the database answers).

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
