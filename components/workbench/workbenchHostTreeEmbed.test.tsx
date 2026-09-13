import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { createDomRenderer, installDomEnvironment, flushEffects, runWithAct } from '../test-support/renderReactDom';
import { installTreeEnvironmentMocks } from './testEnvironmentMocks';
import { TERMINAL_THEMES } from '../../infrastructure/config/terminalThemes';

test('workbench sidebar hosts the host tree inline instead of a second floating column', async () => {
  const env = installDomEnvironment();
  const restore = installTreeEnvironmentMocks();
  const renderer = await createDomRenderer(env.document);
  try {
    const { AppWorkbenchSessionLayer } = await import('../../application/app/AppWorkbenchSessionLayer');
    const { terminalHostTreeStore } = await import('../../application/state/terminalHostTreeStore');
    const { TooltipProvider } = await import('../ui/tooltip');
    terminalHostTreeStore.setIsOpen(true);
    await runWithAct(() => {});
    const noop = () => {};
    await renderer.render(<TooltipProvider><AppWorkbenchSessionLayer {...({
      enabled: true,
      hosts: [{
        id: 'host-1', label: 'web-01', hostname: '10.0.0.1', username: 'root',
        port: 22, protocol: 'ssh', tags: [], os: 'linux', group: '',
      }],
      customGroups: [],
      groupConfigs: [],
      sessions: [],
      workspaces: [],
      editorTabs: [],
      logViews: [],
      orderedTabs: [],
      showSftpTab: false,
      showHostTreeSidebar: true,
      currentTerminalTheme: TERMINAL_THEMES[0],
      followAppTerminalTheme: false,
      themeById: new Map(),
      onConnectHost: noop,
      switchTabKeyBinding: null,
      dynamicTabTitleMode: 'off',
      onActivateTab: noop, onActivateWorkspaceSession: noop, onCloseSession: noop,
      onCloseLogView: noop, onOpenQuickSwitcher: noop, onRenameSession: noop,
      onReconnectSession: noop, onRenameWorkspace: noop, onCopyWorkspace: noop, onCloseWorkspace: noop,
      onStartSessionDrag: noop, onEndSessionDrag: noop,
      onReorderTabs: noop, onRemoveSessionFromWorkspace: noop, onAppendHostToWorkspace: noop,
      onAddSessionToWorkspace: noop, onEditHost: noop,
    } as React.ComponentProps<typeof AppWorkbenchSessionLayer>)} /></TooltipProvider>);

    // The host tree renders inside the session layer column...
    const section = renderer.container.querySelector('[data-section="app-workbench-host-tree-section"]');
    assert.ok(section, 'embedded host tree section missing');
    assert.ok(section.querySelector('[data-section="terminal-host-tree-sidebar"]'), 'host tree sidebar must render inside the section');
    assert.ok(section.querySelector('[data-row-type="host"]'), 'host rows must be reachable from the sidebar');

    // ...and it must not publish a layout width, or content surfaces would
    // offset themselves for a tree that is already a flex sibling.
    assert.equal(terminalHostTreeStore.getLayoutWidth(), 0);

    // Closing the tree removes the section again.
    await runWithAct(() => { terminalHostTreeStore.setIsOpen(false); });
    await flushEffects();
    assert.equal(renderer.container.querySelector('[data-section="app-workbench-host-tree-section"]'), null);
  } finally {
    await renderer.unmount();
    restore();
    env.cleanup();
  }
});
