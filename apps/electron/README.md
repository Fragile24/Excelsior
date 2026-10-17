# Excelsior Desktop

Electron loads a static Next.js frontend and connects to the personal Go engine over WebSocket. The Go engine owns work and history; the UI can reconnect to an active run.

## Frontend ownership

`EngineClient` owns the socket, correlated commands, selection and reconnect restoration. `useEngine` connects it to React through one command executor. The pure reducer in `engineState` keeps one conversation record per session: blocks, run identity, usage, pending interaction and outcome. Snapshots replace complete records; live updates must match both workspace and run. Tool results match their call IDs, including parallel calls to the same tool.

`page` composes the UI from that state. The sidebar receives session metadata and workspace paths directly, while workspace names are derived from paths. `usePreferences` owns local theme and workspace history. The transcript owns scrolling, and the settings dialog uses native modal focus and Escape handling.

## Development

From the repository root, build the engine:

```sh
go build -trimpath -ldflags "-s -w" -o excelsior.exe ./cmd/excelsior
```

In separate terminals, run `npm run dev:engine` and `npm run dev` from `apps/electron`. Development does not auto-start an engine unless `EXCELSIOR_AUTO_ENGINE=1`. Local desktop authentication loads the owner token through the preload bridge.

## External engine / future mobile access

Run `excelsior engine --workspace <project>` independently. Set `EXCELSIOR_ENGINE` to its WebSocket URL before launching Electron; this disables auto-spawn. Closing desktop then leaves the external engine and its runs alive.

For a remote engine, set `EXCELSIOR_ENGINE_TOKEN` or paste its owner token into Settings. Manual tokens are scoped to the URL in browser session storage. Use an encrypted private network or a private TLS gateway. Add the client origin with the engine's `--origin` flag if needed. Mobile UI is not implemented yet.

Selecting a session restores its history, partial output, and pending question/approval. Either connected device can answer; stale replies are rejected. Escape dismisses a pending interaction or cancels the active run if no dialog is open. Permission settings are read from the engine and changed only on explicit user action.

## Packaging

```sh
npm run build:win   # Next.js static export and Windows installer
npm run pack       # unpacked application
```

The packaging hook builds the matching Go executable for each target and copies it into resources/engine. Use build:linux or build:mac for those native targets. Frontend assets are served from the packaged excelsior://desktop origin and are not embedded in Go. Packaged desktop auto-spawns the local engine unless external mode is selected. An engine spawned by Electron stops when Electron exits.

## Environment

- `EXCELSIOR_ENGINE`: external engine URL (default connection: `ws://localhost:17812/v1/ws`).
- `EXCELSIOR_ENGINE_TOKEN`: token for that configured engine.
- `EXCELSIOR_ENGINE_ADDR`: shared listener/client address, default `127.0.0.1:17812`.
- `EXCELSIOR_AUTO_ENGINE=0`: disable auto-spawn; `1` enables it in development.
- `EXCELSIOR_TOKEN_FILE`: shared Go token-file override in a dedicated configuration directory.
- `EXCELSIOR_WORKSPACE`: writable engine workspace; packaged default is under userData.
- `ELECTRON_START_URL`: development renderer URL override.

See the root [architecture document](../../ARCHITECTURE.md) for the protocol and process limits.

Run npm test and npm run lint for contract/client/native-trust checks. After npm run pack, run node tests/installed-smoke.cjs with the unpacked executable path. This uses a temporary workspace and local fake provider to check the real renderer, persistence and owned-engine shutdown.
