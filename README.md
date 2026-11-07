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
tag_writes = false       # let administrators edit tags, which rewrites files under roots

[storage]
data_dir = ""            # "" means $XDG_DATA_HOME/kitsune-server

[log]
level = "info"           # debug, info, warn or error

[lastfm]
api_key = ""             # from an API account at last.fm/api; set both or neither
shared_secret = ""       # never logged; `kitsune-server config` shows it as "(set)"
```

Unknown keys are errors, so a typo cannot be silently ignored. Paths must be
absolute. `kitsune-server config` prints the configuration in effect.

With `[lastfm]` set, each user can connect their Last.fm account on the You page, and the
server sends their plays, the song playing and their hearts as loves. Last.fm sends the browser
back to `/api/v1/lastfm/callback` at `server.base_url`, or at the address the browser used when
that is not set. Keep the secret out of committed files: fill it in with `op inject` or similar.

## Building

    mise run release

gives `dist/kitsune-server`: one static file with the web client inside it. A plain `go build`
gives a server with the API alone, for working on the client under `mise run web:dev`.

## Running

Start the server, then open it in a browser:

    kitsune-server serve

On the first start there are no accounts yet. The server logs a warning with a setup code and its
address, and every page of the web client goes to "It's yours.", where you make the first account:
an administrator, signed in at once. Once any account exists the page is closed for good.

From your own network (loopback, the private ranges, link-local and `100.64.0.0/10`, which is
Tailscale's) the page asks for nothing more. From anywhere else it also asks for the setup code
from the log. The code is held in memory only, changes at every start, and stops working once an
account exists. Five wrong codes make the page wait, as sign-in does. Behind a reverse proxy, the
client's address counts only through the proxies in `server.trusted_proxies`; any other proxy is
the client, and its address decides.

`kitsune-server users add -admin NAME` makes an account from the command line instead, for scripts
and recovery. It works while the server runs, and closes the setup page too.

The web client is at the root of `server.listen`. It only runs at the root of a host, so with a
path in `server.base_url` the server gives the API alone and says so at startup.

`serve` keeps the database in step with `library.roots`: a quick scan at startup and another
daily at `library.quick_scan`. The first scan reads all the audio to fingerprint it, which on
a large library takes a while. Tracks appear as it goes, and `GET /api/v1/readyz` answers 503 until
it finishes, so point health checks that restart containers at `GET /api/v1/healthz`. A moved file
keeps its track; a vanished one is deleted after `library.missing_grace`. A root that is
unreadable or suddenly empty is skipped, so an unmounted disk never deletes anything.

### In a container

`compose.yaml` builds the image and runs it. Beside it, write a `server.toml` with the library at
`/music`, and create the data folder:

    printf '[library]\nroots = ["/music"]\n' > server.toml
    mkdir data
    export KITSUNE_MUSIC=/path/to/music
    docker compose up -d

Then open `http://<host>:4533` and make the first account. From outside your network, the setup
code is in the log: `docker compose logs kitsune`.

A known limit: when Docker's userland proxy forwards a connection, as it can for a published port,
the server sees the bridge's address, which is private, so everyone counts as your network and no
code is asked for. That is fine for a server reachable only on a home network or a Tailnet. Before
exposing a fresh server to the internet, make the account first (open the page from home, or
`docker compose run --rm kitsune users add -admin NAME`), or put it behind a proxy listed in
`server.trusted_proxies`.

`KITSUNE_UID` and `KITSUNE_GID` name the user it runs as, 1000 by default, which must own `data`.
`KITSUNE_PORT` is the host port, 4533 by default, which is also Navidrome's: choose another to run
both. `TZ` sets the zone `library.quick_scan` is read in. The music is mounted read-only; to edit
tags, set `library.tag_writes` and drop the `:ro`.

### Album covers

The scan renders each album's cover once, from the first of: an embedded front cover, any
other embedded picture, or `cover`, `folder`, `front` or `album` (`.jpg`, `.jpeg`, `.png`,
`.webp`) in the track's folder or the parent of a disc folder. Renditions at 128, 512 and 1024
pixels (never upscaled) go to `<data_dir>/artwork`, so `GET /api/v1/artwork/{hash}?size=` is a file read,
cached for a year. The cache is derived data. A quick scan only looks at changed audio files, so a
replaced `cover.jpg` or a deleted cache folder takes a full scan.

Albums have public IDs, like tracks. A known limit: an album's ID is not stable across a move
or a retag, because its identity is its folder, name and artist. A moved album gets a new ID and a
fresh added time, while its tracks keep theirs.

Logs are JSON on stderr. `log.level = "debug"` adds one line per request; it records the
path only, never the query. `GET /api/v1/healthz` reports that the process is up, and
`GET /api/v1/readyz` that it is ready to serve. On SIGINT or SIGTERM the server stops accepting
connections and drains in-flight requests for up to 8 seconds, then closes what is left, such as a
stream a client stopped reading, and exits cleanly; a second signal quits at once.

### Browsing the library

Every route is under `/api/v1` and, except login and setup, needs a session. `api/openapi.yaml` is the contract.

- `GET /albums` lists albums that have a live track, a page at a time: `sort=added` (the default,
  newest first) or `sort=name` (ignoring case), `limit` 1 to 200 (default 60), and `cursor`, the
  previous page's `nextCursor`. Paging is by keyset, so albums added between pages never repeat or
  skip one. A cursor belongs to the sort that issued it; a bad one is a 400. Each entry carries what
  a page transition needs (title, artist, track count, the cover's hash and colors) and what the
  Albums page sorts and filters by: `sortName`, `sortArtist`, `addedAt` and `displayGenres`, an
  album's genres with commas split, ID3v1 numbers stripped and one spelling across the library.
- `GET /albums/{id}` returns an album with its tracks, in disc and track order (an unset disc counts
  as disc 1), its total duration, its formats, and both its raw `genres` and its `displayGenres`.
- `GET /library` counts live albums and tracks, their duration, the configured roots and the files
  the scanner could not read.

- `POST /admin/library/scan` starts a `quick` or `full` scan, for administrators. It answers 202, or
  409 while one runs. `GET /library` reports its progress.
- `GET /admin/library/issues` lists the files a scan couldn't use and the tracks it warned about,
  for administrators. Paths are relative to their root.
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
