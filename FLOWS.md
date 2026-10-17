# Execution flows

Both standalone CLI and WebSocket commands enter internal/chat.Coordinator. See [Architecture](ARCHITECTURE.md) and [transport flows](docs/flows.md).

1. Acquire the workspace store lease, reserve the session, and load history once. Corrupt history fails preparation.
2. Enqueue the starting snapshot for subscribers. Construct the runner at the composition root and execute tools outside the ordering mutex.
3. Update the projection and enqueue events under the same boundary used for snapshots. Each subscription carries its immutable workspace.
4. A valid interaction reply matches workspace, session, run, interaction and kind; resolution is queued before execution resumes.
5. Cancellation before commit prevents saving. Once commit starts, shutdown waits for its actual result.
6. Publish succeeded, failed, canceled or persistence_failed. Only persisted success confirms durable history.
7. Retain the latest terminal projection within the process budget. Reconnect includes its outcome and unsaved availability; restart cannot recover unsaved output.

CLI without a session uses ephemeral execution. Remote CLI authenticates, waits for a correlated start snapshot, and cancels that exact run. Desktop retains drafts until acknowledgement and restores workspace and subscriptions before enabling commands.

Desktop authenticates the configured engine or starts its packaged binary in a writable workspace. Closing desktop drains only its own child engine through the parent pipe.
