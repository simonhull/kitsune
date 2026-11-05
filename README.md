# Kitsune

A self-hosted music server with a native API, an OpenSubsonic compatibility layer,
a web UI and a terminal client.

**Status:** early rewrite. The previous Subsonic terminal client is preserved at the
`legacy/subsonic-tui` tag.

## Configuration

`kitsune-server` reads `$XDG_CONFIG_HOME/kitsune/server.toml` (normally
`~/.config/kitsune/server.toml`), or the file named by `-config FILE`. Every key is
optional; these are the defaults:

```toml
[server]
listen = ":4533"         # host:port
base_url = ""            # public URL when behind a reverse proxy, e.g. "https://example.com/music"
trusted_proxies = []     # proxies whose X-Forwarded-For is believed: CIDRs or addresses

[library]
roots = []               # absolute directories to scan; they must not overlap
missing_grace = "24h"    # how long a vanished file keeps its track
quick_scan = "03:30"     # daily quick scan, local time; it also runs at startup

[storage]
data_dir = ""            # "" means $XDG_DATA_HOME/kitsune-server

[log]
level = "info"           # debug, info, warn or error
```

Unknown keys are errors, so a typo cannot be silently ignored. Paths must be
absolute. `kitsune-server config` prints the configuration in effect.

## Running

Create an administrator first, then start the server:

    kitsune-server users add -admin NAME
    kitsune-server serve

`serve` refuses to start while the database has no users, and prints the command above.

`serve` keeps the database in step with `library.roots`: a quick scan at startup and another
daily at `library.quick_scan`. The first scan reads all the audio to fingerprint it, which on
a large library takes a while. Tracks appear as it goes, and `GET /api/v1/readyz` answers 503 until
it finishes, so point health checks that restart containers at `GET /api/v1/healthz`. A moved file
keeps its track; a vanished one is deleted after `library.missing_grace`. A root that is
unreadable or suddenly empty is skipped, so an unmounted disk never deletes anything.

### Album covers

The scan renders each album's cover once, from the first of: an embedded front cover, any
other embedded picture, or `cover`, `folder`, `front` or `album` (`.jpg`, `.jpeg`, `.png`,
`.webp`) in the track's folder or the parent of a disc folder. Renditions at 128, 512 and 1024
pixels (never upscaled) go to `<data_dir>/artwork`, so `GET /api/v1/artwork/{hash}?size=` is a file read,
cached for a year. The cache is derived data, but it is only rebuilt when an audio file changes:
a replaced `cover.jpg` is not noticed, and if you delete the folder you need a new database.

Albums have public IDs, like tracks. A known limit: an album's ID is not stable across a move
or a retag, because its identity is its folder, name and artist. A moved album gets a new ID and a
fresh added time, while its tracks keep theirs.

Logs are JSON on stderr. `log.level = "debug"` adds one line per request; it records the
path only, never the query. `GET /api/v1/healthz` reports that the process is up, and
`GET /api/v1/readyz` that it is ready to serve. On SIGINT or SIGTERM the server stops accepting
connections and drains in-flight requests for up to 8 seconds; a second signal quits at once.

### Browsing the library

Every route is under `/api/v1` and, except login, needs a session. `api/openapi.yaml` is the contract.

- `GET /albums` lists albums that have a live track, a page at a time: `sort=added` (the default,
  newest first) or `sort=name` (ignoring case), `limit` 1 to 200 (default 60), and `cursor`, the
  previous page's `nextCursor`. Paging is by keyset, so albums added between pages never repeat or
  skip one. A cursor belongs to the sort that issued it; a bad one is a 400. Each entry carries what
  a page transition needs: title, artist, track count and the cover's hash and colors.
- `GET /albums/{id}` returns an album with its tracks, in disc and track order (an unset disc counts
  as disc 1), its total duration and its formats.
- `GET /library` counts live albums and tracks, their duration, the configured roots and the files
  the scanner could not read.

- `GET /tracks/{id}/stream` serves a track's original file, with no transcoding. It takes single and
  multiple byte ranges, and its ETag comes from the file's size and modification time, so a retag
  that keeps both keeps the ETag. `If-Range` matches an ETag only. A missing track, or one whose file
  has left the disk, is a 404.

An album with no live track is left out everywhere.

### Behind a reverse proxy

- Set `base_url` to the public URL, such as `https://example.com/music`, and
  `trusted_proxies` to the proxy's address or network. Kitsune believes
  `X-Forwarded-For` only from those peers, which keeps the login rate limits per client
  rather than per proxy. An `https://` `base_url` also marks cookies `Secure`.
- Kitsune serves the whole API under `/api/v1`, after the path of `base_url` (so
  `/music/api/v1/healthz`); everything outside is a 404. The session cookie stays at the base path, or `/`.
  The proxy must pass the
  path through unchanged. In Caddy use `handle /music/*`, not `handle_path`. In nginx use
  `location /music/ { proxy_pass http://127.0.0.1:4533; proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for; }`
  with no URI after the port. Caddy sets `X-Forwarded-For` by default; nginx does not.
- After you add a base path, a browser may keep an old `kitsune_session` cookie at
  `Path=/` until it expires. Signing in again works; clearing the site's cookies tidies it up.
- Keep `trusted_proxies` narrow: a client inside a trusted network can claim any address.

## Development

    mise install
    mise run check
