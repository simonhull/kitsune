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
- `GET /library/index` is the one browse list that doesn't page: the client needs every row's height
  to place the scrollbar and the rail. Its order lives in `orderIndex`, and nowhere else. `fields`
  adds what the song columns show, each read by its own query only when asked for (`IndexOptions`)
  and sent through the nil-able `TrackFields`, so an index without it costs what it did;
  `TestLibraryIndexWithoutFieldsIsUnchanged` holds its bytes. `albums` (what a `library` event names)
  gives just those albums in the same shape, each with `after`, the album before it in the whole order,
  which is how a client patches its copy in place; their queries are the `…Of` twins of the whole
  ones, joined from the albums out. What the whole index reads of the library (albums, tracks, their
  artists, details and tags), most of its cost, is kept between library writes (`libraryrows.go`);
  marks, plays and shuffle rules are read every time. A write that changes what the index shows goes
  through `inLibraryTx`, an edit's renames, merges, years and covers included.
- `GET /library/index?artist=` is the artist page's list: the songs the `artist` shuffle scope draws
  from, in the index's shape under one heading, the artist's own albums first. Its queries are the
  `…ByArtist` twins, and it never reads or fills what `libraryrows.go` keeps. `ArtistDetail.songCount`
  counts the same songs.
- Tag edits are one route, `POST /albums/{id}/edits`, and `GET /albums/{id}/tags` reads what an editor
  shows, from the files. The validator is the album's `etag` field in the body, a hash of
  `ulid:size:mtime_ms` per track, and not a response header. Clients send it back verbatim as
  `If-Match`; comparison is strong and happens under the library lock.
- A canceled query fails with `sqlite3: interrupted`, not `context.Canceled`: check
  `r.Context().Err()` before logging an error from a handler a client can abandon.
- Responses carry `api` types built field by field (`toAPIUser`); never marshal a
  `store` type, so a new store column can't leak.
- A person's preferences are `GET`/`PUT /me/prefs/{key}`. `internal/prefs` alone says which keys
  there are and what each holds, and checks a value on the way in and again on the way out; the
  store keeps the text. A new key is an entry there and its schema in the contract. Server settings
  never go there.
- `ratings.heart_stars` is what a hearted song with no stars of its own counts as wherever stars are
  read (0 for nothing, else 1★ to 5★ in halves). It is worked out where it's read
  (`prefs.HeartStars.Of`, `marks.counted`), and never written or sent as a song's `stars`.
- Every song a client starts is a listen (`POST /me/listens`), whether or not it counts as a play:
  its start and its end share a client ID and are one `listens` row, and once ended it never
  changes. Only `skipped`, moving on while it sounds, counts against a song; pausing, a restart,
  the page closing and an error never do. `plays` and Last.fm stay as they were.
- `GET /events` is every client's one stream of changes (server-sent events). `internal/events` numbers
  them, keeps 5 minutes or 10,000 for `Last-Event-ID`, and filters each by its `Audience`; past that a
  client gets `resync`. Handlers publish only after a change succeeded (`s.publish`), and the scanner
  through its `library.Notifier` (`server.LibraryEvents`). An event carries ids and keys, never a value
  a route wouldn't send: a new kind gets a schema in the contract, a case in `events_test.go`, and a
  place in `TestNoEventHoldsASecret`. The handler sets its own write deadline, since the server has none.
- Nothing the server writes (bodies, headers, logs) contains a password or token, except
  login's and reset confirmation's responses and `Set-Cookie`, and the setup code's and each reset
  code's one log line (see Operations).
- `GET /setup` and `POST /setup` are public. They make the first account, an administrator, while
  there is none; once one exists (`users add` included) `needed` is false for good and `POST` is 409.
  "Your network" is `isLocal` (loopback, private, link-local, `100.64.0.0/10`) on `clientIP`; from
  outside it, `POST` needs the setup code, compared in constant time, and wrong codes count against
  the login limiter by IP. The store inserts the first user only while `users` is empty, in one
  statement, so a race gives one 200 and one 409.

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
  `WriteBatch` and `MarkMissing` note the albums they touch, a moved track's old one included, once
  committed; the scanner takes them (`TakeChangedAlbums`) to say what changed.
