import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, mkdtempSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { join, resolve } from 'node:path';
import { createWailsRuntimeClient } from '../../infrastructure/runtime/wails/wailsRuntimeClient';
import { getActiveRuntimeClient, setActiveRuntimeClient } from '../../infrastructure/runtime/runtimeClient';
import { provideTerminalCompletions } from './autocomplete/terminalCompletionProviders';
import { fileURLToPath } from "node:url";
import { useRef } from "react";
import { act, create } from 'react-test-renderer';
import { renderToStaticMarkup } from "react-dom/server";

import { useTerminalAutocomplete } from "./autocomplete/useTerminalAutocomplete.ts";
import type { Snippet } from "../../domain/models";

test("useTerminalAutocomplete can render before any autocomplete interaction", () => {
  function Probe() {
    const termRef = useRef(null);
    const containerRef = useRef(null);
    const autocomplete = useTerminalAutocomplete({
      termRef,
      containerRef,
      sessionId: "session-1",
      hostId: "host-1",
      hostOs: "linux",
      onAcceptText: () => {},
    });

    return <span>{typeof autocomplete.repositionPopup}</span>;
  }

  assert.doesNotThrow(() => {
    renderToStaticMarkup(<Probe />);
  });
});

test('mounted completion discards async results after session or cwd changes', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  let api!: ReturnType<typeof useTerminalAutocomplete>;
  let input = 'cd /data';
  const buffer = { type: 'normal', cursorY: 0, baseY: 0, viewportY: 0,
    get cursorX() { return input.length + 2; },
    getLine: () => ({ isWrapped: false, translateToString: () => '$ ' + input }),
  };
  const term = { buffer: { active: buffer }, cols: 100, rows: 30, options: {}, unicode: { activeVersion: '6' } };
  let resolve!: (items: any[]) => void;
  let calls = 0;
  let late: ((items: any[]) => void) | undefined;
  const provider = async (_input: string, options: any) => { calls++; late = options.onLatePathSuggestions; return new Promise<any[]>(done => { resolve = done; }); };
  function Probe({ session = 'a', cwd = '/data' }) {
    api = useTerminalAutocomplete({ termRef: { current: term as never }, containerRef: { current: null },
      sessionId: session, hostId: 'host', hostOs: 'linux', protocol: 'ssh', getCwd: () => cwd,
      settings: { debounceMs: 1, minChars: 1, showPopupMenu: true }, provideCompletions: provider,
      onAcceptText() {},
    });
    return null;
  }
  let root!: ReturnType<typeof create>;
  await act(async () => { root = create(<Probe />); });
  try {
    await act(async () => { api.handleInput(input); await new Promise(done => setTimeout(done, 20)); });
    assert.equal(calls, 1);
    await act(async () => { root.update(<Probe session='b' />); });
    await act(async () => { resolve([{text:'cd /data/OLD/', displayText:'OLD/', source:'path', score:750}]); });
    assert.equal(api.state.popupVisible, false);
    await act(async () => { api.handleInput('\u0015'); api.handleInput(input); await new Promise(done => setTimeout(done, 20)); });
    assert.equal(calls, 2);
    await act(async () => { root.update(<Probe session='b' cwd='/other' />); });
    await act(async () => { resolve([{text:'cd /data/STALE/', displayText:'STALE/', source:'path', score:750}]); });
    assert.equal(api.state.popupVisible, false);
    await act(async () => { api.handleInput('\u0015'); api.handleInput(input); await new Promise(done => setTimeout(done, 20)); });
    await act(async () => { resolve([{text:'cd /data/Mihomo/', displayText:'Mihomo/', source:'path', score:750}]); });
    assert.equal(api.state.popupVisible, true);
    assert.equal(api.state.suggestions[0].text, 'cd /data/Mihomo/');
    await act(async () => { late?.([{text:'cd /data/My\\ files/', displayText:'My files/', source:'path', score:750}]); });
    assert.equal(api.state.suggestions[0].text, 'cd /data/My\\ files/');
    await act(async () => { root.update(<Probe session='c' cwd='/new' />); });
    await act(async () => { late?.([{text:'cd /data/WRONG/', displayText:'WRONG/', source:'path', score:750}]); });
    assert.ok(api.state.suggestions.every(item => !item.text.includes('WRONG')));
  } finally { await act(async () => root.unmount()); }
});

