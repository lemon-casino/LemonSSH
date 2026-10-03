import assert from 'node:assert/strict';
import test from 'node:test';
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { JSDOM } from 'jsdom';

import { setNotify } from '../notification.ts';
import { I18nProvider } from '../i18n/I18nProvider.tsx';
import { PluginContributionHost } from '../../components/plugins/PluginContributionHost.tsx';
import { requestOpenPluginView } from './usePluginViewLifecycle.ts';

const VIEW_OPEN_SNAPSHOT = {
  locale: 'en',
  plugins: [{
    id: 'com.example.views',
    version: '1.0.0',
    displayName: 'View Plugin',
    description: '',
    commands: [],
    keybindings: [],
    menus: [],
    settings: [],
    views: [{ id: 'com.example.views.status', title: 'Status', location: 'modal', entry: '', visible: true }],
  }],
} as LemonSSHPluginContributionSnapshot;

function failingViewBridge() {
  return {
    getPluginRuntimeStatus: async () => ({ available: true, experimental: true as const }),
    getPluginContributions: async () => VIEW_OPEN_SNAPSHOT,
    onPluginContributionsChanged: () => () => {},
    openPluginView: async () => {
      throw new Error('Plugin views are unavailable');
    },
    closePluginView: async () => undefined,
    setPluginViewBounds: async () => undefined,
    setPluginViewVisibility: async () => undefined,
    setPluginEnvironment: async () => undefined,
  };
}

function installDom() {
  const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/' });
  const define = (target: Record<string, unknown>, key: string, value: unknown) => {
    Object.defineProperty(target, key, { configurable: true, writable: true, value });
  };
  define(globalThis, 'window', dom.window);
  define(globalThis, 'document', dom.window.document);
  define(globalThis, 'CustomEvent', dom.window.CustomEvent);
  define(globalThis, 'Event', dom.window.Event);
  define(globalThis, 'MutationObserver', dom.window.MutationObserver);
  define(globalThis, 'getComputedStyle', dom.window.getComputedStyle.bind(dom.window));
  define(globalThis, 'IS_REACT_ACT_ENVIRONMENT', true);
  if (!globalThis.navigator) define(globalThis, 'navigator', dom.window.navigator);
  const matchMedia = () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
  });
  define(dom.window, 'matchMedia', matchMedia);
  define(dom.window, 'requestAnimationFrame', (callback: (time: number) => void) => {
    callback(0);
    return 0;
  });
  define(dom.window, 'cancelAnimationFrame', () => {});
  return dom;
}

test('a failed plugin view open reports a visible error instead of failing silently', async () => {
  const dom = installDom();
  const originalWindow = globalThis.window;
  const reported: Array<{ type: string; message: string }> = [];
  setNotify({
    success: () => {},
    error: (message) => reported.push({ type: 'error', message }),
    warning: () => {},
    info: () => {},
  });

  globalThis.window = Object.assign(dom.window, { lemonssh: failingViewBridge() }) as unknown as typeof window;

  const container = dom.window.document.createElement('div');
  dom.window.document.body.appendChild(container);
  const root = createRoot(container);

  try {
    await act(async () => {
      root.render(React.createElement(I18nProvider, {
        locale: 'en',
        children: React.createElement(PluginContributionHost, { locale: 'en', theme: 'dark' }),
      }));
    });

    assert.equal(dom.window.document.querySelector('[role="alert"]'), null);

    await act(async () => {
      requestOpenPluginView({ viewId: 'com.example.views.status', context: { 'lemonssh.surface': 'settings' } });
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // The failure is announced through the notification port…
    assert.equal(reported.length, 1);
    assert.equal(reported[0]?.type, 'error');
    assert.match(reported[0]?.message ?? '', /Plugin views are unavailable/);
    // …and rendered as a dismissible alert instead of vanishing.
    const alert = dom.window.document.querySelector('[role="alert"]');
    assert.ok(alert);
    assert.match(alert.textContent ?? '', /Plugin views are unavailable/);
  } finally {
    await act(async () => { root.unmount(); });
    container.remove();
    globalThis.window = originalWindow;
  }
});
