# Electron to Wails behavior parity

The reference for the remaining migration decisions is Electron commit `90375169`, immediately before the legacy runtime was removed. Wails remains the only desktop runtime.

## Restored behavior

- **Automation scripts:** an embedded Goja JavaScript runtime executes control flow, functions, expressions and async/await. The `nct` API exposes terminal I/O, waits, dialogs, screen snapshots, logging, progress and session metadata. Recorded scripts continue to work. Node.js modules and dynamic code generation are unavailable. CPU-bound execution, pending host calls and logs are bounded; Stop interrupts the interpreter and cancels waiting host operations. Observer checks are enforced in the host, including computed API calls.
- **CodeBuddy elicitation:** the native process uses the SDK 0.3.230 stream-json control protocol. Initialization advertises form elicitation; create requests become existing UI cards; accept/decline/cancel responses return on stdin. Complete notifications, cancellation, duplicate requests and expiry settle pending cards. CLI tool calls remain subject to the LemonSSH host's approval and scope checks.
- **Tool integration modes:** Skills supplies the LemonSSH CLI with the chat scope; MCP injects the native MCP server in the client's configuration. Managed child credentials are pinned to their issuing chat and revoked when the turn ends. The external-MCP opt-in remains independent. Cursor/Grok workspace entries are leased and restored only if unchanged; an existing LemonSSH entry or simultaneous conflicting chat fails explicitly rather than overwriting configuration.
- **Copied session windows:** new connections belong to the creating window. Closing or expiring that window closes its owned terminals, including a connection that finishes after close. The source terminal is untouched. Attach/observe popups retain their existing restore-to-source behavior.
- **Tray panel:** 360 × 520, positioned next to the tray using Wails native positioning, with a six-pixel offset. Windows left-click restores the main window and right-click toggles the panel; macOS/Linux left-click toggles the panel. Focus loss hides the warm panel.
- **Plugin lifecycle:** `PluginService` already implements install, enable/disable and uninstall, including runtime shutdown and permission revocation. The unused duplicate lifecycle manager was removed; no pre-granted permission model was added.

## Release policy

The Electron application used `electron-updater`; Windows signing was optional while the SignPath application was pending. There was no production Ed25519 key to migrate. LemonSSH retains its existing optional manifest-signing support and checksum verification; this change does not create keys, change release secrets, publish artifacts or make platform signing a source gate.

## Verification boundaries

Automated coverage includes Go race tests, a real child-process CodeBuddy protocol fixture (not a live provider account), tool-mode/configuration tests, chat-token scope and revocation tests, JavaScript runtime regression tests and frontend adapter tests.

Windows desktop smoke testing used a separate profile: the packaged app started, the copied local-terminal window opened and closed without disconnecting the source, and a script containing an array, a for loop, computed sendLine arguments and await produced PARITY_1 / PARITY_2 / PARITY_3 in the real local terminal with a Completed run status. The test instance was closed afterwards.

The full frontend suite passed with --test-concurrency=4 (7,457 passed, 41 skipped). Its default parallel run still hit existing wall-clock limits in the two 50,000-entry SFTP history/resume tests; the time limits were not relaxed and both tests passed in isolation. Go full tests and targeted race tests passed. The Wails Windows artifact and portable ZIP were built. NSIS was skipped because makensis is not installed.

Live provider authentication, remote account behavior and native tray placement still require target-environment smoke tests. Linux/macOS installer validation requires those operating systems. Unit tests do not substitute for those checks.