for (const mode of ['popup', 'inline'] as const) {
  test(`mounted ${mode} uses active Wails runtime for empty-history paths and successive children`, async () => {
    // The external boundary is native RPC; everything above it is production code.
    // A real filesystem fixture supplies native-shaped JSON, without window.netcatty.
    const fixture = mkdtempSync(join(process.cwd(), '.completion-fixture-'));
    mkdirSync(join(fixture, 'data/Mihomo/config'), { recursive: true });
    mkdirSync(join(fixture, 'other/Next'), { recursive: true });
    const previousRuntime = getActiveRuntimeClient();
    const previousDocument = globalThis.document;
    const previousRaf = globalThis.requestAnimationFrame;
    const element = () => ({ style: {}, textContent: '', appendChild() {}, remove() {}, querySelector() { return null; } });
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true,
      document: { createElement: element }, requestAnimationFrame: (cb: () => void) => setTimeout(cb, 0) });
    let cwd: string | undefined;
    let nativeCwd = '/data';
    let input = '';
    const requests: string[] = [];
    const runtime = createWailsRuntimeClient({ terminal: {
      ListAutocompleteDirectory: async (id, directory, foldersOnly, prefix, limit) => {
        assert.equal(id, `native-${mode}`);
        requests.push(directory);
        const lookup = directory.startsWith('/') ? directory : `${nativeCwd}/${directory}`;
        const entries = readdirSync(resolve(fixture, `.${lookup}`), { withFileTypes: true })
          .filter(entry => (!foldersOnly || entry.isDirectory()) && entry.name.toLowerCase().startsWith(prefix.toLowerCase()))
          .slice(0, limit).map(entry => ({ name: entry.name, type: entry.isDirectory() ? 'directory' as const : 'file' as const }));
        return JSON.parse(JSON.stringify({ success: true, entries }));
      },
    }, sftp: {} } as never);
    setActiveRuntimeClient(runtime);
    const buffer = { type: 'normal', cursorY: 0, baseY: 0, viewportY: 0,
      get cursorX() { return input.length + 2; },
      getLine: () => ({ isWrapped: false, translateToString: () => '$ ' + input }) };
    const term = { element: element(), buffer: { active: buffer }, cols: 120, rows: 30,
      options: { fontSize: 14 }, unicode: { activeVersion: '6' },
      _core: { _renderService: { dimensions: { css: { cell: { width: 9, height: 18 } } } } },
      onRender: () => ({ dispose() {} }), onResize: () => ({ dispose() {} }) };
    let api!: ReturnType<typeof useTerminalAutocomplete>;
    function Probe({ display = mode, enabled = true }: { display?: 'inline' | 'popup'; enabled?: boolean }) {
      api = useTerminalAutocomplete({ termRef: { current: term as never }, containerRef: { current: null },
        sessionId: `native-${mode}`, hostId: `empty-${mode}`, hostOs: 'linux', protocol: 'ssh', getCwd: () => cwd,
        settings: { enabled, debounceMs: 1, showPopupMenu: display === 'popup', showGhostText: display === 'inline', livePreview: true },
        provideCompletions: (text, options) => provideTerminalCompletions(null, {
          input: text, session: { sessionId: options.sessionId!, hostId: options.hostId, protocol: 'ssh', status: 'connected', cwd: options.cwd },
          hostOs: 'linux', maximum: 50, cwdSource: options.cwdSource, onLatePathSuggestions: options.onLatePathSuggestions }),
        onAcceptText: text => { input = text.startsWith('\x15') ? text.slice(1) : input + text; },
      });
      return null;
    }
    let root!: ReturnType<typeof create>;
    const settle = () => new Promise(done => setTimeout(done, 100));
    const type = async (text: string) => { await act(async () => { api.handleInput('\x15'); input = text; api.handleInput(text); await settle(); }); };
    const key = async (key: string) => { await act(async () => { api.handleKeyEvent({ key, type: 'keydown', preventDefault() {} } as KeyboardEvent); await settle(); }); };
    try {
      await act(async () => { root = create(<Probe />); });
      await type('cd /data');
      assert.ok(api.state.suggestions.some(item => item.text === 'cd /data/Mihomo/'), 'absolute path must work without cwd or legacy bridge');
      if (mode === 'popup') {
        await key('ArrowDown');
        assert.ok(api.state.subDirPanels.some(panel => panel.entries.some(entry => entry.name === 'Mihomo')), 'selected directory previews children');
        await act(async () => { api.selectSuggestion(api.state.suggestions.find(item => item.text === 'cd /data/')!); await settle(); });
      } else {
        assert.equal(api.ghostTextAddon?.getSuggestion(), 'cd /data/');
        await key('ArrowRight');
      }
      assert.equal(input, 'cd /data/');
      assert.ok(api.state.suggestions.some(item => item.text === 'cd /data/Mihomo/'), 'acceptance must request the next child level');
      if (mode === 'popup') {
        await key('ArrowDown');
        assert.ok(api.state.subDirPanels.some(panel => panel.entries.some(entry => entry.name === 'config')));
        await act(async () => { api.selectSuggestion(api.state.suggestions.find(item => item.text === 'cd /data/Mihomo/')!); await settle(); });
      } else { await key('ArrowRight'); }
      assert.equal(input, 'cd /data/Mihomo/');
      assert.ok(api.state.suggestions.some(item => item.text === 'cd /data/Mihomo/config/'), 'successive acceptance must request children');
      nativeCwd = '/other'; cwd = undefined;
      await type('cd ');
      assert.ok(api.state.suggestions.some(item => item.text === 'cd Next/'), 'relative lookup follows native cwd');
      assert.ok(requests.includes('.'));
      await act(async () => { root.update(<Probe display={mode === 'popup' ? 'inline' : 'popup'} />); });
      await act(async () => { await settle(); });
      assert.equal(api.state.popupVisible, mode === 'inline', 'mode change refreshes current input without a keystroke');
      assert.equal(api.ghostTextAddon?.isActive(), mode === 'popup');
      await act(async () => { root.update(<Probe enabled={false} />); });
      assert.equal(api.state.popupVisible, false);
      assert.equal(api.ghostTextAddon?.isActive() ?? false, false);
      await act(async () => { root.update(<Probe enabled />); });
      await act(async () => { await settle(); });
      assert.ok(api.state.suggestions.some(item => item.text === 'cd Next/'));
    } finally {
      if (root) await act(async () => root.unmount());
      setActiveRuntimeClient(previousRuntime);
      Object.assign(globalThis, { document: previousDocument, requestAnimationFrame: previousRaf });
      rmSync(fixture, { recursive: true, force: true });
    }
  });
}

