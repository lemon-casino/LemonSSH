import type { Snippet, SnippetKind } from './models';

export function getSnippetKind(snippet: Pick<Snippet, 'kind'>): SnippetKind {
  return snippet.kind === 'script' ? 'script' : 'snippet';
}

export function isScriptSnippet(snippet: Pick<Snippet, 'kind'>): boolean {
  return getSnippetKind(snippet) === 'script';
}

/** Common interactive shell prompts (root uses #, regular user uses $). */
export const DEFAULT_SHELL_PROMPT_PATTERNS = ['# ', '$ ', '~# ', '~$ ', '% '];

/** Default wait after each recorded command — matches waitForPrompt in generated scripts. */
export const DEFAULT_RECORDING_PROMPT_TIMEOUT_MS = 30000;

/** Regex matching a shell prompt on the last line (~# / ~$ / user@host:path#). */
export const SHELL_PROMPT_END_REGEX = /(?:~[#$]\s*|[@][^\n]{0,120}[:][^\n]{0,120}[#$%]\s*)$/m;

/** Minimal smoke script for verifying manual run and onOutput triggers. */
export const SCRIPT_SMOKE_TEST = `// === Smoke test ===
// Manual:  trigger=manual, pick a target host, click "Run now"
// onOutput: trigger=onOutput, pattern=LEMONSSH_SMOKE, save, connect to target host, then run: echo LEMONSSH_SMOKE
//
await nct.screen.waitForPrompt(30000);
await nct.screen.sendLine('echo lemonssh-smoke-ok');
nct.log('Smoke test passed');
await nct.dialog.alert('LemonSSH script smoke test OK');
`;

/** Full integration test for onConnect / manual run; dialog API enabled by default. */
export const SCRIPT_INTEGRATION_TEST = `// LemonSSH Integration Test — onConnect / manual replay exercise
// Trigger: onConnect or Run now | Permission: Auto or Confirm (dialogs need non-Observer)
// One supported nct.* call per line: the replay parser rejects any other
// JavaScript (no if/for, no template literals, no property reads).

await nct.screen.waitForPrompt(60000);
nct.log('=== LemonSSH Integration Test START ===');

nct.log('[1/12] sendLine + waitForText');
await nct.screen.sendLine('echo nc-it-BOOTSTRAP_OK');
await nct.screen.waitForText('nc-it-BOOTSTRAP_OK', 20000);

nct.log('[2/12] waitForAny');
await nct.screen.sendLine('echo ANY_CHECK && uname -s');
await nct.screen.waitForAny(["ANY_CHECK", "/Linux/"], 20000);

nct.log('[3/12] getText');
const text = await nct.screen.getText();
nct.log(text);

nct.log('[4/12] send raw + Enter');
await nct.screen.send('echo -n "nc-it-RAW"');
await nct.screen.sendLine('');
await nct.screen.waitForText('nc-it-RAW', 20000);

nct.log('[5/12] waitForRegex');
await nct.screen.sendLine('echo BUILD_ID=nc-it-001');
await nct.screen.waitForRegex("/BUILD_ID=nc-it-001/", 20000);

nct.log('[6/12] session.sleep');
await nct.session.sleep(800);

nct.log('[7/12] progress');
nct.progress.start('Health sampling', 2);
await nct.screen.sendLine('echo "== Sample 1/2 ==" && date && uptime');
await nct.screen.waitForPrompt(60000);
nct.progress.step('sample 1/2');
await nct.screen.sendLine('echo "== Sample 2/2 ==" && df -h /');
await nct.screen.waitForPrompt(60000);
nct.progress.step('sample 2/2');
nct.progress.done();

nct.log('[8/12] screen.clear (uncomment to run)');
// await nct.screen.clear();

nct.log('[9/12] dialog confirm / prompt / alert');
const go = await nct.dialog.confirm('Integration test finished OK. Continue to prompt/alert?');
nct.log(go);
const note = await nct.dialog.prompt('Optional note:');
nct.log(note);
await nct.dialog.alert('Done. Check the run log for captured values.');

nct.log('[10/12] select dialog');
const shell = await nct.dialog.select('Preferred login shell?', ['/bin/bash', '/bin/zsh'], '/bin/bash');
nct.log(shell);

nct.log('[11/12] form dialog');
const answers = await nct.dialog.form({ title: 'Replay check', message: 'Confirm environment', fields: [ { type: 'select', name: 'shell', label: 'Login shell', options: ['/bin/bash', '/bin/zsh'] }, { type: 'number', name: 'count', label: 'Samples', defaultValue: 2, min: 1, max: 10 } ] });
nct.log(answers);

nct.log('[12/12] session logging (uncomment to run)');
// await nct.session.startLog('./lemonssh-it.log');
// await nct.screen.sendLine('echo nc-it-LOGGED');
// await nct.screen.waitForText('nc-it-LOGGED', 20000);
// await nct.session.stopLog();

await nct.screen.sendLine('echo nc-it-ALL_PASSED');
await nct.screen.waitForText('nc-it-ALL_PASSED', 20000);
nct.log('=== LemonSSH Integration Test PASSED ===');
`;

export const DEFAULT_SCRIPT_TEMPLATE = `// LemonSSH automation script — recorded-replay against the active terminal
//
// The runner parses one supported nct.* call per line (no JS engine):
// no if/for, no template literals, no property reads. Unknown lines fail
// with "unsupported script line" before anything runs.
//
// nct.screen.waitForPrompt(ms)            wait for shell prompt (# root / $ user)
// nct.screen.waitForText(text, ms)         wait for exact output text
// nct.screen.waitForRegex(pattern, ms?)    wait for regex output, including multiline
// nct.screen.waitForAny([patterns], ms?)   wait until any pattern matches
// nct.screen.sendLine(cmd)                 type command + Enter; send(text) raw keys only
// nct.screen.getText() | clear()
// nct.session.sleep(ms) | startLog(path) | stopLog() | disconnect()
// nct.dialog.confirm(msg)->bool | prompt(msg)->string | alert(msg)
// nct.dialog.form({ fields })->object; fields: select/radio/checkbox/textarea/number, optional visibleWhen
// nct.dialog.select/radio/checkbox are convenience helpers
// nct.progress.start(label, total)         opt-in determinate progress
// nct.progress.step(detail?) | set(n, detail?) | done()
// nct.log(msg)  run log panel. Type "nct." in editor for autocomplete snippets.
//
await nct.screen.waitForPrompt(30000);
await nct.screen.sendLine('echo hello');
nct.log('Done');
`;

const SCRIPT_WRITE_PATTERN = /nct\.screen\.(send(?:Line)?|clear)\s*\(|nct\.session\.(disconnect|startLog)\s*\(/;

export function scriptContainsWriteOperations(content: string): boolean {
  return SCRIPT_WRITE_PATTERN.test(String(content || ''));
}
