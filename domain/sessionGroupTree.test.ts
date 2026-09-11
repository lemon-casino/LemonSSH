import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { buildSessionGroupTree, flattenSessionGroupTree } from './sessionGroupTree';
import type { HostProtocol } from './models/connection';
import type {
  BuildSessionGroupTreeOptions,
  SessionGroupTreeNode,
} from './sessionGroupTree';

const makeSession = (
  id: string,
  hostId: string,
  extra: { workspaceId?: string; hiddenFromTabs?: boolean; label?: string } = {},
) => ({
  id,
  hostId,
  label: extra.label ?? `tab-${id}`,
  workspaceId: extra.workspaceId,
  hiddenFromTabs: extra.hiddenFromTabs,
});

const makeHost = (id: string, label: string, group?: string, protocol: HostProtocol = 'ssh') => ({
  id,
  label,
  group,
  protocol,
});

const makeOptions = (
  overrides: Partial<BuildSessionGroupTreeOptions> = {},
): BuildSessionGroupTreeOptions => ({
  sessions: [],
  hosts: [],
  customGroups: [],
  groupConfigs: {},
  logViews: [],
  editorTabs: [],
  fixedItems: [],
  ...overrides,
});

const findNode = (
  root: SessionGroupTreeNode | null,
  id: string,
): SessionGroupTreeNode | undefined => {
  if (!root) return undefined;
  if (root.id === id) return root;
  for (const child of root.children) {
    const found = findNode(child, id);
    if (found) return found;
  }
  return undefined;
};

const childIds = (node: SessionGroupTreeNode | null | undefined): string[] =>
  node ? node.children.map((child) => child.id) : [];

