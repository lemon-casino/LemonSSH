import type { GroupConfig, GroupNode, Host } from './models/connection';
import type { TerminalSession } from './models/terminal';
import { buildHostGroupTree } from './hostGroupTree';

/**
 * Session input for the session group tree.
 *
 * Structurally derived from `TerminalSession`; the display `label` is resolved
 * by the caller (customName / dynamicTitle / host label precedence) because the
 * domain model does not carry a single ready-to-render label.
 */
export type SessionGroupTreeSessionInput = Pick<TerminalSession, 'id' | 'hostId'> &
  Partial<Pick<TerminalSession, 'workspaceId' | 'hiddenFromTabs'>> & {
    label: string;
  };

/**
 * Host input for the session group tree. Structurally derived from `Host`;
 * `buildHostGroupTree` only reads `id` / `label` / `group`.
 */
export type SessionGroupTreeHostInput = Pick<Host, 'id' | 'label'> &
  Partial<Pick<Host, 'group' | 'protocol'>>;

export type SessionGroupTreeNodeType = 'group' | 'host' | 'session' | 'section';

export interface SessionGroupTreeNode {
  type: SessionGroupTreeNodeType;
  /** Group path, host id, session id, section name, or `workspace:<id>`. */
  id: string;
  label: string;
  depth: number;
  children: SessionGroupTreeNode[];
  /** Group nodes only: recursive session total including sub-groups. */
  sessionCount?: number;
  /** Host nodes only. */
  hostId?: string;
  hostLabel?: string;
  sessionIds?: string[];
  /** Session nodes only. */
  sessionId?: string;
  title?: string;
}

export interface SessionGroupTreeSections {
  /** Caller-provided fixed entries (e.g. Vaults, SFTP) rendered at depth 0. */
  fixed: SessionGroupTreeNode[];
  /** Root of the pruned group tree; a section wrapping the top-level groups. */
  groupTree: SessionGroupTreeNode | null;
  ungrouped: SessionGroupTreeNode | null;
  localTerminals: SessionGroupTreeNode | null;
  workspaces: SessionGroupTreeNode | null;
  logs: SessionGroupTreeNode | null;
  editors: SessionGroupTreeNode | null;
  others: SessionGroupTreeNode | null;
}

export interface SessionGroupTreeFlatRow {
  node: SessionGroupTreeNode;
  depth: number;
}

export interface BuildSessionGroupTreeOptions {
  sessions: SessionGroupTreeSessionInput[];
  hosts: SessionGroupTreeHostInput[];
  /** Ordered custom groups; each entry's `group` is a `/`-separated path. */
  customGroups: Array<{ group: string }>;
  /** Per-path group configuration; only `order` participates here. */
  groupConfigs: Record<string, { order?: number }>;
  logViews: Array<{ id: string; label: string }>;
  editorTabs: Array<{ id: string; label: string }>;
  fixedItems: Array<{ id: string; label: string }>;
}

// Stable section identifiers; also used as node ids so expandedPaths keys stay unique.
const GROUP_TREE_SECTION_ID = 'sessionGroups';
const UNGROUPED_SECTION_ID = 'ungrouped';
const LOCAL_SECTION_ID = 'localTerminals';
const WORKSPACES_SECTION_ID = 'workspaces';
const LOGS_SECTION_ID = 'logs';
const EDITORS_SECTION_ID = 'editors';
const OTHERS_SECTION_ID = 'others';

const LOCAL_HOST_ID_PREFIX = 'local-';

const isEmptyGroupPath = (group: string | undefined | null): boolean =>
  !group || group.trim().length === 0;

const isLocalSession = (
  session: SessionGroupTreeSessionInput,
  host: SessionGroupTreeHostInput | undefined,
): boolean => session.hostId.startsWith(LOCAL_HOST_ID_PREFIX) || host?.protocol === 'local';

const buildSessionNode = (
  session: SessionGroupTreeSessionInput,
  depth: number,
): SessionGroupTreeNode => ({
  type: 'session',
  id: session.id,
  label: session.label,
  depth,
  children: [],
  sessionId: session.id,
  title: session.label,
});

