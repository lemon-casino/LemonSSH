import { DEFAULT_SCRIPT_TEMPLATE } from './snippetScript.ts';

const EXECUTION_MODEL = `## Execution model (JavaScript)

LemonSSH executes JavaScript in an isolated Go-hosted runtime. Scripts support
variables, functions, loops, conditionals, template literals, RegExp, promises
and async/await. Terminal access is exposed only through the nct API below.
Node.js modules, require, process, filesystem/network globals and dynamic code
generation (eval/Function) are not available. CPU-bound execution is interrupted
after one second without yielding; await nct.sleep(...) for long computations.
Stop cancels both JavaScript execution and pending host operations. Observer
mode rejects terminal writes even when called through computed property names.`;

const SOURCE_RULES = `## Script source rules

- Write top-level statements with await, an async function main(), or an async IIFE.
- A declared main() is called automatically; the conventional final await main(); is not run twice.
- Await terminal and dialog operations so dependent actions happen in order.
- Arguments may be computed expressions. Dialog form specifications may be built dynamically.
- The language: python field is a UI label only; Python is not an execution runtime.`;

const TRIGGER_GUIDE = `## Triggers and host targeting

| trigger | Behavior |
|---------|----------|
| manual | Run from Vault or via scripts_run / snippets_run |
| onConnect | Runs after SSH connect (global targetsAllHosts, dynamic targetGroups, then host connectScriptIds queue) |
| onOutput | Runs when terminal output matches triggerPattern (regex) |

Use targets (host id array), targetGroups (dynamic group path array), or targetsAllHosts: true to scope runs. Group paths include nested groups and are resolved against the latest host inventory.
For per-host onConnect order, use host_connect_scripts_set. Group-scoped scripts remain inherited and are not copied into host queues.`;

const NCT_API = `## nct API reference

### nct.screen
- await nct.screen.waitForPrompt(ms = 60000) — wait for a shell prompt.
- await nct.screen.waitForText(text, ms = 30000) — wait for exact literal text; returns the matched text.
- await nct.screen.waitForRegex(pattern, ms = 30000) — accepts a RegExp, regex source string, or legacy "/body/flags" string; returns matched text.
- await nct.screen.waitFor(pattern, ms = 30000) — literal strings or RegExp.
- await nct.screen.waitForAny(patterns, ms = 30000) — returns the zero-based matching pattern index.
- Successful waits consume matching output so later waits do not reuse old matches. Timeouts offer retry, skip, or stop when the dialog host is available.
- await nct.screen.sendLine(command, { sensitive: true }?) — type command then Enter. Sensitive values are masked in run logs.
- await nct.screen.send(text, { sensitive: true }?) — raw keys without Enter.
- await nct.screen.getText(startRow?, endRow?) — capture the screen buffer; row bounds are inclusive. Falls back to recent terminal output if the screen is unavailable.
- await nct.screen.clear() — send terminal clear-screen bytes and clear captured output.
- nct.screen.rows, nct.screen.cols, nct.screen.currentRow — last captured screen dimensions/cursor row.

### nct.session
- nct.session.connected, nct.session.name, nct.session.hostname, nct.session.username — session metadata.
- await nct.session.sleep(ms), or await nct.sleep(ms) — cancellable delay.
- await nct.session.startLog(path?), await nct.session.stopLog().
- await nct.session.disconnect() — close this session and end its script.

### nct.dialog
- await nct.dialog.confirm(message) — boolean yes/no.
- await nct.dialog.prompt(message, defaultValue = "", { sensitive: true }?) — text input, with default and sensitive masking.
- await nct.dialog.alert(message).
- await nct.dialog.form({ title?, message?, fields }) — returns an object keyed by visible field names.
- await nct.dialog.select(message, options, defaultValue?), await nct.dialog.radio(message, options, defaultValue?) — selected string.
- await nct.dialog.checkbox(message, defaultChecked = false) — boolean.

Form fields support select, checkbox, radio, textarea, and number. Choice options may be strings or { label, value, description?, disabled? }; values must be non-empty and unique. Number fields support min, max and step validation. visibleWhen: { field, equals|notEquals|truthy|falsy } must reference an earlier field; hidden fields are omitted. Reserved field names __proto__, prototype, constructor are rejected. A cancelled prompt or form rejects; catch the error to recover.

### Progress and logs
- nct.progress.start(label, total), nct.progress.set(current, detail?), nct.progress.step(detail?), nct.progress.done().
- nct.log(message), console.log(...values) — append to the script run log.
- nct.version — application version.

Runs are bounded to 128 pending host calls, 512 log notifications and 20,000 host operations. Scripts cannot access native services outside nct.`;

/** Markdown reference for AI agents — single source for scripts_reference tool and prompts. */
export function getScriptApiReference(): string {
  return [
    '# LemonSSH automation script reference',
    '',
    'Automation scripts are Vault snippets with `kind: "script"`. They execute JavaScript against the active terminal session.',
    '', EXECUTION_MODEL, '', SOURCE_RULES, '', TRIGGER_GUIDE, '', NCT_API,
    '', '## Minimal template', '', '```javascript', DEFAULT_SCRIPT_TEMPLATE.trim(), '```',
  ].join('\n');
}
