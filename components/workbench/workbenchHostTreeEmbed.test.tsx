import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { createDomRenderer, installDomEnvironment, dispatchDomEvent, flushEffects } from '../test-support/renderReactDom';
import { installTreeEnvironmentMocks } from './testEnvironmentMocks';
import { TERMINAL_THEMES } from '../../infrastructure/config/terminalThemes';

function makeProps(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    enabled: true,
    hosts: [
      {
        id: 'host-1', label: 'web-01', hostname: '10.0.0.1', username: 'root',
        port: 22, protocol: 'ssh', tags: [], os: 'linux', group: '',
      },
      {
        id: 'host-2', label: 'idle-02', hostname: '10.0.0.2', username: 'root',
        port: 22, protocol: 'ssh', tags: [], os: 'linux', group: '',
      },
    ],
    customGroups: [],
    groupConfigs: [],
    sessions: [
      { id: 's1', hostId: 'host-1', hostLabel: 'web-01', username: 'root', hostname: '10.0.0.1', status: 'connected' },
    ],
    workspaces: [],
    editorTabs: [],
    logViews: [],
    orderedTabs: ['s1'],
    showSftpTab: false,
    currentTerminalTheme: TERMINAL_THEMES[0],
    onConnectHost: () => {},
    onNewHost: () => {},
    switchTabKeyBinding: null,
    dynamicTabTitleMode: 'off',
    onActivateTab: () => {}, onActivateWorkspaceSession: () => {}, onCloseSession: () => {},
    onCloseLogView: () => {}, onRenameSession: () => {},
    onReconnectSession: () => {}, onRenameWorkspace: () => {}, onCopyWorkspace: () => {}, onCloseWorkspace: () => {},
    onStartSessionDrag: () => {}, onEndSessionDrag: () => {},
    onReorderTabs: () => {}, onRemoveSessionFromWorkspace: () => {}, onAppendHostToWorkspace: () => {},
    onAddSessionToWorkspace: () => {}, onEditHost: () => {},
    ...overrides,
  };
}

test('workbench sidebar is one merged host+session tree with the toolbar above it', async () => {
  const env = installDomEnvironment();
  const restore = installTreeEnvironmentMocks();
  const renderer = await createDomRenderer(env.document);
  try {
    const { AppWorkbenchSessionLayer } = await import('../../application/app/AppWorkbenchSessionLayer');
    const { terminalHostTreeStore } = await import('../../application/state/terminalHostTreeStore');
    const { activeTabStore } = await import('../../application/state/activeTabStore');
    const { TooltipProvider } = await import('../ui/tooltip');
    const connected: string[] = [];
    activeTabStore.setActiveTabId('s1');
    await renderer.render(<TooltipProvider><AppWorkbenchSessionLayer {...(makeProps({
      onConnectHost: (host: { id: string }) => connected.push(host.id),
    }) as unknown as React.ComponentProps<typeof AppWorkbenchSessionLayer>)} /></TooltipProvider>);

    // One tree: the host toolbar sits above it, and no second column/section.
    assert.ok(renderer.container.querySelector('[data-section="terminal-host-tree-toolbar"]'), 'host toolbar must render above the merged tree');
    assert.equal(renderer.container.querySelector('[data-section="app-workbench-host-tree-section"]'), null, 'no embedded second section');
    assert.equal(renderer.container.querySelector('[data-section="app-host-tree-layer-embedded"]'), null);

    // Session-less hosts stay listed as connectable rows.
    const idleRow = renderer.container.querySelector('[data-host-id="host-2"]');
    assert.ok(idleRow, 'session-less host must appear in the merged tree');

    // Clicking a session-less host row connects it.
    await dispatchDomEvent(idleRow!, new env.window.MouseEvent('click', { bubbles: true }));
    assert.deepEqual(connected, ['host-2']);

    // The active session's host row exists with its session nested.
    assert.ok(renderer.container.querySelector('[data-host-id="host-1"]'));
    assert.ok(renderer.container.querySelector('[data-section="workbench-tree-session"]'), 'session row nested under its host');

    // The merged tree must not publish a host-tree layout width, or content
    // surfaces would offset for a tree that is a plain flex sibling.
    assert.equal(terminalHostTreeStore.getLayoutWidth(), 0);
  } finally {
    await renderer.unmount();
    restore();
    env.cleanup();
  }
});

test('merged tree filtering keeps session hosts visible and hides non-matching connect hosts', async () => {
  const env = installDomEnvironment();
  const restore = installTreeEnvironmentMocks();
  const renderer = await createDomRenderer(env.document);
  try {
    const { AppWorkbenchSessionLayer } = await import('../../application/app/AppWorkbenchSessionLayer');
    const { TooltipProvider } = await import('../ui/tooltip');
    await renderer.render(<TooltipProvider><AppWorkbenchSessionLayer {...(makeProps() as unknown as React.ComponentProps<typeof AppWorkbenchSessionLayer>)} /></TooltipProvider>);
    // The search input lives in the expandable panel *next to* the toolbar
    // row, not inside it, so query the container directly.
    const input = renderer.container.querySelector('input');
    assert.ok(input, 'search input must be reachable in the toolbar');
  } finally {
    await renderer.unmount();
    restore();
    env.cleanup();
  }
});