const buildHostNode = (
  host: SessionGroupTreeHostInput,
  hostSessions: SessionGroupTreeSessionInput[],
  depth: number,
): SessionGroupTreeNode => ({
  type: 'host',
  id: host.id,
  label: host.label,
  depth,
  children: hostSessions.map((session) => buildSessionNode(session, depth + 1)),
  hostId: host.id,
  hostLabel: host.label,
  sessionIds: hostSessions.map((session) => session.id),
});

const buildSectionNode = (
  id: string,
  label: string,
  children: SessionGroupTreeNode[],
): SessionGroupTreeNode | null =>
  children.length > 0 ? { type: 'section', id, label, depth: 0, children } : null;

const countTreeSessions = (node: SessionGroupTreeNode): number => {
  if (node.type === 'session') return 1;
  return node.children.reduce((sum, child) => sum + countTreeSessions(child), 0);
};

/**
 * Groups sessions by host group paths into sections for the session sidebar.
 *
 * Routing precedence (first match wins, after excluding `hiddenFromTabs`):
 * 1. local terminal   - hostId starts with `local-` OR host.protocol === 'local'
 * 2. workspace        - session.workspaceId is non-empty
 * 3. others           - hostId not found in hosts (host deleted)
 * 4. group tree       - host found with a non-empty `group` path
 * 5. ungrouped        - host found with an empty `group`
 *
 * Note: "orphan sessions" (sessions without a workspace) are NOT the same as
 * "host deleted" sessions. A session without a workspace whose host still
 * exists lands in the group tree / ungrouped / local sections, never in
 * `others`; `others` is exclusively for sessions whose host is gone.
 *
 * Group hierarchy, host placement, and trimming are delegated to
 * `buildHostGroupTree`; empty groups and hosts (zero sessions) are pruned.
 */
