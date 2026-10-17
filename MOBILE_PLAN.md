# Mobile control plan

Mobile is a future client of the owner's existing engine. No mobile application is implemented here.

Reuse the transport-independent desktop EngineClient and protocol decoder, supplying a native socket factory and platform credential adapter. Keep React and Electron APIs out of the shared client.

The engine owns workspace files, tools, provider credentials and atomic JSON history. Persistent work requires an OS lease. Each client connects to one owner's engine; there are no user accounts or username/password login.

Pair with an engine URL and owner token; send the token in the initial auth envelope and require run-lifecycle-v1. Use a private encrypted network or private TLS gateway remotely. Discovery and cloud relays remain future work.

Restore confirmed workspace, settings, sessions and desired subscriptions before enabling commands. Scope state by endpoint/workspace/session and validate run and interaction identities. Preserve drafts on failed or uncertain delivery; never automatically replay mutations.

Future mobile acceptance: simultaneous desktop/mobile subscriptions, first-valid interaction reply, reconnect during a run, exact-run cancellation, failed-save display, and unavailable in-memory projection handling. The shared protocol fixture and lifecycle tests supply the contract.
