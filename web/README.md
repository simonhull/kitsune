# Kitsune web client

A SvelteKit single-page client. It runs on fixtures for now, with no server.

    mise install          # the pinned Bun
    mise run web:dev      # dev server
    mise run web:build    # static build in web/build
    mise run web:preview  # serve that build
    mise run web:check    # lint, svelte-check, tests, build

Bun is the package manager and script runner. Tests run under Vitest (`bun run test`,
never `bun test`).

## The morph lab

Page transitions are morphs: a persistent overlay of flying elements, run on springs from the
choreography in `src/lib/morph/choreography.ts`. Pages only register shared elements with
`use:morph`.

- `?latency=300` delays every fixture response, to prove no first frame waits for data.
- `?playing=<album id>` plays another album, for the path where a hero condenses into the island.

In dev, a small HUD at the bottom left shows how long the last morph took to start.

## Native binaries

The web toolchain ships native `.node` binaries (rolldown, lightningcss). They are build
tooling for the client and never reach the server binary, so Kitsune's pure-Go rule does
not apply to them. The client that ships is static HTML, CSS and JavaScript.

## Fonts

Geist and Geist Mono are self-hosted through Fontsource. The client makes no runtime
requests to third parties: it ships inside a self-hosted server.
