import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { createDomRenderer, installDomEnvironment, dispatchDomEvent } from '../test-support/renderReactDom';
import { installTreeEnvironmentMocks } from './testEnvironmentMocks';

test('compact workbench menu exposes every vault section and closes after navigation', async () => {
  const env = installDomEnvironment();
  const restore = installTreeEnvironmentMocks();
  const globals = ['requestAnimationFrame', 'cancelAnimationFrame', 'NodeFilter', 'HTMLInputElement'] as const;
  const descriptors = Object.getOwnPropertyDescriptors(globalThis);
  Object.assign(globalThis, {
    requestAnimationFrame: () => 1,
    cancelAnimationFrame: () => {},
    NodeFilter: env.window.NodeFilter,
    HTMLInputElement: env.window.HTMLInputElement,
  });
  const renderer = await createDomRenderer(env.document);
  try {
    const { WorkbenchChrome } = await import('./WorkbenchChrome');
    const { TooltipProvider } = await import('../ui/tooltip');
    const selected: string[] = [];
    await renderer.render(<TooltipProvider><WorkbenchChrome theme="dark" themePreference="dark" onThemeChange={() => {}} isMacClient={false} showWindowControls={false} onSelectVaultSection={section => selected.push(section)} onOpenQuickSwitcher={() => {}} onOpenSettings={() => {}} externalMcpEnabled={false} onToggleExternalMcp={() => {}} /></TooltipProvider>);
    const trigger = renderer.container.querySelector('button[aria-haspopup="dialog"]');
    assert.ok(trigger);
    await dispatchDomEvent(trigger, new env.window.MouseEvent('click', { bubbles: true }));
    const dialog = env.document.querySelector('[role="dialog"]');
    assert.ok(dialog);
    const buttons = Array.from(dialog.querySelectorAll('button'));
    assert.equal(buttons.length, 8);
    const logs = buttons.find(button => button.textContent === 'vault.nav.logs');
    assert.ok(logs);
    await dispatchDomEvent(logs, new env.window.MouseEvent('click', { bubbles: true }));
    assert.deepEqual(selected, ['logs']);
    assert.equal(env.document.querySelector('[role="dialog"]'), null);
    assert.ok(renderer.container.querySelector('[data-section="workbench-chrome-actions"]')?.classList.contains('app-no-drag'));
  } finally {
    await renderer.unmount();
    for (const key of globals) {
      if (descriptors[key]) Object.defineProperty(globalThis, key, descriptors[key]);
      else Reflect.deleteProperty(globalThis, key);
    }
    restore(); env.cleanup();
  }
});
