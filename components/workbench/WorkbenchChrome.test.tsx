import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { createDomRenderer, installDomEnvironment, dispatchDomEvent } from '../test-support/renderReactDom';
import { installTreeEnvironmentMocks } from './testEnvironmentMocks';

test('compact workbench menu toggles inline in the bar and collapses after navigation', async () => {
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
    // Expanded by default: the sections render inline in the bar.
    const toggle = renderer.container.querySelector('button[aria-expanded]');
    assert.ok(toggle, 'compact menu toggle missing');
    assert.equal(toggle.getAttribute('aria-expanded'), 'true');
    const inlineNav = renderer.container.querySelector('[data-section="workbench-chrome-inline-nav"]');
    assert.ok(inlineNav, 'expanded sections must render inside the bar, not a dialog');
    assert.equal(env.document.querySelector('[role="dialog"]'), null);
    const buttons = Array.from(inlineNav.querySelectorAll('button'));
    assert.equal(buttons.length, 8);
    const logs = buttons.find(button => button.textContent === 'vault.nav.logs');
    assert.ok(logs);
    // The toggle keeps breathing room from the first section.
    assert.notEqual(env.window.getComputedStyle(inlineNav).marginLeft, '0px');

    // First click collapses to the toggle only.
    await dispatchDomEvent(toggle, new env.window.MouseEvent('click', { bubbles: true }));
    assert.equal(toggle.getAttribute('aria-expanded'), 'false');
    assert.equal(renderer.container.querySelector('[data-section="workbench-chrome-inline-nav"]'), null);

    // Second click expands again.
    await dispatchDomEvent(toggle, new env.window.MouseEvent('click', { bubbles: true }));
    assert.equal(toggle.getAttribute('aria-expanded'), 'true');

    // Navigate: the selection lands and the bar collapses.
    const logsAgain = Array.from(
      renderer.container.querySelectorAll('[data-section="workbench-chrome-inline-nav"] button'),
    ).find(button => button.textContent === 'vault.nav.logs');
    assert.ok(logsAgain);
    await dispatchDomEvent(logsAgain, new env.window.MouseEvent('click', { bubbles: true }));
    assert.deepEqual(selected, ['logs']);
    assert.equal(renderer.container.querySelector('[data-section="workbench-chrome-inline-nav"]'), null);
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
