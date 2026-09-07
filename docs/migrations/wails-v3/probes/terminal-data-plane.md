# P0-03 Terminal Binary Data-Plane Probe

Status: `needs-verification`

This is disposable feasibility evidence for the authenticated loopback
WebSocket candidate. It is not a production terminal owner, does not replace
the Electron MessagePort path, and must not be imported by production React or
Go code.

## Probe Boundary

- Source: `experiments/terminal-data-plane/`
- Framework: Wails `v3.0.0-beta.12`, Go 1.25, xterm `6.1.0-beta.292`
- Control plane: generated Wails typed bindings for session, route, snapshot,
  popup handoff, report, and autorun lifecycle calls
- Data plane: binary loopback WebSockets only; separate data and urgent routes
- Workloads: canonical Electron sustained-output fixture, long unbroken line,
  one million short lines, and metadata-only ingress
- Excluded: production PTY/SSH/SFTP, profiles, credentials, plugins, sync,
  updater, AI, and Electron owner changes

The listener binds `127.0.0.1` on an ephemeral port. Every route validates the
exact Host and an explicit Origin, then consumes independent cryptographically
random data and urgent tokens. Tokens are carried as one-use WebSocket
subprotocol values and are never logged.

## Protocol Candidate

Version 2 frames carry a magic value, kind, generation, sequence, credit cost,
payload length, correlation, and backend send timestamp. The implementation
enforces:

- binary frames and bounded 128 KiB payloads;
- monotonically increasing session generations and sequences;
- an initial zero-credit state and a 1 MiB receive window;
- credit return only after xterm processing, including explicit zero-payload
  metadata handling;
- retained replay bounded by outstanding credit;
- ordered drain before same-window rebind, reload, or popup movement;
- a separate urgent ETX route that is not blocked by output credit;
- one-use, bounded popup handoff and source-recovery state;
- lifecycle-owned cancellation, listener shutdown, and autorun timeout.

The 1 MiB window is copied from the Electron fixture as a comparison input. It
is not a newly approved production threshold.

## Automated Evidence

The root check performs a clean frontend install, protocol/controller tests,
frontend production build, Go tests, race tests, and vet:

```bash
npm run check:terminal-data-plane-probe
```

Focused coverage includes malformed frames, Host/Origin/token rejection, token
replay, zero-credit pause/resume, metadata-only credit, urgent ACK during a
stall, stale generations, exact reconnect replay, ordered drain, popup handoff
and recovery races, cancellation, bounded 1/4/8-session runs, memory/report
bounds, and autorun outcome/timeout/exit behavior.

The frontend harness records bounded raw evidence for backend-to-WebView and
backend-to-xterm latency, xterm callback latency, frontend queue high-water,
credit, pause/resume, urgent renderer RTT, route interruptions, payload digest,
and explicit start/steady/complete/settled memory phases. Process RSS is
available on Windows; unsupported samples remain explicit rather than being
filled with synthetic values.

## Windows WebView2 Smoke

Recorded on 2026-08-31 using Windows 10 22H2 x64 build 19045, WebView2
`149.0.4022.52`, Wails `v3.0.0-beta.12`, and Go `1.25.0`.

The native executable was launched with:

```powershell
$env:NETCATTY_TERMINAL_DATA_PLANE_AUTORUN='1'
terminal-data-plane-probe.exe
```

The autorun used a real Wails WebView2 and xterm instance. It stalled credit
before starting output, received an urgent ETX ACK while stalled, resumed,
performed an ordered same-window rebind, processed the complete canonical
Electron workload, settled resources, and exited with code 0.

```json
{
  "workload": "sustained",
  "sessionCount": 1,
  "chunkCount": 1600,
  "payloadBytes": 9708106,
  "creditBytes": 9708106,
  "frameCount": 1600,
  "sequenceIntegrity": true,
  "byteIntegrity": true,
  "creditIntegrity": true,
  "digestIntegrity": true,
  "urgentRendererRttMicros": 300,
  "generation": 2,
  "rebindCount": 1,
  "frontendQueueHighWaterBytes": 289255,
  "backendOutstandingHighWaterBytes": 923593,
  "memoryStart": true,
  "memorySteady": true,
  "memoryComplete": true,
  "memorySettled": true,
  "totalDurationMillis": 3284
}
```

One WebView2 shutdown diagnostic reported failure to unregister
`Chrome_WidgetWin_0` with Windows error 1412 after the success marker. The
process still returned 0; formal repeated rounds must determine whether this is
benign teardown noise or a cleanup defect.

## Exit Assessment

The loopback WebSocket candidate is feasible on this Windows host and advances
`TERM-01` to `probe`. It is not selected as the production data plane yet.
P0-03 remains `needs-verification` because the following evidence is absent:

- three warm-ups and 30 measured rounds paired with the accepted Electron
  envelope on the same hardware;
- WKWebView and WebKitGTK runs on required macOS/Linux targets;
- native main-window to popup movement in a real WebView run;
- hidden-window and renderer-reload formal rounds;
- 4/8-session full workloads and retained raw RSS/latency reports;
- a closed P0-01A release target matrix.

No alternate data plane was added. If later platform or paired benchmark
evidence falsifies this candidate, it must be deleted before another candidate
is implemented. P3-01 may productize it only after Gate 3 evidence closes.