export function buildSessionGroupTree(
  options: BuildSessionGroupTreeOptions,
): SessionGroupTreeSections {
  const visibleSessions = options.sessions.filter(
    (session) => session.hiddenFromTabs !== true,
  );

  const hostById = new Map<string, SessionGroupTreeHostInput>();
  for (const host of options.hosts) {
    // First occurrence wins so duplicate ids stay deterministic.
    if (!hostById.has(host.id)) hostById.set(host.id, host);
  }

  const groupedSessions: SessionGroupTreeSessionInput[] = [];
  const ungroupedSessions: SessionGroupTreeSessionInput[] = [];
  const localSessions: SessionGroupTreeSessionInput[] = [];
  const workspaceSessions: SessionGroupTreeSessionInput[] = [];
  const otherSessions: SessionGroupTreeSessionInput[] = [];

  for (const session of visibleSessions) {
    const host = hostById.get(session.hostId);
    if (isLocalSession(session, host)) {
      localSessions.push(session);
    } else if (session.workspaceId) {
      workspaceSessions.push(session);
    } else if (!host) {
      otherSessions.push(session);
    } else if (!isEmptyGroupPath(host.group)) {
      groupedSessions.push(session);
    } else {
      ungroupedSessions.push(session);
    }
  }

  // --- Group tree (reuses buildHostGroupTree for hierarchy + host placement) ---
  const sessionsByHostId = new Map<string, SessionGroupTreeSessionInput[]>();
  for (const session of groupedSessions) {
    const list = sessionsByHostId.get(session.hostId);
    if (list) list.push(session);
    else sessionsByHostId.set(session.hostId, [session]);
  }
  const groupedHosts = options.hosts.filter(
    (host) => (sessionsByHostId.get(host.id)?.length ?? 0) > 0,
  );

  const customGroupPaths: string[] = [];
  for (const entry of options.customGroups) {
    const group = entry.group?.trim();
    if (group && !customGroupPaths.includes(group)) customGroupPaths.push(group);
  }
  const groupConfigList: GroupConfig[] = Object.entries(options.groupConfigs).map(
    ([path, config]) => ({ path, order: config?.order }),
  );

  const groupOrderByPath = new Map<string, number>();
  for (const config of groupConfigList) {
    if (typeof config.order === 'number' && Number.isFinite(config.order)) {
      groupOrderByPath.set(config.path, config.order);
    }
  }
  const customGroupIndex = new Map<string, number>();
  customGroupPaths.forEach((path, index) => {
    if (!customGroupIndex.has(path)) customGroupIndex.set(path, index);
  });

  // Group ordering: explicit groupConfigs[path].order, then customGroups
  // appearance order, then alphabetical by group name.
  const compareGroupNodes = (a: GroupNode, b: GroupNode): number => {
    const orderA = groupOrderByPath.get(a.path);
    const orderB = groupOrderByPath.get(b.path);
    if (orderA !== undefined && orderB !== undefined && orderA !== orderB) {
      return orderA - orderB;
    }
    if (orderA !== undefined) return -1;
    if (orderB !== undefined) return 1;
    const indexA = customGroupIndex.get(a.path);
    const indexB = customGroupIndex.get(b.path);
    if (indexA !== undefined && indexB !== undefined && indexA !== indexB) {
      return indexA - indexB;
    }
    if (indexA !== undefined) return -1;
    if (indexB !== undefined) return 1;
    if (a.name === b.name) return 0;
    return a.name < b.name ? -1 : 1;
  };

  // buildHostGroupTree only reads id/label/group, which the narrowed host
  // input guarantees; the cast keeps the public option shape structural.
  const { groupTree } = buildHostGroupTree(
    groupedHosts as unknown as Host[],
    customGroupPaths,
    groupConfigList,
  );

  const hasSessions = (node: GroupNode): boolean =>
    node.hosts.length > 0 || Object.values(node.children).some(hasSessions);

  const convertGroupNode = (
    node: GroupNode,
    depth: number,
  ): SessionGroupTreeNode => {
    const children: SessionGroupTreeNode[] = [];
    const survivingChildren = Object.values(node.children)
      .filter(hasSessions)
      .sort(compareGroupNodes);
    for (const child of survivingChildren) {
      children.push(convertGroupNode(child, depth + 1));
    }
    for (const host of node.hosts) {
      const hostSessions = sessionsByHostId.get(host.id) ?? [];
      if (hostSessions.length === 0) continue;
      children.push(buildHostNode(host, hostSessions, depth + 1));
    }
    const groupNode: SessionGroupTreeNode = {
      type: 'group',
      id: node.path,
      label: node.name,
      depth,
      children,
    };
    groupNode.sessionCount = countTreeSessions(groupNode);
    return groupNode;
  };

  // --- Flat sections: bucket sessions per host (first-appearance order). ---
  const buildHostBucketChildren = (
    sessions: SessionGroupTreeSessionInput[],
    depth: number,
  ): SessionGroupTreeNode[] => {
    const byHostId = new Map<string, SessionGroupTreeSessionInput[]>();
    for (const session of sessions) {
      const list = byHostId.get(session.hostId);
      if (list) list.push(session);
      else byHostId.set(session.hostId, [session]);
    }
    const nodes: SessionGroupTreeNode[] = [];
    for (const [hostId, hostSessions] of byHostId) {
      const host = hostById.get(hostId);
      if (host) {
        nodes.push(buildHostNode(host, hostSessions, depth));
      } else {
        // No host record (e.g. local shell ids): render sessions directly.
        for (const session of hostSessions) {
          nodes.push(buildSessionNode(session, depth));
        }
      }
    }
    return nodes;
  };

  const workspaceBuckets = new Map<string, SessionGroupTreeSessionInput[]>();
  for (const session of workspaceSessions) {
    const workspaceId = session.workspaceId as string;
    const list = workspaceBuckets.get(workspaceId);
    if (list) list.push(session);
    else workspaceBuckets.set(workspaceId, [session]);
  }
  const workspaceChildren: SessionGroupTreeNode[] = [];
  for (const [workspaceId, bucket] of workspaceBuckets) {
    const sessionNodes = bucket.map((session) => buildSessionNode(session, 2));
    const workspaceNode: SessionGroupTreeNode = {
      type: 'group',
      id: `workspace:${workspaceId}`,
      label: workspaceId,
      depth: 1,
      children: sessionNodes,
    };
    workspaceNode.sessionCount = countTreeSessions(workspaceNode);
    workspaceChildren.push(workspaceNode);
  }

  const toLeafSessionNodes = (
    entries: Array<{ id: string; label: string }>,
    depth: number,
  ): SessionGroupTreeNode[] =>
    entries.map((entry) => ({
      type: 'session',
      id: entry.id,
      label: entry.label,
      depth,
      children: [],
      sessionId: entry.id,
      title: entry.label,
    }));

  return {
    fixed: options.fixedItems.map((item) => ({
      type: 'section' as const,
      id: item.id,
      label: item.label,
      depth: 0,
      children: [],
    })),
    groupTree: buildSectionNode(
      GROUP_TREE_SECTION_ID,
      'Sessions',
      groupTree.filter(hasSessions).sort(compareGroupNodes).map((node) => convertGroupNode(node, 1)),
    ),
    ungrouped: buildSectionNode(
      UNGROUPED_SECTION_ID,
      'Ungrouped',
      buildHostBucketChildren(ungroupedSessions, 1),
    ),
    localTerminals: buildSectionNode(
      LOCAL_SECTION_ID,
      'Local',
      buildHostBucketChildren(localSessions, 1),
    ),
    workspaces: buildSectionNode(WORKSPACES_SECTION_ID, 'Workspaces', workspaceChildren),
    logs: buildSectionNode(LOGS_SECTION_ID, 'Logs', toLeafSessionNodes(options.logViews, 1)),
    editors: buildSectionNode(
      EDITORS_SECTION_ID,
      'Editors',
      toLeafSessionNodes(options.editorTabs, 1),
    ),
    others: buildSectionNode(OTHERS_SECTION_ID, 'Others', buildHostBucketChildren(otherSessions, 1)),
  };
}