test('active Wails providers offer real common command specs and host-scoped saved scripts with empty history', async () => {
  const previous = getActiveRuntimeClient();
  setActiveRuntimeClient(createWailsRuntimeClient({ terminal: {}, sftp: {} } as never));
  try {
    for (const [input, expected] of [['gi', 'git'], ['doc', 'docker'], ['l', 'ls'], ['c', 'cd'], ['git st', 'git status'], ['docker ps', 'docker ps'], ['ls -', 'ls -a']]) {
      const items = await provideTerminalCompletions(null, { input,
        session: { sessionId: 'spec-session', hostId: 'empty-spec-host', protocol: 'ssh', status: 'connected' }, hostOs: 'linux', maximum: 100 });
      assert.ok(items.some(item => item.text.trimEnd() === expected), `${input} must offer ${expected}`);
    }
    const scripts = await provideTerminalCompletions(null, { input: 'Deploy',
      session: { sessionId: 'scripts', hostId: 'host-a', protocol: 'ssh', status: 'connected' }, hostOs: 'linux', maximum: 50,
      snippets: [
        { id: 'allowed', label: 'Deploy app', command: 'bash deploy.sh', targets: ['host-a'] },
        { id: 'other', label: 'Deploy private', command: 'bash private.sh', targets: ['host-b'] },
      ] as never });
    assert.deepEqual(scripts.filter(item => item.source === 'snippet').map(item => item.text), ['Deploy app']);
  } finally { setActiveRuntimeClient(previous); }
});

