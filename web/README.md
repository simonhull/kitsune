# Kitsune web client

A SvelteKit single-page client of the native API.

    mise install          # the pinned Bun
    mise run web:dev      # dev server
    mise run web:build    # static build in web/build
    mise run web:preview  # serve that build
    mise run web:check    # lint, svelte-check, tests, build

Bun is the package manager and script runner. Tests run under Vitest (`bun run test`,
never `bun test`).

## Developing the client

The dev server proxies `/api` to `KITSUNE_API` (default `http://localhost:4533`), so the session
cookie is same-origin. Run a server beside it, with a `server.toml` naming your music:

    [library]
    roots = ["/path/to/music"]

    kitsune-server users add -admin NAME
    kitsune-server serve
    mise run web:dev

Add `?source=fixtures` to the first URL you open to run on the boards' fake library, with no
server and no login, for tests and clips. It holds until the next full page load.

On `/health`, `&health=<state>` opens on `idle`, `starting`, `running`, `clean`, `mixed` or `failed`,
and `&role=user` shows any of them as a non-admin. Scans from the buttons run at the real library's
pace.

## The morph lab

Page transitions are morphs: a persistent overlay of flying elements, run on springs from the
choreography in `src/lib/morph/choreography.ts`. Pages only register shared elements with
`use:morph`.

With `?source=fixtures`:

- `?latency=300` delays every fixture response, to prove no first frame waits for data.
- `?playing=<album id>` holds the cue on another album, for the path where a hero condenses into
  the island.
- `?scan=<seconds>&gap=<seconds>` runs a first scan of a 927-album library on a timer, so Home opens
  on the deck. `gap` defaults to 2.7: `?scan=412` is real tempo, about 6 albums a burst, and
  `?scan=30` is the same library as a dense run. A reload resumes the scan, and a new tab starts it
  again.

- `?editFault=<busy|stale|drift|refused|partial>` makes the first tag edit fail that way, for the
  states the editor's boards never drew.

In dev, a small HUD at the bottom left shows how long the last morph took to start.

## Native binaries

The web toolchain ships native `.node` binaries (rolldown, lightningcss). They are build
tooling for the client and never reach the server binary, so Kitsune's pure-Go rule does
not apply to them. The client that ships is static HTML, CSS and JavaScript.

## Fonts

Geist and Geist Mono are self-hosted through Fontsource. The client makes no runtime
requests to third parties: it ships inside a self-hosted server.