describe('buildSessionGroupTree', () => {
  it('returns empty sections for empty input', () => {
    const sections = buildSessionGroupTree(makeOptions());

    assert.deepEqual(sections.fixed, []);
    assert.equal(sections.groupTree, null);
    assert.equal(sections.ungrouped, null);
    assert.equal(sections.localTerminals, null);
    assert.equal(sections.workspaces, null);
    assert.equal(sections.logs, null);
    assert.equal(sections.editors, null);
    assert.equal(sections.others, null);
  });

  it('maps fixed items to depth-0 section nodes', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        fixedItems: [
          { id: 'vaults', label: 'Vaults' },
          { id: 'sftp', label: 'SFTP' },
        ],
      }),
    );

    assert.equal(sections.fixed.length, 2);
    assert.equal(sections.fixed[0].type, 'section');
    assert.equal(sections.fixed[0].id, 'vaults');
    assert.equal(sections.fixed[0].label, 'Vaults');
    assert.equal(sections.fixed[0].depth, 0);
    assert.deepEqual(sections.fixed[0].children, []);
    assert.equal(sections.fixed[1].id, 'sftp');
  });

  it('builds multi-level nesting with group, host, and session nodes', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s1', 'h-web'),
          makeSession('s2', 'h-web'),
          makeSession('s3', 'h-api'),
          makeSession('s4', 'h-db'),
        ],
        hosts: [
          makeHost('h-web', 'Web Server', 'Prod/Web'),
          makeHost('h-api', 'Api Server', 'Prod/Web/Api'),
          makeHost('h-db', 'Db Server', 'Prod/Db'),
        ],
        customGroups: [{ group: 'Prod/Web' }],
      }),
    );

    const root = sections.groupTree;
    assert.ok(root);
    assert.equal(root.type, 'section');
    assert.equal(root.id, 'sessionGroups');
    assert.equal(root.depth, 0);

    const prod = findNode(root, 'Prod');
    assert.ok(prod);
    assert.equal(prod.type, 'group');
    assert.equal(prod.depth, 1);

    const web = findNode(root, 'Prod/Web');
    assert.ok(web);
    assert.equal(web.type, 'group');
    assert.equal(web.label, 'Web');
    assert.equal(web.depth, 2);

    // Sub-groups render before hosts; both at depth 3.
    assert.deepEqual(childIds(web), ['Prod/Web/Api', 'h-web']);
    const api = findNode(root, 'Prod/Web/Api');
    assert.ok(api);
    assert.equal(api.depth, 3);

    const webHost = findNode(root, 'h-web');
    assert.ok(webHost);
    assert.equal(webHost.type, 'host');
    assert.equal(webHost.depth, 3);
    assert.equal(webHost.hostId, 'h-web');
    assert.equal(webHost.hostLabel, 'Web Server');
    assert.deepEqual(webHost.sessionIds, ['s1', 's2']);

    const sessionNode = findNode(root, 's1');
    assert.ok(sessionNode);
    assert.equal(sessionNode.type, 'session');
    assert.equal(sessionNode.depth, 4);
    assert.equal(sessionNode.sessionId, 's1');
    assert.equal(sessionNode.title, 'tab-s1');
    assert.equal(sessionNode.label, 'tab-s1');
  });

  it('prunes groups and hosts with zero sessions, including empty custom groups', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [makeSession('s1', 'h1')],
        hosts: [
          makeHost('h1', 'Host 1', 'Prod/Web'),
          makeHost('h2', 'Idle Host', 'Prod'),
        ],
        customGroups: [{ group: 'Prod' }, { group: 'Prod/Empty' }, { group: 'Unused' }],
      }),
    );

    const root = sections.groupTree;
    assert.ok(root);
    // "Unused" and "Prod/Empty" custom groups carry no sessions and are pruned.
    assert.deepEqual(childIds(root), ['Prod']);
    const prod = findNode(root, 'Prod');
    assert.ok(prod);
    // Only the branch holding h1 survives; idle host h2 is omitted.
    assert.deepEqual(childIds(prod), ['Prod/Web']);
    const web = findNode(root, 'Prod/Web');
    assert.ok(web);
    assert.deepEqual(childIds(web), ['h1']);
    assert.equal(findNode(root, 'h2'), undefined);
  });

  it('returns a null groupTree when only empty custom groups exist', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        customGroups: [{ group: 'Empty' }],
      }),
    );

    assert.equal(sections.groupTree, null);
  });

  it('excludes hiddenFromTabs sessions from every section', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s-hidden', 'h1', { hiddenFromTabs: true }),
          makeSession('s-visible', 'h1'),
          makeSession('s-hidden-ws', 'h1', { workspaceId: 'ws-1', hiddenFromTabs: true }),
          makeSession('s-hidden-gone', 'deleted-host', { hiddenFromTabs: true }),
        ],
        hosts: [makeHost('h1', 'Host 1', 'Prod')],
      }),
    );

    const webHost = findNode(sections.groupTree, 'h1');
    assert.ok(webHost);
    assert.deepEqual(webHost.sessionIds, ['s-visible']);
    assert.equal(findNode(sections.groupTree, 's-hidden'), undefined);
    assert.equal(sections.workspaces, null);
    assert.equal(sections.others, null);

    const rows = flattenSessionGroupTree(sections, new Set(['Prod', 'h1']));
    assert.equal(rows.some((row) => row.node.id === 's-hidden'), false);
    assert.equal(rows.some((row) => row.node.id === 's-hidden-ws'), false);
    assert.equal(rows.some((row) => row.node.id === 's-hidden-gone'), false);
  });

  it('routes found hosts with a non-empty group into the group tree', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [makeSession('s1', 'h1')],
        hosts: [makeHost('h1', 'Host 1', 'Prod/Web')],
      }),
    );

    assert.ok(findNode(sections.groupTree, 's1'));
    assert.equal(sections.ungrouped, null);
    assert.equal(sections.others, null);
  });

  it('routes hosts with an empty or missing group into the ungrouped section', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s1', 'h-none'),
          makeSession('s2', 'h-blank'),
          makeSession('s3', 'h-space'),
        ],
        hosts: [
          makeHost('h-none', 'No Group'),
          makeHost('h-blank', 'Blank Group', ''),
          makeHost('h-space', 'Whitespace Group', '   '),
        ],
      }),
    );

    const ungrouped = sections.ungrouped;
    assert.ok(ungrouped);
    assert.equal(ungrouped.type, 'section');
    assert.equal(ungrouped.id, 'ungrouped');
    assert.equal(ungrouped.depth, 0);
    assert.deepEqual(childIds(ungrouped), ['h-none', 'h-blank', 'h-space']);
    assert.equal(sections.groupTree, null);
    assert.equal(sections.others, null);

    const hostNode = findNode(ungrouped, 'h-none');
    assert.ok(hostNode);
    assert.equal(hostNode.type, 'host');
    assert.equal(hostNode.depth, 1);
    const sessionNode = findNode(ungrouped, 's1');
    assert.ok(sessionNode);
    assert.equal(sessionNode.depth, 2);
  });

  it('routes local- prefixed sessions into localTerminals even without a host record', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [makeSession('s-local', 'local-shell')],
      }),
    );

    const local = sections.localTerminals;
    assert.ok(local);
    assert.equal(local.id, 'localTerminals');
    // No host record exists, so the session row is direct.
    assert.deepEqual(childIds(local), ['s-local']);
    const sessionNode = findNode(local, 's-local');
    assert.ok(sessionNode);
    assert.equal(sessionNode.type, 'session');
    assert.equal(sessionNode.depth, 1);
    assert.equal(sections.others, null);
  });

  it('routes hosts with protocol local into localTerminals ahead of group routing', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [makeSession('s1', 'h-loc')],
        hosts: [makeHost('h-loc', 'Local Shell', 'Prod', 'local')],
      }),
    );

    const local = sections.localTerminals;
    assert.ok(local);
    // The host record exists, so sessions group under a host node.
    assert.deepEqual(childIds(local), ['h-loc']);
    assert.ok(findNode(local, 's1'));
    // The host's group path is not materialized in the group tree.
    assert.equal(sections.groupTree, null);
  });

  it('routes workspace sessions into the workspaces section with per-workspace groups', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s1', 'h1', { workspaceId: 'ws-1' }),
          makeSession('s2', 'h2', { workspaceId: 'ws-1' }),
          makeSession('s3', 'h2', { workspaceId: 'ws-2' }),
        ],
        hosts: [
          makeHost('h1', 'Host 1', 'Prod'),
          makeHost('h2', 'Host 2', 'Prod'),
        ],
      }),
    );

    const workspaces = sections.workspaces;
    assert.ok(workspaces);
    assert.equal(workspaces.id, 'workspaces');
    assert.deepEqual(childIds(workspaces), ['workspace:ws-1', 'workspace:ws-2']);

    const ws1 = findNode(workspaces, 'workspace:ws-1');
    assert.ok(ws1);
    assert.equal(ws1.type, 'group');
    assert.equal(ws1.label, 'ws-1');
    assert.equal(ws1.depth, 1);
    assert.equal(ws1.sessionCount, 2);
    assert.deepEqual(childIds(ws1), ['s1', 's2']);

    // Workspace sessions leave the group tree even though their hosts are grouped.
    const prod = findNode(sections.groupTree, 'Prod');
    assert.equal(prod, undefined);
    assert.equal(sections.groupTree, null);
  });

  it('routes sessions whose host was deleted into others', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [makeSession('s-gone', 'deleted-host')],
        hosts: [makeHost('h1', 'Host 1', 'Prod')],
      }),
    );

    const others = sections.others;
    assert.ok(others);
    assert.equal(others.id, 'others');
    assert.deepEqual(childIds(others), ['s-gone']);
    assert.ok(findNode(others, 's-gone'));
    assert.equal(sections.groupTree, null);
  });

  it('creates logs and editors sections from log views and editor tabs', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        logViews: [{ id: 'log-1', label: 'app.log' }],
        editorTabs: [{ id: 'ed-1', label: 'notes.md' }],
      }),
    );

    assert.ok(sections.logs);
    assert.equal(sections.logs.id, 'logs');
    const logNode = findNode(sections.logs, 'log-1');
    assert.ok(logNode);
    assert.equal(logNode.type, 'session');
    assert.equal(logNode.label, 'app.log');
    assert.equal(logNode.depth, 1);

    assert.ok(sections.editors);
    const editorNode = findNode(sections.editors, 'ed-1');
    assert.ok(editorNode);
    assert.equal(editorNode.type, 'session');
    assert.equal(editorNode.label, 'notes.md');
  });

  it('aggregates recursive sessionCount across sub-groups and hosts', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s1', 'h-web'),
          makeSession('s2', 'h-web'),
          makeSession('s3', 'h-api'),
          makeSession('s4', 'h-db'),
        ],
        hosts: [
          makeHost('h-web', 'Web', 'Prod/Web'),
          makeHost('h-api', 'Api', 'Prod/Web/Api'),
          makeHost('h-db', 'Db', 'Prod/Db'),
        ],
      }),
    );

    const root = sections.groupTree;
    assert.ok(root);
    const prod = findNode(root, 'Prod');
    assert.equal(prod?.sessionCount, 4);
    const web = findNode(root, 'Prod/Web');
    assert.equal(web?.sessionCount, 3);
    const api = findNode(root, 'Prod/Web/Api');
    assert.equal(api?.sessionCount, 1);
    const db = findNode(root, 'Prod/Db');
    assert.equal(db?.sessionCount, 1);
    // Section wrappers are not group nodes, so they carry no sessionCount;
    // the tree total is the sum of the top-level group counts.
    assert.equal(root.sessionCount, undefined);
    assert.equal(
      root.children.reduce((sum, child) => sum + (child.sessionCount ?? 0), 0),
      4,
    );
  });

  it('orders groups by groupConfigs order ahead of customGroups order', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s-a', 'h-a'),
          makeSession('s-b', 'h-b'),
          makeSession('s-c', 'h-c'),
        ],
        hosts: [
          makeHost('h-a', 'A', 'a'),
          makeHost('h-b', 'B', 'b'),
          makeHost('h-c', 'C', 'c'),
        ],
        customGroups: [{ group: 'b' }, { group: 'a' }, { group: 'c' }],
        groupConfigs: { c: { order: 0 }, a: { order: 1 } },
      }),
    );

    const root = sections.groupTree;
    assert.ok(root);
    assert.deepEqual(childIds(root), ['c', 'a', 'b']);
  });

  it('falls back to customGroups appearance order when no explicit order is set', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s-z', 'h-z'),
          makeSession('s-m', 'h-m'),
        ],
        hosts: [
          makeHost('h-z', 'Z', 'zeta'),
          makeHost('h-m', 'M', 'mu'),
        ],
        customGroups: [{ group: 'zeta' }, { group: 'mu' }],
      }),
    );

    const root = sections.groupTree;
    assert.ok(root);
    // "mu" would win alphabetically, but customGroups order places zeta first.
    assert.deepEqual(childIds(root), ['zeta', 'mu']);
  });

  it('falls back to alphabetical order for ad-hoc groups without order or customGroups entry', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s-y', 'h-y'),
          makeSession('s-x', 'h-x'),
        ],
        hosts: [
          makeHost('h-y', 'Y', 'yellow'),
          makeHost('h-x', 'X', 'xray'),
        ],
      }),
    );

    const root = sections.groupTree;
    assert.ok(root);
    assert.deepEqual(childIds(root), ['xray', 'yellow']);
  });

  it('places curated custom groups ahead of alphabetically-smaller ad-hoc groups', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s-c', 'h-c'),
          makeSession('s-a', 'h-a'),
        ],
        hosts: [
          makeHost('h-c', 'C', 'custom'),
          makeHost('h-a', 'A', 'aaa'),
        ],
        customGroups: [{ group: 'custom' }],
      }),
    );

    const root = sections.groupTree;
    assert.ok(root);
    assert.deepEqual(childIds(root), ['custom', 'aaa']);
  });

  it('does not conflate orphan sessions (no workspace) with host-deleted sessions', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        sessions: [
          // Orphan session: not in any workspace, but its host is alive.
          makeSession('s-orphan', 'h1'),
          // Host-deleted session: also not in any workspace, but must land in others.
          makeSession('s-gone', 'deleted-host'),
          // Host-deleted but inside a workspace: workspace wins over host-deleted.
          makeSession('s-gone-ws', 'deleted-host', { workspaceId: 'ws-9' }),
        ],
        hosts: [makeHost('h1', 'Host 1', 'Prod')],
      }),
    );

    // The orphan session stays in the group tree, never in others or workspaces.
    assert.ok(findNode(sections.groupTree, 's-orphan'));
    assert.equal(findNode(sections.others, 's-orphan'), undefined);
    assert.equal(findNode(sections.workspaces, 's-orphan'), undefined);

    // The host-deleted session lands in others even though it has no workspace.
    assert.ok(findNode(sections.others, 's-gone'));
    assert.equal(findNode(sections.workspaces, 's-gone'), undefined);

    // Workspace membership takes precedence over the host-deleted routing.
    assert.ok(findNode(sections.workspaces, 's-gone-ws'));
    assert.equal(findNode(sections.others, 's-gone-ws'), undefined);
    const ws9 = findNode(sections.workspaces, 'workspace:ws-9');
    assert.ok(ws9);
    assert.equal(ws9.sessionCount, 1);
  });
});

