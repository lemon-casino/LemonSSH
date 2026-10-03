import { DEFAULT_SCRIPT_TEMPLATE } from './snippetScript.ts';

const EXECUTION_MODEL = `## Execution model (recorded-replay runner)

LemonSSH does NOT run a JavaScript engine. A script is parsed line by line into
recorded actions and replayed against the active terminal by the Go runner.
Anything the parser does not recognize is rejected with \`unsupported script line\`
before any action runs — keep scripts inside the grammar below.`;

const SOURCE_RULES = `## Script source rules

- Blank lines, \`//\` comments, and the exact wrapper lines \`async function main()\`, \`}\` and \`await main();\` are ignored. Include them or not — nothing else from JavaScript is understood.
- Every other statement must be exactly one supported \`nct.*\` call (list below). A statement may span multiple lines only while its brackets stay open (useful for \`nct.dialog.form\` object literals).
- No general JavaScript: no \`if\`/\`for\`/\`while\`, no \`throw\`, no template literals or \`\${...}\` interpolation, no arithmetic or string concatenation, no property reads (e.g. \`nct.session.name\`), no helper functions.
- Variables: only \`const name = await nct.dialog...\` and \`const name = await nct.screen.getText(...)\` assignments. A captured \`name\` can then be passed wherever a string argument is expected (\`sendLine\`, \`send\`, \`nct.log\`). No other expressions or reassignments.
- The \`language: python\` field is a UI label only — there is no Python runtime.`;

const TRIGGER_GUIDE = `## Triggers and host targeting

| trigger | Behavior |
|---------|----------|
| manual | Run from Vault or via scripts_run / snippets_run |
| onConnect | Runs after SSH connect (global targetsAllHosts, dynamic targetGroups, then host connectScriptIds queue) |
| onOutput | Runs when terminal output matches triggerPattern (regex) |

Use \`targets\` (host id array), \`targetGroups\` (dynamic group path array), or \`targetsAllHosts: true\` to scope runs. Group paths include nested groups and are resolved against the latest host inventory.
For per-host onConnect order, use \`host_connect_scripts_set\`. Group-scoped scripts remain inherited and are not copied into host queues.`;

const NCT_API = `## nct API reference (replay-supported calls only)

Each call must appear as its own statement, exactly in these shapes.

### nct.screen
- \`await nct.screen.waitForPrompt(ms)\` — wait for a shell prompt (# root / $ user)
- \`await nct.screen.waitForText("text", ms)\` — wait for exact literal text (regex characters are escaped)
- \`await nct.screen.waitForRegex("pattern", ms?)\` — wait for regex output; a quoted \`"/body/flags"\` string compiles as a regex (flags \`i\`, \`m\`, \`s\`), any other quoted string matches literally
- \`await nct.screen.waitForAny(["p1", "/p2/i"], ms?)\` — wait until any quoted pattern matches
- \`await nct.screen.sendLine("cmd")\` or \`await nct.screen.sendLine(name)\` — type command + Enter; an optional \`{ sensitive: true }\` second argument masks it in the run log
- \`await nct.screen.send("text")\` / \`await nct.screen.send(name)\` — raw keys without Enter; also accepts \`{ sensitive: true }\`
- \`const text = await nct.screen.getText()\` — capture the screen buffer; optionally \`getText(startRow, rowCount)\`
- \`await nct.screen.clear()\` — clear the captured screen

### nct.session
- \`await nct.session.sleep(ms)\` — pause between actions (there is no bare \`nct.sleep\` alias in replay)
- \`await nct.session.startLog(path?)\` / \`await nct.session.stopLog()\`
- \`await nct.session.disconnect()\`

### nct.dialog (requires non-Observer permission mode)
- \`const go = await nct.dialog.confirm("msg")\` — yes/no
- \`const value = await nct.dialog.prompt("msg")\` — text input; a \`sensitive\` option masks it. A second default-value argument is parsed but ignored by the replay runner.
- \`await nct.dialog.alert("msg")\`
- \`const answers = await nct.dialog.form({ title?, message?, fields })\` (or bare without \`const\`) — fields support \`select\`, \`checkbox\`, \`radio\`, \`textarea\`, and \`number\`
- \`const pick = await nct.dialog.select("msg", ["a", "b"], "a?")\` (or bare) — convenience single-select
- \`const pick = await nct.dialog.radio("msg", options, default?)\` (or bare)
- \`const flag = await nct.dialog.checkbox("msg", defaultChecked?)\` (or bare)

\`select\` and \`radio\` options may be strings or \`{ label, value, description?, disabled? }\`; option values must be non-empty and unique within the field.
\`textarea\` returns string values; \`number\` returns number values or \`undefined\` when optional and empty. \`number\` fields support submit-time \`min\`, \`max\`, and \`step\` validation.
Fields may use \`visibleWhen: { field, equals|notEquals|truthy|falsy }\` for conditional display; \`visibleWhen.field\` must reference an earlier field. Hidden fields are not validated and are omitted from the submitted object.
\`form\` returns an object keyed by visible field \`name\`. Field names must not be \`__proto__\`, \`prototype\`, or \`constructor\`. Text, number, select, and radio fields are required/defaulted by default; checkbox fields are optional boolean fields unless \`required: true\` is set.
Dialog object literals accept quoted strings, numbers, \`true\`/\`false\`/\`null\`, arrays, and nested objects — no computed values.

### nct.progress (literal arguments only)
- \`nct.progress.start("label", total)\` — opt-in determinate bar
- \`nct.progress.set(n, "detail?")\` / \`nct.progress.step("detail?")\` / \`nct.progress.done()\`

### nct.log
- \`nct.log("message")\` or \`nct.log(name)\` — append to the script run log panel

## Not available to replayed scripts

\`nct.version\`, \`nct.session.connected\` / \`name\` / \`hostname\` / \`username\`, \`nct.screen.rows\` / \`cols\` / \`currentRow\`, the \`nct.screen.waitFor\` helper, and any other API or JavaScript construct not listed above are rejected as \`unsupported script line\`.`;

/** Markdown reference for AI agents — single source for scripts_reference tool and prompts. */
export function getScriptApiReference(): string {
  return [
    '# LemonSSH automation script reference',
    '',
    'Automation scripts are Vault snippets with `kind: "script"`. They replay recorded actions against the active terminal session.',
    '',
    EXECUTION_MODEL,
    '',
    SOURCE_RULES,
    '',
    TRIGGER_GUIDE,
    '',
    NCT_API,
    '',
    '## Minimal template',
    '',
    '```javascript',
    DEFAULT_SCRIPT_TEMPLATE.trim(),
    '```',
  ].join('\n');
}
