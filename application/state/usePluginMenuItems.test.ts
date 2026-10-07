import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import {
  PLUGIN_MENU_LOCATIONS,
  selectPluginMenuItems,
  usePluginMenuItems,
} from './usePluginMenuItems';

function plugin(
  id: string,
  menus: Partial<LemonSSHPluginContributionSnapshot['plugins'][number]['menus'][number]>[] = [],
  commands: LemonSSHPluginContributionSnapshot['plugins'][number]['commands'] = [],
): LemonSSHPluginContributionSnapshot['plugins'][number] {
  return {
    id,
    version: '1.0.0',
    displayName: id,
    description: '',
    commands,
    keybindings: [],
    menus: menus.map((menu, index) => ({
      id: `${id}:menu:${index}`,
      command: `${id}.run`,
      location: 'application',
      title: `${id} command`,
      visible: true,
      enabled: true,
      ...menu,
    })),
    settings: [],
    views: [],
  } as LemonSSHPluginContributionSnapshot['plugins'][number];
}

test('plugin menu locations mirror the Go host catalog', () => {
  assert.deepEqual([...PLUGIN_MENU_LOCATIONS], [
    'commandPalette',
    'application',
    'host/context',
    'terminal/context',
    'terminal/toolbar',
    'statusBar',
  ]);
});

test('selectPluginMenuItems keeps only visible menus of the requested location', () => {
  const plugins = [
    plugin('com.example.a', [
      { location: 'application', title: 'App action' },
      { location: 'statusBar', title: 'Status action' },
      { location: 'application', title: 'Hidden action', visible: false },
    ]),
    plugin('com.example.b', [{ location: 'host/context', title: 'Host action' }]),
  ];

  const items = selectPluginMenuItems(plugins, 'application');
  assert.deepEqual(items.map((menu) => menu.title), ['App action']);
  assert.equal(items[0]?.pluginId, 'com.example.a');
});

test('selectPluginMenuItems unions and sorts multi-location requests', () => {
  const plugins = [
    plugin('com.example.a', [
      { location: 'statusBar', title: 'Status second', group: 'status', order: 2 },
      { location: 'terminal/toolbar', title: 'Toolbar first', group: 'status', order: 1 },
    ]),
  ];

  const items = selectPluginMenuItems(plugins, ['terminal/toolbar', 'statusBar']);
  assert.deepEqual(items.map((menu) => menu.title), ['Toolbar first', 'Status second']);
});

test('selectPluginMenuItems orders by group, then order, then id', () => {
  const plugins = [
    plugin('com.example.a', [
      { title: 'zebra', group: 'b', order: 1 },
      { title: 'alpha', group: 'a', order: 9 },
    ]),
  ];
  assert.deepEqual(
    selectPluginMenuItems(plugins, 'application').map((menu) => menu.title),
    ['alpha', 'zebra'],
  );

  const sameGroup = [
    plugin('com.example.b', [
      { title: 'declared-second' },
      { title: 'declared-first' },
    ]),
  ];
  // Same group and order: the id tie-break keeps placements stable.
  assert.deepEqual(
    selectPluginMenuItems(sameGroup, 'application').map((menu) => menu.title),
    ['declared-second', 'declared-first'],
  );
});

test('selectPluginMenuItems inherits the referenced command icon', () => {
  const commandIcon = { kind: 'theme', name: 'play' } as const;
  const plugins = [
    plugin(
      'com.example.a',
      [{ title: 'With icon' }],
      [{ id: 'com.example.a.run', title: 'Run', enabled: true, icon: commandIcon }],
    ),
  ];

  assert.equal(selectPluginMenuItems(plugins, 'application')[0]?.icon, commandIcon);
});

test('selectPluginMenuItems yields nothing for empty or unrelated plugins', () => {
  assert.deepEqual(selectPluginMenuItems([], 'statusBar'), []);
  assert.deepEqual(selectPluginMenuItems([plugin('com.example.a')], 'statusBar'), []);
});

function Probe(props: { location: Parameters<typeof selectPluginMenuItems>[1] }) {
  const { items, available, loading, executeCommand } = usePluginMenuItems(props.location);
  return React.createElement(
    'span',
    null,
    `${String(available)}:${String(loading)}:${items.length}:${typeof executeCommand}`,
  );
}

test('usePluginMenuItems fails closed without a bridge and still yields a stable executor', () => {
  assert.equal(
    renderToStaticMarkup(React.createElement(Probe, { location: 'application' })),
    '<span>false:true:0:function</span>',
  );
});