describe('flattenSessionGroupTree', () => {
  const buildSections = () =>
    buildSessionGroupTree(
      makeOptions({
        sessions: [
          makeSession('s1', 'h-web'),
          makeSession('s2', 'h-web'),
          makeSession('s3', 'h-api'),
          makeSession('s-plain', 'h-plain'),
          makeSession('s-local', 'local-shell'),
          makeSession('s-ws', 'h-web', { workspaceId: 'ws-1' }),
          makeSession('s-gone', 'deleted-host'),
        ],
        hosts: [
          makeHost('h-web', 'Web', 'Prod/Web'),
          makeHost('h-api', 'Api', 'Prod/Web/Api'),
          makeHost('h-plain', 'Plain'),
        ],
        logViews: [{ id: 'log-1', label: 'app.log' }],
        editorTabs: [{ id: 'ed-1', label: 'notes.md' }],
        fixedItems: [
          { id: 'vaults', label: 'Vaults' },
          { id: 'sftp', label: 'SFTP' },
        ],
      }),
    );

  it('skips children of collapsed group and host nodes', () => {
    const sections = buildSections();
    const rows = flattenSessionGroupTree(sections, new Set());

    const rowIds = rows.map((row) => row.node.id);
    // Fixed items first.
    assert.deepEqual(rowIds.slice(0, 2), ['vaults', 'sftp']);
    // The group tree section and its collapsed top-level group appear,
    // but nested groups, hosts, and sessions do not.
    assert.equal(rowIds.includes('sessionGroups'), true);
    assert.equal(rowIds.includes('Prod'), true);
    assert.equal(rowIds.includes('Prod/Web'), false);
    assert.equal(rowIds.includes('h-web'), false);
    assert.equal(rowIds.includes('s1'), false);
    // Sections always reveal their direct children, but collapsed hosts
    // and workspace groups still hide theirs.
    assert.equal(rowIds.includes('ungrouped'), true);
    assert.equal(rowIds.includes('h-plain'), true);
    assert.equal(rowIds.includes('s-plain'), false);
    assert.equal(rowIds.includes('s-local'), true);
    assert.equal(rowIds.includes('workspace:ws-1'), true);
    assert.equal(rowIds.includes('s-ws'), false);
    assert.equal(rowIds.includes('log-1'), true);
    assert.equal(rowIds.includes('ed-1'), true);
    assert.equal(rowIds.includes('s-gone'), true);
  });

  it('reveals nested rows for expanded group and host paths', () => {
    const sections = buildSections();
    const rows = flattenSessionGroupTree(
      sections,
      new Set(['Prod', 'Prod/Web', 'h-web']),
    );

    const rowIds = rows.map((row) => row.node.id);
    assert.equal(rowIds.includes('Prod/Web'), true);
    assert.equal(rowIds.includes('h-web'), true);
    assert.equal(rowIds.includes('s1'), true);
    assert.equal(rowIds.includes('s2'), true);
    // The unexpanded sibling branch still shows as a collapsed row,
    // but its sessions stay hidden.
    assert.equal(rowIds.includes('Prod/Web/Api'), true);
    assert.equal(rowIds.includes('s3'), false);
  });

  it('emits rows with depths matching node depth in render order', () => {
    const sections = buildSections();
    const rows = flattenSessionGroupTree(
      sections,
      new Set(['Prod', 'Prod/Web', 'Prod/Web/Api', 'h-web', 'h-plain', 'workspace:ws-1']),
    );

    assert.ok(rows.length > 0);
    for (const row of rows) {
      assert.equal(row.depth, row.node.depth);
    }
    const s1Row = rows.find((row) => row.node.id === 's1');
    assert.ok(s1Row);
    assert.equal(s1Row.depth, 4);
    const prodRow = rows.find((row) => row.node.id === 'Prod');
    assert.ok(prodRow);
    assert.equal(prodRow.depth, 1);
    const vaultsRow = rows.find((row) => row.node.id === 'vaults');
    assert.ok(vaultsRow);
    assert.equal(vaultsRow.depth, 0);
  });

  it('returns only fixed rows when every section is empty', () => {
    const sections = buildSessionGroupTree(
      makeOptions({
        fixedItems: [{ id: 'vaults', label: 'Vaults' }],
      }),
    );
    const rows = flattenSessionGroupTree(sections, new Set());

    assert.equal(rows.length, 1);
    assert.equal(rows[0].node.id, 'vaults');
    assert.equal(rows[0].depth, 0);
  });
});