/**
 * Flattens the section trees into rows for virtualized rendering.
 *
 * Children of `group` and `host` nodes are skipped unless the node's id is in
 * `expandedPaths`. Section nodes always reveal their children.
 */
export function getSessionTreeAncestorIds(sections: SessionGroupTreeSections, tabId: string): string[] {
  const visit = (node: SessionGroupTreeNode, ancestors: string[]): string[] | null => {
    const branch = node.type === 'group' || node.type === 'host';
    const path = branch ? [...ancestors, node.id] : ancestors;
    if (node.id === tabId || (node.type === 'group' && node.id === `workspace:${tabId}`)) return path;
    for (const child of node.children) {
      const found = visit(child, path);
      if (found) return found;
    }
    return null;
  };
  for (const root of [...sections.fixed, sections.groupTree, sections.ungrouped, sections.localTerminals, sections.workspaces, sections.logs, sections.editors, sections.others]) {
    if (!root) continue;
    const found = visit(root, []);
    if (found) return found;
  }
  return [];
}

export function flattenSessionGroupTree(
  sections: SessionGroupTreeSections,
  expandedPaths: Set<string>,
): SessionGroupTreeFlatRow[] {
  const rows: SessionGroupTreeFlatRow[] = [];
  const walk = (node: SessionGroupTreeNode) => {
    rows.push({ node, depth: node.depth });
    if (node.children.length === 0) return;
    if ((node.type === 'group' || node.type === 'host') && !expandedPaths.has(node.id)) {
      return;
    }
    for (const child of node.children) walk(child);
  };

  for (const node of sections.fixed) walk(node);
  const sectionNodes = [
    sections.groupTree,
    sections.ungrouped,
    sections.localTerminals,
    sections.workspaces,
    sections.logs,
    sections.editors,
    sections.others,
  ];
  for (const section of sectionNodes) {
    if (section) walk(section);
  }
  return rows;
}
