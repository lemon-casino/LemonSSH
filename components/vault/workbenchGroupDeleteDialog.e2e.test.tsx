/**
 * End-to-end reproduction for: workbench layout -> right-click group -> delete
 * dialog with "also delete all hosts" -> confirm does nothing.
 *
 * The harness drives the real store chain:
 *   vaultHostTreeActionsStore.onDeleteGroup
 *     -> hostTreeInlineGroupDeleteStore.open
 *     -> HostTreeGroupDeleteDialog (real component, real Radix dialog)
 *     -> onConfirmDelete(path, deleteHosts)
 *
 * It asserts the confirm callback actually fires with deleteHosts=true, which
 * is the only thing the dialog is responsible for. A failure here means the
 * dialog itself swallows the click (the regression we are hunting).
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import {
  createDomRenderer,
  dispatchDomEvent,
  flushEffects,
  installDomEnvironment,
} from '../test-support/renderReactDom';
import { installTreeEnvironmentMocks } from '../workbench/testEnvironmentMocks';

test('group delete dialog fires onConfirmDelete with the checkbox value', async () => {
  const env = installDomEnvironment();
  const restore = installTreeEnvironmentMocks();
  // Radix focus-scope walks the document with NodeFilter/Node constants, which
  // the shared DOM harness does not install.
  const globalWithDom = globalThis as Record<string, unknown>;
  const dom = env.window as unknown as Record<string, unknown>;
  for (const key of ['NodeFilter', 'CSS', 'DocumentFragment', 'HTMLInputElement', 'HTMLButtonElement'] as const) {
    if (dom[key] !== undefined) globalWithDom[key] = dom[key];
  }
  const renderer = await createDomRenderer(env.document);
  try {
    const { HostTreeGroupDeleteDialog } = await import('../host/HostTreeGroupDeleteDialog');
    const { hostTreeInlineGroupDeleteStore } = await import(
      '../../application/state/hostTreeInlineGroupDeleteStore'
    );
    const { TooltipProvider } = await import('../ui/tooltip');

    const confirmed: Array<{ path: string; deleteHosts: boolean }> = [];
    await renderer.render(
      React.createElement(
        TooltipProvider,
        null,
        React.createElement(HostTreeGroupDeleteDialog, {
          onConfirmDelete: (path: string, deleteHosts: boolean) => {
            confirmed.push({ path, deleteHosts });
          },
        }),
      ),
    );

    // Open the dialog the way the workbench tree does.
    await flushEffects();
    hostTreeInlineGroupDeleteStore.open('Prod/Web');
    await flushEffects();

    // The dialog portal must have rendered the path row.
    const dialogText = renderer.container.ownerDocument.body.textContent ?? '';
    assert.match(dialogText, /Prod\/Web/, 'dialog must show the clicked group path');

    // Tick "also delete all hosts in this group".
    const checkbox = renderer.container.ownerDocument.body.querySelector<HTMLInputElement>(
      'input[type="checkbox"]',
    );
    assert.ok(checkbox, 'delete-hosts checkbox must render for an unmanaged group');
    await dispatchDomEvent(checkbox!, new env.window.MouseEvent('click', { bubbles: true }));
    await flushEffects();
    assert.equal(checkbox!.checked, true, 'checkbox must reflect the click');

    // Click the destructive confirm button.
    const buttons = Array.from(
      renderer.container.ownerDocument.body.querySelectorAll('button'),
    );
    const deleteButton = buttons.find((button) => button.getAttribute('data-variant') === 'destructive')
      ?? buttons.find((button) => /delete|删除/i.test(button.textContent ?? ''));
    assert.ok(deleteButton, 'delete button must render');

    await dispatchDomEvent(
      deleteButton!,
      new env.window.MouseEvent('click', { bubbles: true, cancelable: true }),
    );
    await flushEffects();

    assert.deepEqual(
      confirmed,
      [{ path: 'Prod/Web', deleteHosts: true }],
      'confirm must reach onConfirmDelete with deleteHosts=true',
    );
  } finally {
    await renderer.unmount();
    restore();
    env.cleanup();
  }
});
