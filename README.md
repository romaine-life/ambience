# ambience

Shared-world ambient pixel-art effects. A server-authoritative simulation broadcasts a pixel grid to browser and terminal clients via SSE; every consumer sees the same frame at the same moment.

Canonical demo: `ambience.romaine.life` (once deployed). Meanwhile: `go run ./cmd/server` then open <http://localhost:8080/>.

## Quick start

```sh
go run ./cmd/server
# open http://localhost:8080/
```

The root URL renders the current effect full-screen in your browser. The same `/ambience.js` module + `/stream` SSE endpoint will be embedded by consumer apps (my-homepage, fzt-automate) later — every consumer is a thin renderer on top of the server's shared state.

## Layout

```
sim/          Pure-Go simulation logic (testable standalone)
cmd/server/   HTTP server binary: SSE, entropy POST (future), static assets
  web/        Demo index.html + ambience.js (canvas renderer), embedded
```

## Status

MVP scope: the Rain effect, no entropy input yet, no persistence. See [issues](https://github.com/nelsong6/ambience/issues) for the roadmap.
