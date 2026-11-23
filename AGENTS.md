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
- The Trash is four admin routes: `GET /admin/trash` lists it by album, newest first; `POST
  /admin/trash` takes `tracks` or `albums`, `POST /admin/trash/restore` and `POST /admin/trash/empty`
  (`tracks` or `all`). Trash and restore are all or nothing and run through the scanner under the
  library lock (`ErrBusy` is a 409 with Retry-After, as an edit's); both publish `library` events for
  the albums touched, and emptying publishes nothing. With `library.tag_writes` off every write is a
  409 saying so, and `Me.canTrash` is false, so the client shows no control. A problem names songs,
  never a path.
- A canceled query fails with `sqlite3: interrupted`, not `context.Canceled`: check
  `r.Context().Err()` before logging an error from a handler a client can abandon.
- Responses carry `api` types built field by field (`toAPIUser`); never marshal a
  `store` type, so a new store column can't leak.
- A person's preferences are `GET`/`PUT /me/prefs/{key}`. `internal/prefs` alone says which keys
  there are and what each holds, and checks a value on the way in and again on the way out; the
  store keeps the text. A new key is an entry there and its schema in the contract. Server settings
  never go there.
- Where a person left off is `GET`/`PUT /me/playback`, one record per person, last write wins: track
  ULIDs (at most 1,000), the index, `positionMs`, and a shuffle's session and position. `internal/playback`
  alone says what a record holds and resolves it against live tracks on the way out (gone songs
  dropped, albums as they are now). It publishes no event. `PruneShuffleSessions` keeps the session a
  record names.
- Playlists are `/me/playlists`, each person's own: another person's is a 404 to every route, never a
  403. `GET /me/playlists` lists them by name with counts, time and a mosaic (their first four albums,
  each once), and with `holding=` (at most 1,000 songs) says how many of those each holds, which is
  how Add to playlist marks ALREADY IN IT. `GET /me/playlists/{id}` gives its entries in order, each
  with its own entry ID (the row's, never reused) and its song in the index's shape; a song in twice is
  two entries. Adds take `tracks` and `albums` (an album's songs in the index's order), skip what the
  playlist holds unless `again`, and land at the end or `before` an entry; Undo of a take-out is that
  add. Moves name an entry and the one it goes `before` or `after`. A name is 1 to 200 characters
  trimmed, a playlist at most 10,000 entries; songs not live are a 422 naming them. Every change
  publishes `playlists` (ids only) to the owner.
- A smart playlist is a playlist with `rules` (`internal/smartlist`'s JSON) and no entries, worked out
  on every read, so a song joins or leaves as it is rated, played, added or trashed with no event of
  its own. `POST /me/playlists` with `rules` (and the `seed` its preview showed) makes one; `PATCH`
  changes `name`, `rules` or both, and rules on a regular playlist are a 409, as are a smart one's
  adds, take-outs and moves. `GET /me/playlists/{id}` is the regular shape plus `smart` and `rules`,
  its entry IDs only places (1 to N), each track with what rules read (`plays`, `lastPlayedAt`,
  `addedAt`, `year`, `format`, `bitrate`). `GET /me/playlists` works each one out as it lists it, and
  with `holding` leaves them out, since nothing is added to one. `POST /me/playlists/preview` is the
  rules editor's live card: the count, time and albums, the count before the limit, and the first 40
  songs, never a path; it follows the stored seed of the playlist named by `id`, else the `seed` sent.
  `POST /me/playlists/{id}/seed` is Pick again; `POST /me/playlists/{id}/copy` makes a regular
  "<name> (copy)" of what any playlist holds now (a 422 past 10,000). Rules a smart playlist can't
  have, rules with songs, a cycle (`cycle` names it) and nesting past `store.MostSmartDepth` are 422
  `SmartProblem`s. Rule changes, seeds and copies publish `playlists`.
- A file playlist (`source: file`) is read from a `.m3u` or `.m3u8` in the library (see Library) and
  is the server's: `GET /me/playlists` lists them for everyone beside a person's own (with `holding`
  it leaves them out), each with `file` (its path relative to its root, never the root, and how many
  lines found no song). `GET /me/playlists/{id}` adds `missing`, those lines in the file's order: the
  ones its read found nothing for and the entries whose song has left the library since (`trashed`
  when in the Trash). It is read-only: rename, rules, delete, adds, take-outs, moves and Pick again
  are a 409 "it comes from a file" (`store.ErrFilePlaylist`, from `ownRow`); `copy` and the
  `playlist` shuffle scope work, since `PlaylistRow` reads a person's own or a file's. A file
  playlist read, read again or deleted publishes `playlists` to everyone (`PlaylistsChanged`).
- `GET /me/playlists/{id}/m3u` exports any playlist a person sees as `.m3u8` (`internal/m3u`'s
  `Write`, a smart one as its rules choose now): paths relative to each song's own root, so no root
  is ever named, and `filename*` from `m3u.FileName`. `POST /admin/playlists/{id}/save` (`root`, a
  root-relative `folder`, `name`, `replace`) writes it into a library folder with paths from that
  folder (`../` where needed), leaving out songs on another root (`leftOut`); a 409 with tag writes
  off or while busy, a 422 for a folder outside its root or a name with a separator or a leading dot,
  and a 409 `PlaylistExists` (the playlist read from it, its count and mtime, and `freeName`) unless
  `replace`. `Scanner.SavePlaylist` writes under the library lock through `os.Root`, a
  `.<name>.kitsune-<random>.tmp` beside the target (which `isWriteTemp` sweeps), synced, then
  renamed, and reads the file at once.
- What a not-found line may show: a relative line as written; an absolute one inside a root as its
  root-relative path; one outside every root only its last two segments after `…/`; a URL only its
  scheme and `//…` (`m3u.Ref.Shown`, `m3u.Shown`). `TestFilePlaylistIsSharedAndNamesNoRoot` holds it.
- The organizer is eight admin routes under `/admin/organize`. `POST /preview` (`template`,
  `compilations`, `tracks` or `albums`; the saved templates without them) plans every song, or some,
  and keeps the plan in memory for 30 minutes (at most 8, the oldest dropped) for `GET
  /previews/{id}` (200 albums a page, `filter`, `q`, an offset `cursor`) and for its run; a template
  that doesn't parse is a 422 `OrganizeTemplateProblem` (`field`, `at` in code points). `GET
  /examples` gives a few songs with what a template reads of them (`OrganizeSong`), so the client's
  copy of the parser draws them as the template is typed. `POST /runs` (`previewId`, and `tracks` or
  `albums` to run some) answers 202 at once and the run goes on as `Config.Work`, the app's background
  work; `GET /runs` (the last 10, a running one first), `GET /runs/{id}` (with up to 500 songs it left
  and why), `POST /runs/{id}/stop` and `/undo`. A run with tag writes off, a scan, an edit or another
  run holding the library, or a root that looks down is a 409 (Retry-After when it may pass). Every
  path in a response is relative to its root, named by its index in `library.roots`; conflicts carry
  the caller's plays and stars and `sameRecording`, true when Duplicates puts the songs in one group
  (`why` says by what, `group` names it for `/server/duplicates/{group}`). Progress is the `organize` event, to administrators: run, state, counts, `albumsDone`
  and the album in hand by ID, never a path. `Me.canOrganize` is `canTrash`'s rule.
- Duplicates is five admin routes under `/admin/duplicates`. `GET` lists the groups (`filter`: `all`,
  `audio`, `recording`, `probably`, `pairs`, `dismissed`; 100 a page by an offset `cursor`; pairs on the
  first page only, and under `all` a pair's groups are in its card), with counts and how many songs the
  fingerprint pass has `checked`. `GET /{group}` is a group (`kind` `group`) or an album pair (`pair`):
  each copy's facts (root-relative path, everyone's `plays`, `raters` and the caller's own `stars`,
  never another person's), the suggested `keeper` and its `reasons`, and `plans`, what keeping each copy
  or album would move. `POST /{group}/merge` (`keep`, `trash`, and the `plan` the confirm showed) is a
  409 `PlanChangedProblem` with the plan as it is now when that changed, a 409 with tag writes off or
  while busy, a 422 for copies not of the group; it publishes `marks` (each person whose keeper's stars
  rose), `playlists` (owners, and everyone for file playlists), `shuffleRules`, and `Forget`s the
  shuffler's history of everyone whose plays moved; the scanner publishes `library`. `POST` and
  `DELETE /{group}/dismiss` are Not duplicates and Bring back. A group's key is a hash of its songs'
  ULIDs (`duplicates.Key`), so a third copy raises a dismissed group again; a pair's, of its albums'.
- ReplayGain in the files is five admin routes under `/admin/replaygain`. `GET` says what a run would
  write now (songs, albums, formats, the four biggest albums), what still waits for the loudness pass
  (`waiting`) or can't be measured, the run going and the last 10 runs. `POST /runs` (`albums`, or none
  for all) answers 202 at once and the run goes on as `Config.Work`; nothing named that can be written
  is a 422, and tag writes off, busy (Retry-After) or a root that looks down a 409, as Organize's. `GET
  /runs/{id}` lists its albums in its order (written, writing, waiting or left) and up to 500 songs it
  left with why, by root-relative path; `POST /runs/{id}/stop` and `/undo`. Progress is the `replaygain`
  event, to administrators: run, state, counts, `albumsDone` and the album in hand by ID, never a path.
  It shares the scanner's claim with scans and Organize, so one waits for the other.
- `GET /me/home` is Home's history, 25 albums each as `GET /albums` lists them: `recent`, by latest
  play, and `onRepeat`, by plays in the last 90 days (`onRepeatWindow`, the server's clock). Both
  count `plays` (imports included, but undated ones never on repeat, `store.SourceUndated`), never
  listens, and only live tracks. Recent reads plays newest first and stops at 25 albums
  (`recentAlbumsSQL`, hand-written since sqlc reads every row); `home-history` is its budget.
- An artist's art (plan 072): `ArtistDetail` carries `images` (`thumb`, `background`, `logo`,
  `banner`, each an `ExtraImage` with its colors, `alpha`, `source` and `credit`, any absent), `bio`
  (`summary`, `body`, `source`, and `url`, `license`, `licenseUrl`, `lang` where the source gives
  them) and `mbid`; `ArtistEntry` in `GET /artists` carries `thumb` (a `Picture`), read in one query
  for the whole list. `AlbumDetail.extras` holds `discs` (every disc's art as disc 0, then by disc),
  `back` and `coverSource`. `GET /artwork/{hash}` takes every ladder's sizes (128, 400, 512, 640, 800,
  1024, 1920, 3840) and answers JPEG or, for an image with `alpha`, PNG. No response names a file or a
  root. The `artists` event (ids only) goes to everyone when a scan or the artist pass changes an
  artist's images or bio. `GET /artists/{id}` for an artist with an MBID and no art at all asks the
  pass for it at a person's priority (`Scraper.Want`) and answers at once; the art comes on the event.
- `GET /admin/metadata` is Health's Metadata card (Phase 2, see Scrape): `remote`, `language`, each
  source (`musicbrainz`, `wikipedia`, `fanarttv`) with `enabled`, `state` (`on`, `off`, `needsKey`,
  `refused`, `resting` with `until`), `key` and `clientKey` (`set`, `missing` or `none`, never a value)
  and its last `failure` in words; `pass` (running, done of total, the artist in hand); `artists`
  counting album artists (`withMbid`, `lookedUp`, `withArt`, `foundNothing`, `withoutMbid`, which
  Matching fixes); `nextRefresh`. A 503 without a scraper. Progress is no event: read it again.
- Stars are the only like: a song's own rating, ½★ to 5★ in halves, is what every list, filter, sort,
  album average and shuffle weight reads. Nothing is hearted any more. The hearts people kept from
  before (`track_hearts`, `album_hearts`, `artist_hearts`) show and weigh nowhere; they wait for
  `POST /me/hearts/convert`, the You page's one-time card, which in one transaction rates every
  hearted song without stars as the person chose (or leaves them unrated for `null`) and deletes all
  of that person's hearts. `GET /me/hearts/summary` says whether the card is drawn (all zeros: not).
  The conversion reaches no Last.fm outbox: those songs were loved through their hearts. Each song it
  rates is a `marks` event. The tables stay until a later plan drops them, after everyone converted;
  `Store.Heart` stays only so tests can stand in for kept hearts. A `library.columns` value naming the
  old `heart` column comes back without it.
- Last.fm loves follow stars: `lastfm_accounts.love_from` (halves, 4★ until moved) is the love line,
  and `loves` its switch. `Store.Rate` and `Unrate` queue a love when a song's own rating crosses the
  line upward (unrated counts as under it) and an unlove for the reverse, in the rating's own
  transaction (`loveCrossed`); a change that stays on one side, and moving the line itself, send
  nothing.
- When an OpenSubsonic layer lands, its `star`/`unstar` map to the love line: `star` raises a song
  rated under `love_from` (or unrated) to `love_from`, and `unstar` drops one at or over it to half a
  star under the line.
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
- A playlist's entries (`playlist_tracks`, `playlists.go`) are spaced `positionStep` (65,536) apart:
  a move writes one row, at the midpoint of its new neighbors, and the playlist is renumbered only
  when a gap runs out (parked below zero first, so the UNIQUE place never collides). Adds go in as one
  statement from a JSON array of `[position, track]`. Entries go with their playlist and person, and
  with their track only when it is truly deleted (`deleteTracks`, by ON DELETE CASCADE): a trashed
  track keeps them, every read of a playlist is through live tracks, so Restore brings them back in
  place.
- `internal/smartlist` alone says what smart rules are: the field and operator table, the limits, and
  their JSON; `Parse` is the only way to a `Rules`, as `ParseSearch` is to a search. `store/smartlist.go`
  compiles a `Rules` to one statement over `smartColumns`, the allow-list, with every value bound;
  `TestNoRuleValueReachesTheStatement` feeds every field SQL punctuation and holds the text unchanged.
  What SQL can't say is worked out in Go first and bound as a JSON array of rows (`json_each`): text
  rules, folded by `FoldText` (a search's folding) against what `libraryrows.go` keeps; genre rules
  through `GenreAlbums` (the server's `library.Genres.AlbumsWhere`); the person's stars, plays, last
  plays and skips (`personFacts`, read once per request, so a rule on them needs no join); and playlist
  rules, a regular playlist through its entries and a smart one by evaluating it, at most
  `MostSmartDepth` (3) smart playlists down. One gone, another person's or past the depth matches
  nothing; a save that names its way back is a `CycleError`. The statement returns only rows (and
  sizes for a GB limit); the limit and the order are applied in Go, ties in the Library's order
  (`libraryOrder`, from `orderIndex`), random choices by a hash of the stored `seed` and each song's
  ULID. What it works out of the library (that order, folded names, details) is kept beside the
  index's rows (`libraryRows.smart`). The `playlist_tracks_not_smart` trigger refuses entries for a
  smart playlist, as `ownEntries` does.
- A file playlist (`playlistfiles.go`) has no owner: `playlists.user_id` is NULL exactly when `source`
  is `file`, a CHECK since `0022`'s rebuild. `playlist_files` says which file it is (root, path, size,
  mtime, the reader's `m3uVersion`, and `missing`, the JSON of its not-found lines) and goes with it.
  Its entries are `playlist_tracks` rows whose position is the line's number and whose `line` is the
  path as written (NULL for every other playlist); `WriteFilePlaylist` replaces them in one
  transaction and keeps the ULID while the file is at its path. Entries go with their track as any
  playlist's do, so a trashed song waits in place for Restore and the purge takes it.
- The organizer's journal (`organize.go`, migration `0023`): `organize_runs` (kind `run`, `undo` or
  `edit`, state `running`, `done`, `stopped` or `undone`, the template, counts as it goes, `undo_of`)
  and `organize_moves` (one row per song, companion, folder removed or playlist rewritten; `planned`,
  then `moved`, `skipped` with a reason, or `undone`). `track_id` is ON DELETE SET NULL, so a purge
  never breaks a journal. `LandOrganize` sets each moved song's `rel_path` (only from the path the
  run found it at) and its row's state in one `inLibraryTx`, and notes the albums. `OrganizeLibrary`
  reads every live song for a plan from the index's kept rows plus `files` (path, size, mtime,
  original year), kept the same way. The last 10 runs, or 90 days, whichever keeps more, stay; the
  rest go with their rows.

- Duplicates (`duplicates.go`, `duplicatemerge.go`, migration `0024`): `duplicate_dismissals` (key,
  the ULIDs as JSON, by user, at). `DuplicateLibrary` reads every live song for a find from the index's
  kept rows plus `duplicateTags` (recordings and release types, kept the same way) and the fingerprints,
  read every time since the pass stores them outside library writes; `DuplicatesMark` (library writes
  and fingerprints stored) says when a find is stale. `MergeAndTrash` is one `inLibraryTx`: it reads
  what the copies hold, works out the plan (`mergeFacts.plan`, the only place a plan is counted) and
  refuses with `PlanChangedError` unless it is the one the confirm showed, then per trashed copy: plays
  and listens repointed; each person's higher stars (with the trashed copy's `rated_at`) on the keeper
  through `loveCrossed`; kept hearts moved unless the keeper has one, the earlier kept; playlist entries
  (file playlists' too) repointed in place, so a playlist holding both holds the keeper twice;
  exclusions carried (the union); a Combine place taken by the keeper unless the person has it in a run
  already, when the copy just leaves (`shuffle_runs_need_two` tidies); `replace` of the quoted ULID in
  `playback.queue`; the copy's own ratings, hearts and exclusions deleted; then `markTrashed`, as
  `MarkTrashed` does. The Last.fm outbox, edit history, lookups and the copy's loudness, fingerprint and
  artwork are left alone.
- ReplayGain writes (`gainwrite.go`, migration `0025`): `gain_runs` (kind `run` or `undo`, state
  `running`, `done`, `stopped` or `undone`, counts, `undo_of`) and `gain_writes` (one row per song: its
  album's ULID and `ord`, the root and path at the time, `planned`, then `written` with the file's size
  and mtime after, or `skipped` with a reason, or `undone`; the four values written, the album's NULL
  when none; `prior`, the ReplayGain tags the file held before as JSON of keys as spelled; an undo's
  row names the write it `undoes`). `track_id` is ON DELETE SET NULL. `GainLibrary` is one statement
  over `tracks_ungained`, the measure of each file and its album's pool (current only when it pools
  exactly the album's live songs, each measured for its file now), and leaves out a file a run wrote
  already (a gain that rounds to 0.00 dB reads as none). The last 10 runs, or 90 days, whichever keeps
  more, stay. `TrackWrite.Retagged` says audiometa rewrote only a file's tags, so its update keeps the
  stored fingerprint and moves its loudness measure (`CarryTrackLoudness`) to the new size and mtime.
- Artist art (`extras.go`, migration `0026`): `artist_images` (one row per artist and kind: `thumb`,
  `background`, `logo`, `banner`), `album_images` (per album, kind `disc` or `back`, and `disc`, 0 for
  every disc) and `artist_bios` (one per artist, plain text). Each says where it came from (`source`:
  `folder` or `nfo` today) and, for a library file, which one (`file_root`, `file_path`, `file_size`,
  `file_mtime`, NULL for any other source, a CHECK); a `pinned` row is an administrator's and no write
  of a scan's touches it (`UpsertFolderArtistImage` skips it, the deletes take `folder` rows alone).
  They go with their artist or album by ON DELETE CASCADE. `WriteExtras` stores a scan's finds in one
  transaction (not a library write: the index shows none of it), returns the artists it changed and
  notes the albums. `FolderLayout` is what the scan reads to find folders: every live track's file
  and every album's owners, kept between library writes as the index's rows are, and what the store
  holds of images and bios. `albums.release_group_mbid` is kept with `coalesce`, as `mbid` is.
- The artist pass's state (`scrape.go`, migration `0027`): `scrape_state` (source `fanarttv` or
  `wikimedia`, kind `artist` (`album` later), keyed by MBID not row, `status` `found`, `none` or
  `failed` with `failures` in a row and `error` in words, `fetched_at`, `next_at`) and `scrape_feeds`
  (when fanart.tv's latest feed was read). `WriteRemote` is one transaction per artist: images as
  `artist_images` rows of source `fanart` or `wikimedia` on `artwork` rows of source `remote`, with
  `remote_id` and `credit`; a bio of source `wikipedia`; the states. Its upserts skip a `folder` or
  `pinned` row and an `nfo` bio, and its deletes take only the named source's row (a `folder` or
  `pinned` source is `ErrInvalidWrite`). A folder that gives up an image or a bio brings every
  source's `next_at` for that artist forward (`AskAgainForArtist`), as does `AskAgain` for the feed.

## Matching
- `internal/matching` finds an album's release at a source (MusicBrainz now; Discogs later, as another
  `Source`) and says what its tags would become. It proposes and never writes: the client applies what
  a person ticks through `POST /albums/{id}/edits`, so everything editing guarantees holds. It knows
  nothing of the store; `server.LookupCache` keeps its answers in `lookups` for a day.
- One `MusicBrainz` client serves the process (built in `cmd/`): its gate keeps every request to one a
  second and serves people before prefetch (`matching.Background`, which is `remote.Background`). It
  names itself `Kitsune/<version> ( https://github.com/simonhull/kitsune )` and asks again after a 503
  or a 429. `ArtistLinks` (`artist/{mbid}?inc=url-rels`, kept a week) gives the artist pass an artist's
  Wikidata item.
- Confidence is evidence-weighted, and evidence is structured facts the client words, as shuffle's
  `why`. Matching never proposes taking a value away, and never guesses: a field the release lacks is
  left out.
- MusicBrainz IDs are fields of the table (`musicbrainz…Id`); a match always writes them with the songs
  it touches, artists' only where they line up with the song's artists.
- Tests use `internal/matching/musicbrainztest`; nothing ever talks to the real MusicBrainz.

## Import
- `kitsune-server import navidrome FILE` brings plays and ratings over from a copy of a
  Navidrome database, once, while a server runs. `internal/store/navidromedb` reads it read-only and
  immutable, so nothing is created beside it, and refuses a non-empty `-wal`. It selects only the
  columns in its `schema.sql`, which sqlc generates from and `Open` checks: the file also holds
  password hashes and Last.fm session keys, and `TestNothingReturnedHoldsASecret` keeps them out.
  `internal/navidrome` plans the import and is pure (no SQL, no writes); `Store.ApplyImport` writes
  each person in one transaction.
- It never creates tracks or accounts, and like `users add` refuses a database with migrations
  pending. Songs match by path under the paired root (`-root`, and `-map OLD=NEW` for a moved file),
  then by a release-track or recording MBID exactly one song on each side has; people by name,
  ignoring ASCII case (`-user`). A starred song with a rating keeps just the rating; one without is
  rated `-heart-stars HALVES` (default 8, 4★; 0 leaves it unrated), as `HeartRatings`, which never
  outrank a rating another Navidrome song gives the same track. Starred albums (matched through their
  songs) and starred or rated artists (by match key, then MBID) are counted in the report and not
  imported. An option that names nothing is an error, not a skip.
- Plays are `plays` rows, never `listens`, which say how much played. Client IDs
  (`navidrome:s:<scrobble>`, `navidrome:d:<song>`, `navidrome:u:<song>:<k>`) make a re-run add
  nothing. A play Navidrome kept only as a count has source `navidrome-undated` and the latest time it
  can have happened, so each song's latest play is its `play_date`; a history view leaves them out.
  Ratings arrive as `halves = 2r` and never replace one Kitsune has.
- Nothing an import writes reaches Last.fm: it never goes through `RecordPlay` or `Rate`, and
  `TestApplyImportQueuesNothingForLastfm` holds `lastfm_outbox` empty.
- Below half the played, marked or listed songs matching by path, it writes nothing (the gate);
  `-dry-run` prints the same table and writes nothing either. Bookmarks are only counted. Tests build
  Navidrome files with `navidromedbtest`; none ever reads a real one.
- Playlists come over: each non-smart playlist of a matched person becomes theirs, source
  `navidrome`, its songs matched as plays are, in Navidrome's order with its repeats, unmatched ones
  counted and left out, and past 10,000 cut and counted. A smart playlist (`rules` set) comes over as
  Kitsune rules when every criterion maps (`internal/navidrome/smart.go`, checked by
  `smartlist.Parse`), with a seed of its own and no entries; one with any criterion that doesn't map
  (startsWith, endsWith, fixed dates, a field Kitsune lacks, nesting past one group, a playlist rule
  naming one that isn't coming over, a limit by a sort no chooser keeps) stays behind whole, never half,
  and the report names it with what stopped it. `loved` is the person's love line (`love_from`, 4★
  without Last.fm); `gt`/`lt` are one step over or under, half a star for ratings; durations round
  inward to minutes. Smart playlists naming each other in a cycle, and any naming one left out, stay
  behind too. The import key `navidrome:<id>` makes a re-run skip a playlist it brought before, smart
  ones included, so one renamed or edited here stays as it is; one deleted here comes back on the next
  run.

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
- The organizer's keys (`settings/organize.go`): `organize.template` and `organize.compilations`
  (each must parse; the error never quotes it), `organize.roots` (an object of absolute folders, each
  with both templates), and the switches `organize.companions` (on), `organize.on_edit` and
  `organize.rewrite_playlists`. server.toml doesn't carry them; a variable or `-set` may seed them.
  `follow.go` hands them to the scanner (`SetOrganize`), which doesn't wait: a run keeps its preview's
  templates.
- The artist pass's keys (`settings/metadata.go`): `metadata.remote`, the master switch, off on a new
  server; `metadata.fanarttv` and `metadata.wikipedia` (on, under it); `metadata.language` (two letters,
  `en`); `fanarttv.api_key` (the project key) and `fanarttv.client_key` (a person's own), secrets as
  `lastfm.shared_secret` is, unset by default. Kitsune ships no key and builds none into a release:
  without a project key fanart.tv is off and says so. server.toml doesn't carry them; a variable (or
  `-set`, keys aside) seeds them. `follow.go` hands them to `Scraper.Configure`, which takes them at
  once and asks every source again.

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
  A field the editor has a row for is read by a scan and by the editor through the row's `scan` list
  (after audiometa's typed field, for the scan), so the two never disagree;
  `TestScanReadsEveryEditorKey` holds each list to every key the row has in any format.
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
  exist. Kitsune writes them for the fields it edits through raw keys; a `typed` field (track and
  disc numbers, comment) is mirrored by audiometa itself, which refuses an edit of both a field and a
  key it binds to the field. The ID3 version, which decides the family, comes from the chunk
  (`chunks.go`).
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
  A file whose tags alone audiometa rewrote (a tag edit, a ReplayGain write) keeps its fingerprint:
  audiometa verified its audio is bit-identical, and the digest covers audio alone.
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
- A trashed track (`trash.go`) keeps its row, ULID and everything people did with it; its file moves
  to `<root>/.kitsune-trash/<ULID>/<relative path>` (`TrashDir`), a rename under `os.Root` on the
  root's own filesystem. Its `missing_since` is its `trashed_at`, so every live read leaves it out
  with no rule of its own (`TestTrashedTrackIsInNoRead` holds every read to it); what reads missing
  tracks to follow, hold or delete them (`FileStates`, `MissingByDigest`, `KnownRoots`,
  `DeleteMissingBefore`, imports) adds `trashed_at IS NULL`, so the outage rules never count one.
  `rel_path` always names where the file is (in the Trash while trashed) and `restore_path` where it
  goes back. Every walker skips the folder through `SkipsFolder`. A trash marks the rows, then moves
  the files; a restore moves them back, then clears the rows, so a server stopped midway leaves the
  file in the library. A restore never overwrites: it refuses, restoring nothing, when a file or a
  song is at an old path (`PathTakenError`), and links before it removes so nothing slips in. Thirty
  days on (`TrashRetention`) `PurgeTrash` deletes the file, then the row through `deleteTracks`, the
  one way a track leaves the store, after each scheduled scan and the start's, in batches like the
  fingerprint pass, and never with tag writes off. A copy trashed by a Duplicates merge has given
  its plays, listens, stars, hearts, playlist entries, exclusions and Combine places to its keeper, so
  Restore brings it back bare.
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
- A ReplayGain run (`gainwrite.go`, `gainundo.go`, `gainfile.go`) writes the measure into the files of
  songs with none, exactly what Kitsune plays (`loudness.Gain` of the track and of the album's pool, the
  true peaks), rounded as a tag holds it, through audiometa's `SetTrackReplayGain` and
  `SetAlbumReplayGain`, never by raw key. It never runs on its own. `StartGain` takes the claim through
  `claimRun` (Organize's too) and the library lock one batch at a time (one album, at most 64 songs);
  per batch it checks each song (live, at its path, size and mtime as the store and the disk have them,
  no gain tagged, measure and pool current, writable), reads the file's ReplayGain tags (`prior`; a
  peak alone or a 0.00 dB gain reads as none, and is written over), journals the batch, writes each
  file, reads it back (`reindexFile` with `retagged`, so the fingerprint and the measure stay) and
  settles the journal. The edit refuses a file whose fresh read holds anything but its `prior`. A file
  is written in place (`WithInPlace`) unless a stream has it open (`OpenFiles`, `stream.Files`), when
  the atomic rewrite leaves the stream its old file; a stream that opens it during a patch of a few
  milliseconds is the known limit. Undo finds each song by track, wherever the organizer put it, skips
  one whose tags no longer read what the run wrote (`retagged`), and writes `RemoveReplayGain` then the
  `prior` as spelled; the songs then play measured again at once. `RecoverGain` runs at start after
  `RecoverOrganize`, before anything scans: a planned row whose file reads what it planned (or, for an
  undo, holds exactly the tags before) is read back and written, any other skipped `interrupted`. A
  test seam, `gainFault`, panics a run at each step as a crash would.
- Playlist files (`playlists.go`, `IsPlaylistFile`: `.m3u`, `.m3u8`) are found by the walk, the
  watcher and the poller beside the audio, and read after the scan's songs under the same claim and
  lock, so a file and its songs arrive together. One is read only when it is new, its size or mtime
  changed, `m3uVersion` went up, the scan is full, or songs were added or moved and it has lines that
  found none (a targeted scan that adds songs reads those files on every root); an unchanged file is
  a stat. `internal/m3u` is pure: `Parse` keeps to 4 MiB, 25,000 lines, 4,096 bytes a line and 10,000
  paths and counts the rest as past the limit; `.m3u` falls back to Windows-1252. A line is matched as
  a string, never through the file system (`m3u.Resolve`, then `matcher`): the track at that root and
  path (a trashed one by its `restore_path`), else the one track whose path folds the same under NFC
  and case folding; two is no match, never a guess, and another extension in the folder is only a
  hint (`otherExtension`). A line that no longer names a song keeps the one its last read found while
  that is live (`FileEntryLines`), so a tidy-up never empties a playlist. A file that can't be read is
  an `unreadable` scan issue, tried every scan. A playlist goes when the walk, or a targeted scan of
  its folder, finds its file gone, and never for a root that looks down, by the same rules as tracks.
  Files are read with tag writes off too; only saving writes.
- Artist and album folders (`extras.go`, `extrafolders.go`, `extranames.go`, `nfo.go`): the walk keeps
  the files named as Kodi and Jellyfin name them (`isExtraFile`), and after the songs and playlist
  files the scan's extras step works out every folder from the store's layout. An album's folder is
  the one folder all its live songs are in, disc folders folded; an artist's folder is the one above
  an album's, when every live song in or below it is on an album that artist alone owns (a
  compilation, Various Artists or a shared album makes it nobody's), so one artist never takes
  another's images; the root can be one. In the artist's folder `artist|thumb|folder` is the thumb,
  `fanart|backdrop|background` (then `fanart1`, `backdrop-2`…) the background, `clearlogo|logo` the
  logo (PNG first), `banner` the banner and `artist.nfo` the bio; in the album's, `cdart|discart|disc`
  is every disc's art, `cd2`, `discart-2` and the like one disc's, and `back` the back cover. Earlier
  names win, then extensions, then the name sorted first. Files are read through `os.Root` and a file
  unchanged since its row (path, size, mtime) is not read again but by a full scan; one that serves
  nothing (a bad image, an nfo naming another artist's MBID) is logged by its root-relative path and
  remembered until it changes (`badExtras`), and the next name is tried. A folder this scan didn't
  read whole (a targeted scan's others, an unreadable one) keeps what it gave. A file gone takes its
  image away; a pin is never touched. Embedded pictures are never read for these. `artist.nfo` is
  read leniently, `<biography>` and `<musicBrainzArtistID>` straight under `<artist>` ignoring case;
  its lines are the body's paragraphs and the first is the summary. A targeted scan runs the step
  only when the folders it read hold such a file, it read or lost a song, or the store holds a file
  from a folder it read. Changed artists are an `artists` event (`Notifier.ArtistsChanged`); changed
  albums ride on `library`. Without an artwork cache only bios are read.
- A scan reads the release group's MBID (`MUSICBRAINZ_RELEASEGROUPID` and its TXXX, through the field
  table's `scan` list) onto the album, for the fetching phases' album art.
- A targeted scan refreshes genres and names only when it read a song or marked one missing.
- `internal/duplicates` is pure: `Find` buckets songs by fingerprint (same audio), MusicBrainz
  recording (lengths within 10 s) and first artist's `MatchKey` plus title folded as a search folds,
  a trailing remaster note taken off (within 2 s), and joins songs inside a bucket only, sorted by
  length, so it is linear. `VersionWords` (one in a title and not the other) and another disc of one
  album keep songs apart for the last two; a bucket of more than `MostInBucket` (16) songs is no
  evidence (the same silence, a placeholder ID). A group's confidence is the strongest any two earn.
  Three or more groups pairing two albums make an `AlbumPair`, with each album's songs the other lacks
  (`Only`), never trashed. `Keeper` orders copies: lossless, then an original album (its owner is the
  song's first artist, not a compilation, and no release type `compilation` or `soundtrack`, from
  `keysReleaseType`), a bitrate over 5% higher, in place by the organizer's templates, more plays,
  added first; its reasons are each way the keeper beats every other copy.
- `Scanner.FindDuplicates` keeps its find until `DuplicatesMark` moves. `Scanner.MergeDuplicates`
  holds the library lock (`exclusive`, so `ErrBusy` while a scan, an edit or a run holds it), checks
  tag writes and that every song is live, runs `MergeAndTrash`, then moves the files into the Trash as
  `TrashTracks` does. A file that won't move puts every file back and unmarks the copies: they return
  bare, since what they held is already on the keeper.
- `internal/organize` is pure, as `internal/smartlist` is: `Parse` reads a template (`<Field>`,
  `<A|B>` fallbacks, `<Track:3>` widths, `[…]` groups that go when their fields are empty, `/` the only
  folder break; it must end in `<Title>` or `<Track>`), `Template.Path` files a `Song`, `Safe` makes a
  name every filesystem a library is read from can hold (the fixed rule, idempotent, fuzzed; a leading
  `.` or `@` becomes `_`, since a scan skips such a name), and `MakePlan` works out every song's target
  and what is in its way: two songs that fold to one path, a song there, or a file Kitsune doesn't know,
  through a folder-listing func its caller hands it. `testdata/templates.json` holds the parser to the
  web client's copy.
- The organizer (`organize*.go`) moves files only through the Trash's `openRoots`: `os.Root`, a link
  then a remove (`moveBack`), never a copy and never over anything; `EXDEV` is `otherFilesystem`, and
  a change of case alone goes through a hidden `.<name>.kitsune-organize` in the same folder. A folder
  that exists only in another case, on a filesystem that ignores case, is refused, not merged.
  `StartOrganize` and `StartUndo` take the claim (no scan while a run goes) and answer at once; the run
  takes the library lock one batch at a time, as the passes do (one album, at most 64 songs, so an
  edit or a scan waits out one batch), and per batch: checks each song is still as the preview found
  it (path, size and mtime in the store and on disk, else `changed`), journals the moves `planned`,
  moves the files, lands them, brings the companions (covers, `.cue`, `.log`, `.lrc`, `.txt`: one named
  for a song follows it; the rest of a folder only when all its songs go to one new folder), and
  removes the folders it emptied, each journaled, a folder holding anything else left. Stop finishes
  the batch in hand. A run never starts, or goes on, while a root it touches looks down
  (`rootLooksDown`, the full scan's test). `RecoverOrganize` runs before anything scans or watches
  (`serve`) and settles every `planned` row by where its file is, never removing the only copy (a
  change of case is judged by the names its folder lists); runs left `running` become `stopped`.
  Undo is a run in reverse over the journal: a song not where the run put it (`movedSince`) or with
  something at its old path (`taken`) stays. With `organize.on_edit`, an edit notes which of its songs
  sat where the templates put them, and `RunOrganizeEdits` (the app's background work) files those by
  their new tags as one run of kind `edit`. With `organize.rewrite_playlists`, a run rewrites the lines
  of UTF-8 playlist files in its roots that named a moved song (relative stays relative), after
  linking the original to `<name>.kitsune-bak`; its undo rewrites them back and removes the copy. A
  test seam, `organizeFault`, stops a run at each step as a crash would.

## Scrape
- `internal/scrape` is the artist pass (plan 072, Phase 2): images and bios for artists, only ever by
  the MBID the library holds (`artists.mbid`), never by a name. Not an MBID, Various Artists and
  MusicBrainz's special-purpose artists (`Askable`) are never asked about. Turning `metadata.remote` on
  sends MBIDs to the sources and nothing else.
- Sources, in precedence after pins and the library's own files: fanart.tv (`scrape/fanarttv`:
  `artistthumb`, `artist4kbackground` then `artistbackground`, `hdmusiclogo` then `musiclogo`,
  `musicbanner`), then Wikimedia (`scrape/wikimedia`: MusicBrainz names the Wikidata item; its
  article's lead in `metadata.language`, else English, from Wikipedia's page summary, credited CC BY-SA
  4.0 with its link; its P18 portrait from Commons at 1024 px, credited from `extmetadata`, as a thumb
  fanart.tv lacks). `Decide`/`DecideBio` and `Pick` are pure: a pin or a folder's image (an `nfo` bio)
  is never replaced; the first source that answered with an image gives it; one not asked or failed
  keeps its own; an image goes only when its own source answered with none and no later one has one
  (then the other sources are asked again). Among one source's images: tier, language (wanted,
  English, none, other), likes, size, the oldest ID. Nothing remote ever removes local art.
- `internal/remote` is every service's client (the gate MusicBrainz used, moved): a gate per service
  with people before `Background` work, one request per key at once, retries on 429 and 503 honoring
  `Retry-After` (a wait past a minute ends the request), a body cap, the User-Agent, and errors that
  wrap `ErrUnavailable`, `ErrNotFound`, `ErrRefused` (401, 403), `ErrRejected` or `ErrTooLarge` and never
  quote a URL. Keys go in headers (fanart.tv's `api-key` and `client-key`), never a URL. Gates:
  fanart.tv's JSON one a second, its images two; Wikimedia one a second for all its sites.
- `Scraper.Run` is the app's background work: a pass at start, when kicked (every `LibraryChanged`,
  so a scan, an edit or a match that gave an MBID), and when the next artist falls due. It waits on
  `Scanner.WaitScanFree` before each artist (a scan's covers first) and renders through the scan's
  `artwork.Cache`, sharing its two renders. People's artists go first, then album artists by
  everyone's plays, then the rest; one artist is one `WriteRemote` and, when it changed, one `artists`
  event. A source is asked only when due and only for what pins and folders leave free.
- Refresh (`NextAsk`, pure): found, 27 to 33 days by a hash of the MBID; none, 7 days then 30; a
  failure (5xx, timeout, a body that doesn't parse or is too large, an image's address not the
  source's, as fanart.tv's bare file names of November 2025) 1 h, 6 h, a day, a week. A 404 is none. A
  refusal (401, 403) stops that source until a setting changes; a source down or busy rests (a minute,
  doubling to an hour, or its `Retry-After`) without counting against later artists. fanart.tv's
  `latest` feed is read once a day and brings forward every artist on it already asked about.
- `describe` words a failure for the card ("fanart.tv refused the key", "fanart.tv answered 503") from
  sentinels and statuses alone. `TestNoErrorQuotesAKey` (`fanarttv`, `scrape`) holds every error,
  log line, stored error and the report to no key against every stand-in fault.
- Tests use `fanarttvtest`, `wikimediatest` and `musicbrainztest` (`AddArtist`); each answers 429 with
  Retry-After, 503, a refused key, bare file names, slowly and too much. The pass's tests guard their
  transports so a request that would leave httptest fails the test.

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
- Weights come from stars alone: the engine has no heart factor. The `rated` scope is songs whose own
  stars are 4★ or more. A session or playback record saved with the old `hearted` scope goes on as
  `rated` (`shuffler.ParseScope`, which both read it through).
- The `playlist` scope is one of the person's playlists: its songs, each once however often it holds
  them, live ones only, with the usual marks and rules; another person's is a 404. A smart playlist's
  are the songs its rules choose when the session starts, the limit applied, its genre rules through
  the shuffler's `Genres`.
- The web client starts every shuffle without waiting: it picks the first song from what it holds
  and plays it, and asks `POST /me/shuffles` with it as `first` at the same time.

## Artwork
- `internal/artwork` renders covers, artist images and album extras during the scan and knows
  nothing of the store or the scanner. Renditions live at `<data_dir>/artwork/<hh>/<hash>-<size>.<ext>`,
  named by the SHA-256 of the source image, so a request only ever reads a file and a file never
  changes. `Put` takes a `Kind`, and each kind has its ladder on the longest side (`Kind.Ladder`):
  covers, thumbs and back covers 128/512/1024, backgrounds and banners 128/640/1920/3840, logos and
  disc art 128/400/800; every ladder starts at 128 and none upscales. Renditions are JPEG flattened
  onto the ground, except a logo's or a disc's whose source has transparency, which stay PNG
  (`Image.Alpha`, `artwork.alpha`). One image put as two kinds keeps both ladders. `Path` returns the
  largest rendition of any ladder at or below the request (JPEG first at a rung), `IsPNG` its format,
  and `Delete` takes every rung of both formats. A zero-size file counts as missing.
- `PruneOrphans` keeps every artwork row an album's cover, an artist's image (`artist_images`) or an
  album's extra (`album_images`) uses; artists go before the images, so an artist pruned frees its
  own. A prune or a delete of missing tracks that deletes nothing leaves the kept rows standing
  (`inLibraryTxIf`), which is what keeps a quick scan's extras step and the next index read warm.
- The scanner finds one cover per album per scan (`covers` in `internal/library/cover.go`): the cover
  of the album's first track that has one. An album's first track, whose cover, year and compilation
  flag it takes, is the first the Library lists (`store.Place`): by disc, songs with no disc number
  after every disc, then track number (none last), title and path; `SettleAlbum` and
  `ListAlbumCoverFiles` spell the same order in SQL. A track looks unless one before it has found a
  cover, and the writer drops a cover that arrives after a better one, so worker order never
  decides. Art never fails a file: bad images become warnings, cache failures are logged. Folder art is read through
  `os.Root`, never through `fs.FS` (it rejects non-UTF-8 names).
- A cover's accent and ground colors come from the encoded 128 rendition, so a fresh render and
  an idempotent `Put` agree; a PNG's pixels less than half opaque have no say, so a logo's colors are
  its ink's.

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
  `KITSUNE_FANARTTV_URL` (its API at `/v3.2/`, images at `/fanart/`) and `KITSUNE_WIKIMEDIA_URL`
  (`/wikidata/`, `/wikipedia/{lang}/`, `/commons/`, `/upload/`) do the same for the artist pass.
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
- Repeat and Shuffle live in the player. Repeat (`off`/`all`/`one`, `repeat.ts`) is the player's
  mode and outlives a queue; an endless shuffle skips `all`. Shuffle belongs to the queue: the
  player's own (`Queue.shuffled`, a seed and the queue's own order to go back to, `shuffledOn`/
  `shuffledOff`) or a session's (`Queue.shuffle`), which turning off leaves for its scope's order
  from the song playing (`Shuffles.toggle`, `scopeOrder.ts`). A queue going on through the Library
  (`continues`) is the exception: Shuffle takes up a session of what the Library's own Shuffle
  shuffles under its filter (`Shuffles.throughList`, set by `libraryView`), with the song playing as
  `first` (`takenUp`), and off goes back on through the Library from the song playing then
  (`backOnList`), its sort and filter as shown; a session restored from a record goes off into its
  scope's order. Both join the playback record
  (`repeat`, `shuffled`); one saved before them is off and off, and `shuffled.order` counts against
  the 1,000. A press reorders in memory and never stops the song; the save follows. The island is
  one Tab stop (`roving.ts`), and the player's keys are `playerKey` in `lib/player/keys.ts`.
- There is one tag editor (`tagEditor.svelte.ts`), which the shell draws over whatever page opened it
  (`EditorHost`): Edit tags, the album and song menus and a selection's Edit all open it, and the album
  page never edits in place. Its session holds the songs, the ticked ones, a draft by song and the
  shared fields over the ticked (`lib/library/edit.ts`), and writes through `POST /albums/{id}/edits`
  when the songs that change are on one album, `POST /edits` otherwise. A match only fills its fields.
  It has a shallow history entry of its own (`editorHistory`): Back and Esc close it and drop its
  edits, as Leave it does; closing it any other way takes the entry back off; leaving the page drops
  the edits too. Edits never outlive their page.
- The Trash's controls (the row and album menus' last item, the selection bar's button, Delete, the
  Server page's link and Server › Trash) show only when `library.user?.canTrash`. Trashing is
  optimistic (`trashBin.svelte.ts`): the rows fold and leave every list and the queue (not the song
  playing, which plays out) before `POST /admin/trash` is sent, and come back if it fails; the
  toast's Undo restores exactly what it moved. Every list reads through `Library.trashed`
  (`trashed.svelte.ts`), which hides those songs until the `library` event's reads land, and with
  nothing on its way hands back the server's own object. Nothing on the play path waits on any of it.
- `library.playlists` (`playlists.svelte.ts`) holds the person's playlists: the list read with the
  shell, each one as its page opens, and what they hold of songs being added (`askHolds`). Every
  change is optimistic (`playlistEdits.ts` works it out, pure) and each playlist's saves go one after
  another; a `playlists` event reads again what it names once nothing of this browser's is in flight,
  and a read begun before a change made here is let go. `playlistActions` is the one way songs go in
  and out from a menu or a page, with its toast (Add it again, Undo exactly what it did). The picker
  (`PlaylistPicker.svelte`, its list `PlaylistList.svelte`) is a submenu beside a ⋯ menu, a popover
  over the selection bar, and on a phone the menu itself reshaped into the list, never a sheet. A playlist's rows (`PlaylistSongs.svelte`) are keyed by entry, placed in a window
  over the list, and a grip moves one (held first by a finger). Play is a `list` queue
  (`player.playList`). The nav has no room for Playlists under 431 px: the You page links to them.
- A smart playlist's page reads `GET /me/playlists/{id}` as a regular page does and reads its rules
  back as a sentence from `lib/library/smartlist.ts`, the client's twin of `internal/smartlist`'s table
  and its words. Its songs come in the order the rules give: no grip, Alt+arrows, Del or Take out, and
  after the album up to three columns the rules, limit and order read (`ruleColumns`). Add to playlist
  never lists a smart one. After a `library` or `marks` event, or this browser's own play, smart pages
  and the list read again 400 ms later (`Playlists.smartMoved`); the server sends no event for that.
- The rules editor (`SmartEditor`) opens in place of the head and the songs, never a modal. Its draft
  (`smartRules.svelte.ts`) has a history entry of its own through `editorHistory`: Back and Esc are
  Leave it, and the draft never outlives the editor. `editorHistory` leaves another live editor's
  entry alone (`claimed`), so the tag editor and the rules editor share a page. A new smart playlist is
  `/playlists/smart`, a fresh draft that makes nothing until Save rules. The live card asks
  `POST /me/playlists/preview` through `LivePreview`, 120 ms after the last change, aborting the one
  before and keeping its last answer until the next arrives, so it never blanks. A new draft sends a
  seed it drew itself and an edit the playlist's `id`, so the card shows what saving would. The
  fixtures work smart playlists out in `lib/library/smartEval.ts`, so e2e needs no server.
- File playlists sit in `library.playlists` beside a person's own (`source: 'file'`), in their own
  From files section of the Playlists page, marked by `FileTag.svelte` (the dog-ear and the paper tag,
  on `--paper` tokens), never by SMART's stamp; Add to playlist never lists them. Their page reads
  like a smart one's (no grip, Alt+arrows, Del or Take out, no rename) with the FROM line, the
  not-found pill and its card (`MissingLines.svelte`); their words live in `fileWords.ts`. Copy to a
  playlist lands on the copy. Export (`PlaylistExport.svelte`) opens on any playlist's ⋯ in place of
  the buttons: Download is a plain link to the `m3u` route, and Save in the library (an admin with
  `canTrash`, never on a phone) uses the folder picker limited to the roots and the overwrite confirm.
- Server › Organize (`/server/organize`, `lib/organize/`) previews, runs and undoes. Its template field
  (`TemplateField.svelte`) draws the template as chips with the client's copy of the parser
  (`template.ts`, held by vitest to `internal/organize/testdata/templates.json`), and the live example
  draws three songs from `GET /admin/organize/examples` with it; the server parses again and is the
  authority. The preview pages 200 albums at a time and searches on the server; a run's head follows
  the `organize` event (`Library.hearOrganize`), and a reload finds a running run in `GET
  /admin/organize/runs`. The nav says ORGANIZING · n% while one goes. `ToolTabs.svelte` heads both
  pages (Organize · Duplicates); a conflict card's Compare in Duplicates opens the group its `group`
  names (`openDuplicates`). The album ⋯ and the selection bar's Organize (More on a phone) show
  with `canOrganize`: a scoped preview, then the page, or a toast when nothing would move. Nothing is
  optimistic, and nothing on the play path waits on it.
- Server › Duplicates (`/server/duplicates`, `/server/duplicates/[group]`, `lib/duplicates/`): the list
  under its filter chips (pairs first, then groups), a group's copies side by side (`CopyCard`, up to
  four across, stacked keeper first on a phone), and the confirm (`MergeConfirm`: in place, on a phone
  too) worded from the server's plan (`words.ts`). The merge sends back the plan it showed; on a 409
  the confirm shows the new one. The list hides a merged group at the press and brings it back if the
  merge fails (`duplicates.svelte.ts`); the toast after it links to the Trash. Play on a copy plays that
  song alone (`queuedCopy`), fetching nothing. The fixtures (`lib/duplicates/fixture.ts`) are the
  boards' groups and the Blackstar pair; a merge sends their copies to the fixture Trash.
- ReplayGain in the files (`lib/replaygain/`): `ReplayGainCard.svelte` on Health, beside MusicBrainz,
  for admins, and the album ⋯'s "Write ReplayGain…" with its confirm (`GainConfirm`, the menu's own box
  reshaped, at a phone's whole width on a phone), for `canTrash` and an album with measured songs. `library.replayGain` (`gain.svelte.ts`)
  outlives the page: it reads `GET /admin/replaygain` at sign-in (so a reload finds a run going),
  follows `replaygain` events (`Library.hearReplayGain`) and the run's albums, and keeps the album
  toast (`GainToast.svelte`) with its Undo. The nav says REPLAYGAIN · n% in Organize's slot. Words live
  in `words.ts`; the fixtures (`fixture.ts`, `?gain=` and `?gainAlbum=`) run on a timer. Nothing is
  optimistic, and nothing on the play path waits on it.
- Journeys (`lib/morph/`) keep the In-Front Rule: the page you go to is always drawn in front of the
  page you leave, and nothing leaving moves away to uncover it. The ghost layer is the first thing in
  the shell, under the stages and the page, and a stage left is held there as a picture. A ghost is a
  picture with no `main` of its own, so code that wants the page on screen asks `pageMain()`.
  Everything a page shows belongs to a `data-morph-part` unit or comes and goes as loose content; a
  shared element registers with `use:morph`, an element known by two names with `also`. An e2e test
  that reads a page's geometry after a navigation waits for `landed` (`e2e/fixture.ts`) first: the
  units arriving are still moving. An album that starts playing goes first in Home's Recently played
  at once (`Library.showQueue`), so Album → Home lands where Home holds it; the next read agrees.
- The nav's eight rooms (`rooms` in `choreography.ts`) unfold into each other from what was pressed
  (`Journey.origin`: the link clicked, else the room's `data-room` place in the nav). A room's
  `data-morph-headline` unit, the first on screen, is born there; everything else gathers around it.
  Into Search the nav's field reshapes into the page's in the born layer, under the nav's glass. A
  room waits one frame unseen for what it already holds to be drawn, never for the server. The same
  item pressed again only presses the pill, and lays a scrolled room again at its top (`relay`).
  Only an item holds the pill, never the fox.
- Every page the island shows on opens into Now playing from the island's cover (`into-now-playing`;
  from an album, its hero when on screen), and the door opens on it after a session ended there.
  Every way out goes back into the hand (`back-into-the-hand`): the poster lands where the page now
  holds the playing album in view (`heldInView`: a card clear of nav and island, Home's Continue card
  first, or its own hero), else in the island's thumb, and nothing is scrolled for it. Play and the
  bar fly back with the controls' pill; the rest of the island comes up in place. Into its artist the
  name under the song leads. The nav comes back where it stands, a fade (`data-returning`).
- Surfaces in the page (menus, confirms, pickers, toasts, the selection bar, the editors, the match
  sheet, the folder picker, Export, MissingLines) move through `lib/morph/surface.ts`, never their own
  keyframes or `--curve-settle`. A surface is one box at its true size and radius: it is born at the
  control just pressed (`watchPresses` reads a control's place on the press only when it opens
  something or sits in a `[data-surface]`; mark one that is replaced by what it opens `data-buds`), or
  at the floor (`floor`) when none is left on screen, and grows into itself on `--morph-fast`
  (`--morph` for the window-sized editors, `big`); its words never move: the engine's own box draws
  its look while the element is clipped to it, and at rest nothing of the engine is left. A new face
  is the same box reshaped (`hold` before the change, `reshape` once placed: `Floating`'s `face`,
  `reshapeOn`), the old words a picture giving way. It leaves by giving way where it stands, as one
  layer prepended to its parent so whatever opens next is in front; nothing goes back into its control.
  Closing an editor lays the page again in front of it (`Morphs.layAgain`, `laidAgain`). One opened
  again while it gives way (`key`) takes over from where that had got to. No scrims but the folder
  picker's backdrop; no sheets on a phone (`Floating`'s `full`). Reduced motion is a 120 ms blend with
  shapes set at once. `Floating` sizes take the page's `--step` (`zoom` on its faces). Nothing on the
  play path reads layout for this: a press on any other control costs a `closest`.
- The Server, Trash, Organize and Duplicates are rooms too, and a duplicate group and Matching are
  their records (`serverRecords`): every pair among them and the nav's rooms unfolds from the press in
  the page (a row, a crumb, a tab, a button; a control whose code navigates counts for 1.5 s, and a
  page with nothing left to press says where with `unfoldFrom`). Any page into Organize unfolds too.
  `RouteName` keeps each apart, so a new Server page needs its own name in `routes.ts`. Each one's
  `h1` is its `data-morph-headline` unit. What both pages say travels instead of a headline being
  born, and the page gathers around where it lands (`Journey.pivotOn`): a group's title (`title:<key>`,
  its row ↔ its headline) and Matching's number (`count:matching`, Health's card ↔ the headline),
  each an inline-block of the words alone; the page they land on scrolls it clear under the picture
  left. To land on something it must hold it as it mounts: a group is read on pointerdown and kept
  (`duplicates.known`), the queue as last read (`knownQueue`). `ToolTabs` is one strip (`kind:
  'strip'`, `StripFlier`): the page's own from the first frame, never in the air nor giving way, its
  fill flowing to the tab pressed. A group to its pair is re-pressed in place. Matching → Album is the
  album's door with the hero born at the link (`fromPress`).
- The door closes on every way to `/login` (`door-closes`): the fox flies from the nav to the door as
  its own mark scaled (`Journey.dress`), the frame (the nav's glass, the island, a stage) dissolves
  where it stands as a picture (`holdFrame`, `data-morph-held` on the bar) on `--morph`, the page's
  words go on `--morph-fast`, and the door gathers around the fox. On Now playing the Play button stands
  in for the fox (`standIn`). Sign out says "Signing out…" (`Repress`) until the server has answered.
  Login → any page `next` names is Login → Home's door (`login-to-page`). In the fixtures a session
  ends on `dispatchEvent(new Event('kitsune:session-ends'))`, and `?matching=board` draws a queue.
- Artist art (`lib/art/`): `art.ts` is pure (rungs per kind, `pictureUrl`, the initial, credit
  lines, the Artwork strip, `fallbackAlbum`). `Picture.svelte` lays a picture's ground, then develops
  it once decoded (`--morph-slow`, `saturate(0) contrast(1.6) brightness(0.5)` to none; opacity alone
  under reduced motion), never holding anything; a new hash develops over the old. `ArtistThumb` is
  round everywhere (the Artists list, Search's hits and Top result, the album's artist line, Now
  playing's name, the hero's head), the initial on `initialColors` with none, registered as
  `face:<artistId>`, the lead that travels with `name:`. A thumb with no read of its own comes from
  `library.artistThumb`, which `wantThumbs` fills from `GET /artists` once, in the background.
- An artist's stage (`ArtistStage.svelte`, `stage.svelte.ts`) is the shell Backdrop's third stage,
  `data-morph-stage="artist"`, keyed by artist: on their page the backdrop under the name (its logo at
  the top right, off register), else the stage of the album they play most (`heroStage`); under 431 px
  the square thumb (`--artist-stage`, set on `.shell`). On Now playing it is the playing artist's
  backdrop with the cover's light on it (`.poster.lit` then steps aside); going to the artist the same
  element stays and its height reshapes: the engine leaves a stage a journey names but doesn't hold,
  and that is already on screen, be (`stagesStaying`). The `artists` event re-reads an open artist
  and the Artists list; the fixtures' `?art=late` holds Low Tide Choir's art for `kitsune:art-arrives`.
  Album extras: the disc out of its sleeve behind the cover and Now playing's poster (@560, still,
  `translate` only), the Artwork strip after the songs, a Cover Art Archive credit under the cover.
- Nothing is cued: `library.playing` follows the player, which starts empty until something plays or
  `resume` restores the record, paused. Saving (`lib/player/resume.svelte.ts`) never waits on the play
  path and only writes once this browser has changed something.
- `app.html` sets `data-theme` before the first paint, from `localStorage['kitsune.theme']` or the
  system, by the same rule as `lib/theme`'s `resolveTheme`: change them together. The person's choice
  is the `ui.theme` pref, read after sign-in and forgotten at sign-out.
- An open tab learns of a new build by SvelteKit's version poll (`version.pollInterval` in
  `web/vite.config.ts`). The next navigation loads it whole, but only when nothing plays and the
  server holds where the player is (`lib/shell/freshness.ts`): `reloadsInto` reloads a paused, saved
  player, which comes back after the load. The server serves `_app/version.json` uncached for this. A tab left open across a deploy can't fetch the old build's files: the shell fetches every route's code and the tag editor's when it starts (`preloadCode`, `editorCode`), so anything new that is loaded lazily joins them there.

## Speed
- Every interaction has a p95 budget in `perf/budgets.json`, the one file that holds them. The browser
  suite (`web/perf`, Playwright) times clicks, keys and page loads in a release build against generated
  libraries of 1,000 and 50,000 songs, 20 runs each; `internal/server`'s `*Speed` tests time the
  stream's first byte, `GET /library/index`, `GET /me/home`, a 10,000-entry `GET /me/playlists/{id}`,
  a smart playlist's preview, a `GET /me/playlists` of ten smart ones, a 10,000-song `.m3u8` read
  against 50,000 songs, a quick scan with 50 unchanged playlist files, held to the same scan
  without them, an organize preview of 50,000 songs, an organize run of 1,000, per song, a find of duplicates over
  50,000 songs after a fingerprint landed, a ReplayGain run of 1,000 real FLACs, per song,
  `GET /admin/replaygain` over 50,000 songs with 2,000 to write, an artist's page with all four
  images and a bio, `GET /artists` of 2,000 artists with thumbs, a quick scan beside unchanged
  art, held to the same scan without it, and the artist pass over one artist against the stand-ins
  (`scrape-artist`: its JSON, choices, a 4K background and three more images rendered, one write). The
  browser suite also holds that a song plays on while its file moves. Each fails past its budget,
  and CI runs both on every PR: the speed tests in their own job, the browser suite as four shards (each
  library in two, the files split by time in `web/perf/shards.ts`, one worker each), and the `perf` job
  gathers every shard's samples into one table in its summary and fails when any of them did. Budgets are about 1.5x what was measured when
  they were set; on CI they are multiplied by `ciMultiplier`, which comes from CI's own numbers.
- `mise run perf` runs everything (`scripts/perf.sh`; Playwright arguments pass through). It writes
  only under `$KITSUNE_PERF_DIR` (default `kitsune-perf` in the temp directory) and reuses the
  libraries there. Each server starts on a fresh database, and nothing is timed until its first scan
  and the fingerprint and loudness passes after it are done: the suite measures a server at rest.
  Only the loud albums lack ReplayGain, so the loudness pass has little to do. `KITSUNE_PERF_CHROMIUM` picks a local Chromium, `KITSUNE_PERF_LIBRARIES=1k` measures
  one size, `KITSUNE_PERF_SHARD=1/2` runs one shard of the files, `KITSUNE_PERF_VERBOSE=1` prints every sample, and `KITSUNE_PERF_PORT` moves the servers off 14610, for a second worktree measuring at once. The speed tests skip under `-race`.
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
- The web client's behavior is tested in a browser by `web/e2e` (`mise run web:e2e`), on the client's
  fixtures (`?source=fixtures`), with no server. Any page error fails the test. A bug a rendered page
  shows gets its regression test there; a pure function's goes in vitest.

## Commits and docs
- Gitmoji prefix, single line, no body, no attribution trailers.
- Plans, specs and design notes never go in this repository.