test('mounted popup surfaces saved snippets host-scoped with their source object and never types or executes their commands', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  const previousRuntime = getActiveRuntimeClient();
  const previousDocument = globalThis.document;
  const previousRaf = globalThis.requestAnimationFrame;
  const element = () => ({ style: {}, textContent: '', appendChild() {}, remove() {}, querySelector() { return null; } });
  Object.assign(globalThis, { document: { createElement: element }, requestAnimationFrame: (cb: () => void) => setTimeout(cb, 0) });
  setActiveRuntimeClient(createWailsRuntimeClient({ terminal: {}, sftp: {} } as never));
  let input = '';
  const writes: string[] = [];
  const accepted: Snippet[] = [];
  const deploySnippet = { id: 'deploy-1', label: 'Deploy stack', command: 'bash /opt/deploy.sh', targets: ['snippet-host'] };
  const privateSnippet = { id: 'deploy-2', label: 'Deploy private', command: 'bash /opt/private.sh', targets: ['other-host'] };
  const buffer = { type: 'normal', cursorY: 0, baseY: 0, viewportY: 0,
    get cursorX() { return input.length + 2; },
    getLine: () => ({ isWrapped: false, translateToString: () => '$ ' + input }) };
  const term = { element: element(), buffer: { active: buffer }, cols: 120, rows: 30,
    options: { fontSize: 14 }, unicode: { activeVersion: '6' },
    _core: { _renderService: { dimensions: { css: { cell: { width: 9, height: 18 } } } } },
    onRender: () => ({ dispose() {} }), onResize: () => ({ dispose() {} }) };
  let api!: ReturnType<typeof useTerminalAutocomplete>;
  function Probe() {
    api = useTerminalAutocomplete({ termRef: { current: term as never }, containerRef: { current: null },
      sessionId: 'snippet-session', hostId: 'snippet-host', hostOs: 'linux', protocol: 'ssh',
      settings: { debounceMs: 1, minChars: 1, showPopupMenu: true, showGhostText: false, livePreview: true },
      provideCompletions: (text, options) => provideTerminalCompletions(null, {
        input: text, session: { sessionId: options.sessionId!, hostId: options.hostId, protocol: 'ssh', status: 'connected' },
        hostOs: 'linux', maximum: 50, snippets: options.snippets }),
      snippets: [deploySnippet, privateSnippet] as never,
      onAcceptText: text => { writes.push(text); input = text.startsWith('\x15') ? text.slice(1) : input + text; },
      onAcceptSnippet: snippet => { accepted.push(snippet); },
    });
    return null;
  }
  let root!: ReturnType<typeof create>;
  try {
    await act(async () => { root = create(<Probe />); });
    await act(async () => { api.handleInput('\x15'); input = 'dep'; api.handleInput('dep'); await new Promise(done => setTimeout(done, 80)); });
    const snippetSuggestion = api.state.suggestions.find(item => item.source === 'snippet');
    assert.ok(snippetSuggestion, 'popup must surface a snippet candidate at the command position');
    assert.equal(snippetSuggestion?.text, 'Deploy stack');
    assert.equal(snippetSuggestion?.snippet, deploySnippet, 'candidate must carry the source snippet object for the accept path');
    assert.equal(api.state.suggestions.some(item => item.snippet?.id === 'deploy-2'),
      false, 'snippets scoped to other hosts must not surface');
    await act(async () => { api.handleKeyEvent({ key: 'ArrowDown', type: 'keydown', preventDefault() {} } as KeyboardEvent); });
    assert.equal(input, 'dep', 'live preview must keep the typed line instead of typing the snippet command');
    assert.ok(writes.every(write => !write.includes('deploy.sh') && !write.includes('\r')),
      'navigating to a snippet must not type or execute its command');
    await act(async () => { api.selectSuggestion(snippetSuggestion!); });
    assert.deepEqual(accepted, [deploySnippet], 'accepting must delegate the source snippet to the host send path');
    assert.ok(writes.every(write => !write.includes('deploy.sh') && !write.includes('\r')),
      'accepting must never type the snippet command or send Enter');
  } finally {
    if (root) await act(async () => root.unmount());
    setActiveRuntimeClient(previousRuntime);
    Object.assign(globalThis, { document: previousDocument, requestAnimationFrame: previousRaf });
  }
});

test("mount effect re-arms disposedRef after dispose cleanup (HMR / StrictMode)", () => {
  // dispose() sets disposedRef=true on effect cleanup. Fast Refresh preserves
  // refs, so without resetting on mount, fetchSuggestions stays dead forever
  // while handleInput keeps scheduling — "fetch-scheduled" with no popup.
  const source = readFileSync(
    fileURLToPath(new URL("./autocomplete/useTerminalAutocomplete.ts", import.meta.url)),
    "utf8",
  );
  assert.ok(source.includes("disposedRef.current = false;"));
  assert.ok(source.includes("return () => { dispose(); };"));
});