- FTS rows are written in the same transaction as their track and through `foldSearch`;
  a search query must fold the same way.
- Nothing reaches `MATCH` except a `SearchQuery`, and `ParseSearch` is the only way to make one. Its
  caps are what stop a long query taking down a read connection; never hand-build an FTS expression.
  The `tracks_fts` table is contentless: it can rank, but can't say which column matched.

## Matching
- `internal/matching` finds an album's release at a source (MusicBrainz now; Discogs later, as another
  `Source`) and says what its tags would become. It proposes and never writes: the client applies what
  a person ticks through `POST /albums/{id}/edits`, so everything editing guarantees holds. It knows
  nothing of the store; `server.LookupCache` keeps its answers in `lookups` for a day.
- One `MusicBrainz` client serves the process (built in `cmd/`): its gate keeps every request to one a
  second and serves people before prefetch (`matching.Background`). It names itself
  `Kitsune/<version> ( https://github.com/simonhull/kitsune )` and asks again after a 503.
- Confidence is evidence-weighted, and evidence is structured facts the client words, as shuffle's
  `why`. Matching never proposes taking a value away, and never guesses: a field the release lacks is
  left out.
- MusicBrainz IDs are fields of the table (`musicbrainz…Id`); a match always writes them with the songs
  it touches, artists' only where they line up with the song's artists.
- Tests use `internal/matching/musicbrainztest`; nothing ever talks to the real MusicBrainz.

## Settings
- Server settings live in the database's `settings` table, one JSON value per key with its source.
  `internal/settings` alone says which keys there are, what each holds (`defs`) and when a change
  applies; the store keeps the text. `listen` and `data_dir` are not settings: the server needs them
  before it opens the database, so `internal/config` reads them from the file at every start.
- server.toml, `KITSUNE_<KEY>` variables and `serve -set KEY=VALUE` only seed a key the database has
  no value for (flag over env over file), and only `cmd/` seeds (`seedsFor`, `settings.Apply`), at
  startup. A seed that differs from the database's value logs one warning naming the key and source.
  Clearing a key in the browser, or `kitsune-server settings reset KEY`, deletes its row, so the next
  start seeds it again.
- A `settings.Service` holds the live values as an immutable `Snapshot`, swapped atomically after a
  `Change` stored every value in one transaction. Owners subscribe to their keys and apply a change
  in `cmd/kitsune-server/follow.go`: the scanner through `Scanner.Configure`, which waits for the
  library lock, so a scan or an edit never sees roots, grace or tag writes change midway; the
  scrobbler through `SetClient`; the schedule through `SetDaily`; the log level through a
  `slog.LevelVar`. `base_url` and `trusted_proxies` apply only at a restart (`restartRequired`).
  `server.Config` keeps startup values; anything live is read from its owner.
- `lastfm.shared_secret` is a secret: stored as written, because the server signs with it, and never
  logged, returned (`GET /admin/settings` says only `set`) or put in an error. Never quote a value
  in a `KeyError`. A secret can't be a `-set` flag, which the process list shows.
- `GET /admin/folders` is the folder picker's browser of what the server sees: directories only, read
  through `os.Root` from `Config.FolderRoot` (`/`), never a file name or size. A symlink is listed as
  the folder it names, and an absolute target is read under the root, since `os.Root` refuses every
  absolute link. The audio count is bounded by time and folders. Tests point the root at a temp dir.

## Auth
- `internal/auth` owns credentials. Passwords are Argon2id in PHC format; the store only
  ever sees the encoded hash. Session tokens (`kss_…`) are shown to the client once and
  stored as SHA-256 hashes. No password, token or hash appears in an error, log or response.
- Tests hash with cheap parameters (`auth.Params{Time: 1, MemoryKiB: 64, Threads: 1}`);
  only benchmarks use `auth.DefaultParams()`.
