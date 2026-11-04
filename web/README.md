# Kitsune web client

A SvelteKit single-page client. It runs on fixtures for now, with no server.

    mise install          # the pinned Bun
    mise run web:dev      # dev server
    mise run web:build    # static build in web/build
    mise run web:preview  # serve that build
    mise run web:check    # lint, svelte-check, tests, build

Bun is the package manager and script runner. Tests run under Vitest (`bun run test`,
never `bun test`).

## Native binaries

The web toolchain ships native `.node` binaries (rolldown, lightningcss). They are build
tooling for the client and never reach the server binary, so Kitsune's pure-Go rule does
not apply to them. The client that ships is static HTML, CSS and JavaScript.

## Fonts

Geist and Geist Mono are self-hosted through Fontsource. The client makes no runtime
requests to third parties: it ships inside a self-hosted server.
