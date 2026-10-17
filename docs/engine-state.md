# Frontend engine state

`apps/electron/lib/engineState.ts` is a pure reducer from validated wire messages to the React view. Socket commands and reconnection belong to `EngineClient`; local preferences belong to `usePreferences`.

## State

`EngineState` holds connection status, workspace, session metadata, selection, permission mode and errors. Its `conversations` map contains one record per session:

- `blocks`: user, assistant, reasoning, tool and error output.
- `runId/running`: the current run and its liveness.
- `usage`: token totals for that run.
- `pending`: one typed question or permission envelope, or null.
- `outcome`: the terminal result, or null.

The selected view comes from `activeConversation(state)`. Components receive this record and the original session metadata.

## Updates

`reduce` handles connection status, selection, reset and local errors. Server messages pass through a typed switch:

1. Workspace changes reset all conversation records while retaining connection status.
2. Messages from another workspace are ignored.
3. Live deltas and interaction requests require a matching, running session.
4. Terminal outcomes require the current run ID. Interaction resolution also requires the pending interaction ID.

Session selection is published by the client after its snapshot is acknowledged. Background snapshots do not select sessions. The client refreshes session metadata after terminal events.

## Snapshots and streaming

A snapshot builds a fresh conversation from history, replays current-run events, and applies its pending interaction and outcome. Repeated snapshots replace the record atomically.

History is projected in message order so reused tool-call IDs retain the correct arguments. Live tool blocks match both run and call IDs; parallel results update their original block. Text and reasoning fragments append to the last matching block. Unknown delta types leave the view unchanged.

All transitions preserve the previous state. The transcript owns scrolling; the page composes views and issues commands through `useEngine.run`.