- Changing `DefaultParams` is safe: hashes carry their parameters, and logins rehash.
- A new password goes through `Service.SetPassword`, which ends sessions in the same write as the hash,
  as its `SignOut` says: `POST /me/password` keeps the caller's session and ends the others (the
  current password is checked, and wrong ones count against the login limiter); a reset and
  `users passwd` end them all (`-keep-sessions` keeps them); an administrator's `POST
  /admin/users/{id}/password` ends them all and refuses the caller's own account.
- `kitsune-server users passwd NAME` sets a password from the command line, while a server runs
  on the database, and `users list` prints names and roles. Like `users add`, both refuse a database
  with migrations pending and never create one. In the image: `docker exec -it kitsune
  /kitsune-server users passwd NAME`.

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
- The watcher (`watch.go`) follows the roots and only marks folders dirty; once they settle (10 s),
  `Scanner.ScanDirs` reads just those folders under the claim and the lock, through the scan's read
  path. A targeted scan marks a track missing only when its folder, or the nearest read folder above
  it, was read and the file is gone. It never deletes or prunes, and leaves a root that looks down
  (missing, empty, losing most of its tracks) to the full-scan rules. A running scan that walked after
  the last change absorbs it.
- A root the OS can't report on is polled every 5 minutes instead (`poll.go`: folder mtimes and audio
  sizes and mtimes): one on a network or FUSE filesystem, by `/proc/self/mountinfo` (`networkFS` is
  the one list of types), or one past the watch limit (ENOSPC on add). `GET /admin/settings` says
  each folder's mode and why.
- Unreadable files are retried every scan (`retrySize`); corrupted and unsupported ones
  wait for the file to change, or for another tag reader: each such issue keeps the audiometa
  version that judged it (`scan_issues.reader`, `Options.Reader`), and a scan reads the file again
  once its own differs.
- WAV and AIFF keep tags in an ID3v2 chunk and in their own text (LIST/INFO; NAME, AUTH, ANNO).
  A field's `native` keys are mirrors of its ID3 keys, read after them and kept in step where they
  exist; audiometa mirrors the typed fields itself. The ID3 version, which decides the family, comes
  from the chunk (`chunks.go`).
- One library lock covers a whole scan, walk included, and a whole edit. An edit that meets it
  gets `ErrBusy`; a scan that meets an edit waits. Readers never take it. The fingerprint pass holds
  it for one batch at a time (32 files or about 2 s), so an edit or a scan waits out a batch, and
  the pass lets them go first and pauses while a scan is claimed.
- Fingerprints (`audio_digest`) are only for following a moved file, and may be missing. A scan
  doesn't hash, which would read all the audio, unless a missing track has a fingerprint: then a
  file with no track at its path is hashed inline, so it can be matched. A file at a known path never
  is; a write without a fingerprint keeps the stored one while size and mtime are unchanged, and
  clears it otherwise. `Scanner.RunFingerprints` fills the rest in the background, one reader in path
  order, at start and after every scan and edit, as the app's background work. A file that moves
  before the pass reaches it gets a new ID: the known limit. `GET /library` carries the pass's
  progress (`Scanner.Fingerprints`), a snapshot swapped like `Progress`; readiness never waits for it.
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
- Loudness is measured where ReplayGain is missing (`loudness.go`, `Scanner.RunLoudness`): every file
  of an album with a song that has no gain tagged, then the album from its files' gated blocks pooled
  (`loudness.Pool`), never their mean. `internal/loudness` is the pure BS.1770-4 / R 128 meter, held
  to EBU Tech 3341/3342 in its tests; `internal/pcm` decodes FLAC, MP3 and uncompressed WAV and AIFF,
  and nothing else is measured yet. A measure is Kitsune's own (`track_loudness`, `album_loudness`),
  kept per file by size, mtime and `loudnessVersion`, which a change to either package's numbers
  raises; it is never written to a file. The pass runs after the fingerprint pass, one file at a time
  in path order, a batch under the library lock, and stops midway through a file for a waiting scan
  or edit. A track's `replayGain` is its tags' when it has a gain tagged, else the measure, and says
  which (`source`).

## Shuffle
- `internal/shuffle` is the pure engine: units, weights, a finite order and an endless stream, given
  plain values. It knows nothing of the store. Its bench (`internal/shuffle/bench`, `BASELINE.md`) is
  how a change to it is judged: hard rules fail the report, and a change lands only when no metric
  is worse and one is better.
- `internal/shuffler` feeds it: it builds a pool from the store (the scope's songs, the person's
  marks, Combine and Exclude, and their listening), and keeps each session's state
  (`shuffle_sessions`, opaque to the store) so `GET /me/shuffles/{id}/next` continues it on any
  device. A run holding a song in scope plays whole, even into another album. A person's history is
  kept in memory and dropped by their own listens and plays (`Service.Forget`). Flow's similarity is
  genre overlap and co-listening from listens outside shuffle only, so flow never learns from itself;
  with nothing known it is off.
- A pick's `why` is structured factors, never prose: the client words them.
- The `rated` scope and the weights take counted stars: a song's own, else `ratings.heart_stars` for
  an unrated heart (`prefs.HeartStars.Of`, in `keeper` and `build` both). The shuffler reads the pref
  itself; the engine only sees stars, with `ByHeart` so `why.heartStars` can say so. The heart's own
  factor still applies on top.
- The web client starts every shuffle without waiting: it picks the first song from what it holds
  and plays it, and asks `POST /me/shuffles` with it as `first` at the same time.

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
- `mise run release` builds `dist/kitsune-server` with the web client inside it. `internal/webui` embeds
  `dist`, which holds only `.gitkeep` in git: a plain `go build` gives the API alone, so CI and tests need
  no client built. `scripts/build.sh` fills it for the compile and empties it after.
- `internal/server/web.go` serves the client for what is outside the API. It is not a route, so it stays
  out of the contract. Anything under `/api`, any write, any missing file and the bare `/healthz` and
  `/readyz` stay 404 problems, so a health check missing `/api/v1` fails instead of passing on a page:
  only a path that could be a client route gets `index.html`. A client route must have no dot in its last
  segment. A base path serves no client, because the build's paths are fixed to the root of a host.
- The image is `FROM scratch`: no shell, no zone files (`time/tzdata` is in the binary), no `/tmp`. Temp
  files go beside their targets, and must keep doing so. SQLite keeps its temp storage in memory (`temp_store`) for the same reason: a big sort or table rebuild would otherwise fail with `IOERR_GETTEMPPATH`. There are no CA certificates either: `cmd/kitsune-server/roots.go` bundles Mozilla's (`x509roots/fallback`, used only when the system has none), or every HTTPS call, Last.fm's included, fails. CI runs `scripts/smoke-image.sh` against it.
- `cmd/kitsune-server` is the only place that reads the environment, signals or flags;
  `internal/server` takes everything through `server.Config`.
- Logs are `log/slog` JSON. Log `r.URL.Path`, never the full URL or query: OpenSubsonic
  clients send credentials in the query string.
- `serve` starts with no accounts. It then makes a setup code (`crypto/rand`, 80 bits) and logs it
  once, in the startup WARN line: the one secret ever written to a log, on purpose, since whoever
  reads the log runs the server. The code lives in memory only (`server.Config.SetupCode`) and never
  appears in a response.
- A forgotten password is reset with a code from the log, since Kitsune sends no email.
  `POST /auth/reset` answers 202 whether or not the name exists; for a real one `auth` makes a code
  (`auth.NewCode`, the setup code's shape) and the server logs it once, in a WARN line naming the
  user: the only other secret ever written to a log, on purpose. Codes live in `auth.Service`'s
  memory, one per person (a new request replaces the old), for `auth.ResetCodeTTL` (15 minutes), and
  a restart voids them. Requests are limited per IP and per name on their own limiter, so asking
  never locks anyone out. `POST /auth/reset/confirm` compares the normalized code in constant time;
  wrong codes count against the login limiter, and a wrong, expired or used code and an unknown name
  are one 403. Success ends every session of that person and signs them in, as login does.
- `KITSUNE_MUSICBRAINZ_URL`, read only in `cmd/`, points the server at a stand-in MusicBrainz (its web
  service at `/ws/2/`, covers at `/coverart/`), as `KITSUNE_LASTFM_URL` does for Last.fm.
- `KITSUNE_LASTFM_URL`, read only in `cmd/`, points the server at a stand-in Last.fm (its API at
  `/2.0/`, its approval page at `/api/auth/`) for manual checks; production never sets it. Tests use
  `internal/lastfm/lastfmtest` and nothing ever talks to the real Last.fm.
- `Config.Ready` backs `/readyz`; new readiness conditions join it there (today: the first scan has finished, then the database answers).

## Web client
- A theme is one set of values for the color tokens in `web/src/lib/styles/tokens.css`, under
  `[data-theme]`. Components read tokens, never raw colors: `rawColors.test.ts` fails on a hex,
  `rgb()` or `hsl()` in a component's style, unless a `raw-color:` comment on or above the line says
  why. Cover colors come from data, through `style:` bindings. The fox is `--fox` as a fill,
  `--fox-text` as words and `--fox-line` as a ring or underline: on light the orange itself fails.
- An album's colors come from `paletteFor(entry, theme.current)`; each theme has its own rules.
- One `EventSource` per signed-in client (`lib/library/live.ts`), opened by the shell and closed at
  sign-out; `Library` applies each event. A `library` event patches the index in place
  (`indexPatch.ts`, from `GET /library/index?albums=`), so the Library page keeps its song on screen;
  a gap the server can't fill (`resync`, or a stream reopened after the browser gave up) reads everything
  again. While events arrive nothing polls for scans. Nothing on the way to playback waits for any of it.
- Song columns are one order for every list (`library.columns`), the number and the title in it:
  they move but never hide, and an old value without them gets them back first. A column dragged
  moves by its own style rule (`columnMotion.ts`, written through the CSSOM: a custom property on the
  list restyled everything under it), one resized by `--col-<id>` on the list; neither changes a row
  or any state until the task that ends it. `reorderIn` keeps which columns fit the same on a drop.
  A list's grid is a rule too (`songGrid.ts`), and the task that ends a move reads no layout back;
  the settle starts after the frame that shows the new order, and the save is sent after it.
- An artist page's queue is its list as shown, and stops at its end: `player.playList` holds the songs
  ahead as track and album IDs (`Queue.later`) and fetches each next album after the song before it
  has begun, so nothing waits on the play path.
- `app.html` sets `data-theme` before the first paint, from `localStorage['kitsune.theme']` or the
  system, by the same rule as `lib/theme`'s `resolveTheme`: change them together. The person's choice
  is the `ui.theme` pref, read after sign-in and forgotten at sign-out.

## Speed
- Every interaction has a p95 budget in `perf/budgets.json`, the one file that holds them. The browser
  suite (`web/perf`, Playwright) times clicks, keys and page loads in a release build against generated
  libraries of 1,000 and 50,000 songs, 20 runs each; `internal/server`'s `*Speed` tests time the
  stream's first byte and `GET /library/index`. Each fails past its budget, and CI's `perf` job runs
  both on every PR, with the table in the job summary. Budgets are about 1.5x what was measured when
  they were set; on CI they are multiplied by `ciMultiplier`, which comes from CI's own numbers.
- `mise run perf` runs everything (`scripts/perf.sh`; Playwright arguments pass through). It writes
  only under `$KITSUNE_PERF_DIR` (default `kitsune-perf` in the temp directory) and reuses the
  libraries there. Each server starts on a fresh database, and nothing is timed until its first scan
  and the fingerprint and loudness passes after it are done: the suite measures a server at rest.
  Only the loud albums lack ReplayGain, so the loudness pass has little to do. `KITSUNE_PERF_CHROMIUM` picks a local Chromium, `KITSUNE_PERF_LIBRARIES=1k` measures
  one size, `KITSUNE_PERF_VERBOSE=1` prints every sample, and `KITSUNE_PERF_PORT` moves the servers off 14610, for a second worktree measuring at once. The speed tests skip under `-race`.
- Nothing new may wait on the play path, from a click to the `playing` event. A change on that path,
  or to a budget, carries the suite's numbers before and after, and no change is called faster without them.

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
