import assert from 'node:assert/strict';
import test from 'node:test';
import { createElement, useEffect } from 'react';
import { act, create } from 'react-test-renderer';
import { JSDOM } from 'jsdom';
import { useSettingsState } from './useSettingsState';
import { useTerminalSettingsStore, type TerminalSettingsSnapshot } from './terminalSettingsStore';
import { hostStorageAdapter, configureHostProfileClient, hydrateHostProfile, flushHostProfileWrites } from '../../infrastructure/persistence/hostStorageAdapter';
import { STORAGE_KEY_TERM_SETTINGS, STORAGE_KEY_TERM_FONT_SIZE } from '../../infrastructure/config/storageKeys';
import { resolveTerminalAutocompleteSettings } from '../../components/terminal/autocomplete/terminalAutocompleteSettings';
import type { ProfileClient } from '../../infrastructure/runtime/profile/profileClient';

for (const canonical of [false, true]) {
  test(`mounted settings publish live terminal changes (${canonical ? 'canonical' : 'browser'} storage)`, async () => {
    const dom = new JSDOM('<html><body></body></html>', { url: 'http://localhost' });
    const originals = new Map<string, PropertyDescriptor | undefined>();
    const install = (key: string, value: unknown) => {
      originals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
      Object.defineProperty(globalThis, key, { configurable: true, value });
    };
    const windowRecord = dom.window as unknown as Record<string, unknown>;
    for (const key of ['window', 'document', 'localStorage', 'CustomEvent', 'navigator']) install(key, windowRecord[key]);
    for (const key of ['addEventListener', 'removeEventListener', 'dispatchEvent']) install(key, (windowRecord[key] as (this: unknown, ...args: unknown[]) => void).bind(dom.window));
    install('IS_REACT_ACT_ENVIRONMENT', true);
    install('BroadcastChannel', undefined);
    let revision = 1;
    const remote = new Map<string, string>();
    const client: ProfileClient = {
      revision: async () => revision, domains: async () => ['settings'],
      domainKeys: async domain => [...remote.keys()].filter(key => key.startsWith(`${domain}/`)).map(key => key.slice(domain.length + 1)),
      getRawBase64: async (domain, key) => remote.get(`${domain}/${key}`),
      setRawBase64: async () => { throw new Error('unexpected write'); }, deleteRaw: async () => { throw new Error('unexpected delete'); },
      write: async (expected, mutations) => {
        assert.equal(expected, revision);
        for (const edit of mutations) {
          if (edit.delete) remote.delete(`${edit.domain}/${edit.key}`);
          else remote.set(`${edit.domain}/${edit.key}`, edit.valueBase64!);
        }
        return { revision: ++revision };
      },
    };
    let settings!: ReturnType<typeof useSettingsState>;
    let snapshot!: TerminalSettingsSnapshot;
    let mounts = 0;
    let completionSettings: ReturnType<typeof resolveTerminalAutocompleteSettings>;
    function Settings() { settings = useSettingsState({ enableSystemEffects: false }); return null; }
    function TerminalSubscriber() {
      snapshot = useTerminalSettingsStore();
      completionSettings = resolveTerminalAutocompleteSettings({ protocol: 'ssh', terminalSettings: snapshot.terminalSettings });
      useEffect(() => { mounts++; }, []);
      return null;
    }
    let root: ReturnType<typeof create> | undefined;
    const settle = async () => { await flushHostProfileWrites(); await new Promise(resolve => setTimeout(resolve, 20)); };
    try {
      if (canonical) { configureHostProfileClient(client); await hydrateHostProfile(); }
      await act(async () => { root = create(createElement('div', null, createElement(Settings), createElement(TerminalSubscriber))); });
      await act(settle);
      await act(async () => { settings.updateTerminalSetting('autocompleteGhostText', false); });
      await act(settle);
      assert.equal(snapshot.terminalSettings.autocompleteGhostText, false);
      // Adapter notifications are the only same-window signal, including Wails cache refreshes.
      await act(async () => {
        hostStorageAdapter.write(STORAGE_KEY_TERM_SETTINGS, { ...snapshot.terminalSettings, autocompleteGhostText: true, autocompletePopupMenu: false, rightClickBehavior: 'paste' });
        await settle();
      });
      assert.equal(snapshot.terminalSettings.autocompleteGhostText, true);
      assert.equal(snapshot.terminalSettings.autocompletePopupMenu, false);
      assert.equal(completionSettings?.showGhostText, true);
      assert.equal(completionSettings?.showPopupMenu, false);
      assert.equal(completionSettings?.livePreview, true);
      assert.equal(snapshot.terminalSettings.rightClickBehavior, 'paste');
      // Browser payloads are invalidations when Go owns the canonical profile.
      const incoming = JSON.stringify({ ...snapshot.terminalSettings, autocompleteGhostText: false, autocompletePopupMenu: true });
      await act(async () => {
        if (canonical) { remote.set(`settings/${STORAGE_KEY_TERM_SETTINGS}`, Buffer.from(incoming).toString('base64')); revision++; }
        else dom.window.localStorage.setItem(STORAGE_KEY_TERM_SETTINGS, incoming);
        dom.window.dispatchEvent(new dom.window.StorageEvent('storage', { key: STORAGE_KEY_TERM_SETTINGS, newValue: canonical ? '{"autocompleteGhostText":true}' : incoming }));
        await settle();
      });
      assert.equal(snapshot.terminalSettings.autocompleteGhostText, false);
      assert.equal(snapshot.terminalSettings.autocompletePopupMenu, true);
      await act(async () => { hostStorageAdapter.writeString(STORAGE_KEY_TERM_FONT_SIZE, '19'); await settle(); });
      assert.equal(snapshot.terminalFontSize, 19);
      assert.equal(mounts, 1);
    } finally {
      await act(async () => { root?.unmount(); await settle(); });
      configureHostProfileClient(undefined);
      dom.window.close();
      for (const [key, descriptor] of originals) {
        if (descriptor) Object.defineProperty(globalThis, key, descriptor);
        else Reflect.deleteProperty(globalThis, key);
      }
    }
  });
}
