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

## Development

    mise install
    mise run check