test('new-group inline editor appears on the merged tree and commits via vault actions', async () => {
  const env = installDomEnvironment();
  const restore = installTreeEnvironmentMocks();
  const descriptors = Object.getOwnPropertyDescriptors(globalThis);
  Object.assign(globalThis, { requestAnimationFrame: () => 1, cancelAnimationFrame: () => {} });
  const renderer = await createDomRenderer(env.document);
  const { hostTreeInlineGroupEditStore } = await import('../../application/state/hostTreeInlineGroupEditStore');
  const { vaultHostTreeActionsStore } = await import('../../application/state/vaultHostTreeActionsStore');
  try {
    const { AppWorkbenchSessionLayer } = await import('../../application/app/AppWorkbenchSessionLayer');
    const { TooltipProvider } = await import('../ui/tooltip');
    const committed: string[] = [];
    vaultHostTreeActionsStore.setActions({
      onDeleteHost: () => {}, onDuplicateHost: () => {}, onCopyCredentials: () => {},
      onRenameHost: () => {}, onNewGroup: () => {}, onRenameGroup: () => {},
      onDeleteGroup: () => {},
      commitInlineGroupRename: (name: string) => {
        committed.push(name);
        hostTreeInlineGroupEditStore.clear();
        return true;
      },
      cancelInlineGroupEdit: () => hostTreeInlineGroupEditStore.clear(),
      commitInlineHostRename: () => {}, cancelInlineHostEdit: () => {},
      moveHostToGroup: () => {}, moveGroup: () => {}, reorderHost: () => {}, reorderGroup: () => false,
    });
    await renderer.render(<TooltipProvider><AppWorkbenchSessionLayer {...(makeProps({
      customGroups: ['New Group'],
    }) as unknown as React.ComponentProps<typeof AppWorkbenchSessionLayer>)} /></TooltipProvider>);

    // Simulate what startInlineNewGroup leaves behind: an empty custom group
    // plus an active inline edit for it.
    hostTreeInlineGroupEditStore.startEdit({ groupPath: 'New Group', initialName: 'New Group', isNew: true });
    await flushEffects();
    const input = renderer.container.querySelector<HTMLInputElement>('input[data-inline-group-edit="true"]');
    assert.ok(input, 'inline new-group editor must render on the merged tree');
    assert.equal(input!.value, 'New Group');

    // Enter commits through the registered vault action.
    await dispatchDomEvent(input!, new env.window.KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    await flushEffects();
    assert.deepEqual(committed, ['New Group']);
    assert.equal(renderer.container.querySelector('input[data-inline-group-edit="true"]'), null, 'editor closes after commit');
  } finally {
    vaultHostTreeActionsStore.setActions(null);
    await renderer.unmount();
    for (const key of ['requestAnimationFrame', 'cancelAnimationFrame'] as const) {
      if (descriptors[key]) Object.defineProperty(globalThis, key, descriptors[key]);
      else Reflect.deleteProperty(globalThis, key);
    }
    restore();
    env.cleanup();
  }
});

test('filterMergedTreeHosts prunes the connect list but never session hosts', async () => {
  const { filterMergedTreeHosts } = await import('../../domain/sessionGroupTree');
  const hosts = [
    { id: 'host-1', label: 'web-01', tags: ['edge'] },
    { id: 'host-2', label: 'idle-02', tags: ['edge'] },
    { id: 'host-3', label: 'db-03', tags: [] },
  ];
  const sessionHostIds = new Set(['host-1']);

  // No filter: everything passes through untouched.
  assert.deepEqual(
    filterMergedTreeHosts(hosts, sessionHostIds, { searchTerm: '', selectedTags: [] }),
    hosts,
  );

  // Search that matches nothing: only the session host survives.
  assert.deepEqual(
    filterMergedTreeHosts(hosts, sessionHostIds, { searchTerm: 'zzz-no-match', selectedTags: [] }).map((host) => host.id),
    ['host-1'],
  );

  // Tag filter keeps matching connect hosts and session hosts.
  assert.deepEqual(
    filterMergedTreeHosts(hosts, sessionHostIds, { searchTerm: '', selectedTags: ['edge'] }).map((host) => host.id),
    ['host-1', 'host-2'],
  );

  // Label search reaches the connect list; the session host always stays.
  assert.deepEqual(
    filterMergedTreeHosts(hosts, sessionHostIds, { searchTerm: 'idle', selectedTags: [] }).map((host) => host.id),
    ['host-1', 'host-2'],
  );
});
