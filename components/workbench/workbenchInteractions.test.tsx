import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { createDomRenderer, installDomEnvironment, dispatchDomEvent } from '../test-support/renderReactDom';
import { installTreeEnvironmentMocks } from './testEnvironmentMocks';

test('workbench layer sends shared reorder and workspace insertion callbacks', async () => {
  const env = installDomEnvironment();
  const restore = installTreeEnvironmentMocks();
  const renderer = await createDomRenderer(env.document);
  try {
    const { AppWorkbenchSessionLayer } = await import('../../application/app/AppWorkbenchSessionLayer');
    const calls: unknown[][] = [];
    const noop = () => {};
    await renderer.render(<AppWorkbenchSessionLayer {...({
      enabled: true, hosts: [], customGroups: [], groupConfigs: [],
      sessions: [
        { id: 's1', hostId: 'deleted', hostLabel: 'one', username: 'root', hostname: 'example', status: 'connected' },
        { id: 's2', hostId: 'deleted', hostLabel: 'two', username: 'root', hostname: 'example', status: 'connected', workspaceId: 'ws1' },
      ],
      workspaces: [{ id: 'ws1', title: 'Ops', root: { type: 'pane', id: 'p1', sessionId: 's2' } }],
      editorTabs: [], logViews: [], orderedTabs: ['s1', 'ws1'], showSftpTab: false,
      dynamicTabTitleMode: 'off', switchTabKeyBinding: null,
      onActivateTab: noop, onActivateWorkspaceSession: noop, onCloseSession: noop,
      onCloseLogView: noop, onOpenQuickSwitcher: noop, onRenameSession: noop,
      onReconnectSession: noop, onRenameWorkspace: noop, onCopyWorkspace: noop, onCloseWorkspace: noop,
      onStartSessionDrag: noop, onEndSessionDrag: noop,
      onReorderTabs: (...args: unknown[]) => calls.push(['reorder', ...args]),
      onRemoveSessionFromWorkspace: noop, onAppendHostToWorkspace: noop,
      onAddSessionToWorkspace: (...args: unknown[]) => calls.push(['insert', ...args]),
    } as React.ComponentProps<typeof AppWorkbenchSessionLayer>)} />);
    const workspace = renderer.container.querySelector('[data-tab-id="ws1"]')!;
    assert.ok(workspace);
    Object.defineProperty(workspace, 'getBoundingClientRect', { value: () => ({ top: 0, bottom: 40, height: 40 }) });
    const data = new Map([['tab-reorder-id', 's1'], ['session-id', 's1']]);
    const drop = async (clientY: number) => {
      const event = new env.window.MouseEvent('drop', { bubbles: true, cancelable: true, clientY });
      Object.defineProperty(event, 'dataTransfer', { value: { types: Array.from(data.keys()), getData: (key: string) => data.get(key) ?? '' } });
      await dispatchDomEvent(workspace, event);
    };
    await drop(2);
    await drop(20);
    assert.deepEqual(calls, [
      ['reorder', 's1', 'ws1', 'before'],
      ['insert', 'ws1', 's1', { direction: 'horizontal', position: 'right' }],
    ]);
  } finally {
    await renderer.unmount();
    restore();
    env.cleanup();
  }
});
