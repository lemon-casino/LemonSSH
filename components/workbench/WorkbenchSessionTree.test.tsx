import assert from "node:assert/strict";
import test from "node:test";
import React from "react";

import {
  createDomRenderer,
  dispatchDomEvent,
  flushEffects,
  installDomEnvironment,
} from "../test-support/renderReactDom.tsx";
import { installTreeEnvironmentMocks } from "./testEnvironmentMocks.ts";

import type { Host, TerminalSession, Workspace } from "../../types";

const noop = () => {};

/**
 * Dynamic import: terminalHostTreeStore (a transitive dependency of the tree)
 * reads localStorage at module load, so the mock must be installed first.
 */
async function loadComponentModule() {
  const [component, domain] = await Promise.all([
    import("./WorkbenchSessionTree.tsx"),
    import("../../domain/sessionGroupTree.ts"),
  ]);
  return {
    WorkbenchSessionTree: component.WorkbenchSessionTree,
    buildSessionGroupTree: domain.buildSessionGroupTree,
  };
}

function clickElement(
  env: ReturnType<typeof installDomEnvironment>,
  element: Element,
) {
  return dispatchDomEvent(
    element,
    new env.window.MouseEvent("click", { bubbles: true, cancelable: true }),
  );
}

function makeHost(overrides: Partial<Host> & { id: string; label: string }): Host {
  return {
    group: undefined,
    protocol: "ssh",
    ...overrides,
  } as Host;
}

function makeSession(
  overrides: Partial<TerminalSession> & { id: string; hostId: string },
): TerminalSession {
  return {
    status: "connected",
    workspaceId: null,
    hiddenFromTabs: false,
    ...overrides,
  } as TerminalSession;
}

type SectionsBuilder = (
  sessions: TerminalSession[],
  hosts: Host[],
) => import("../../domain/sessionGroupTree").SessionGroupTreeSections;

function makeSectionsBuilder(buildSessionGroupTree: Awaited<ReturnType<typeof loadComponentModule>>["buildSessionGroupTree"]): SectionsBuilder {
  return (sessions, hosts) =>
    buildSessionGroupTree({
      sessions: sessions.map((session) => ({
        id: session.id,
        hostId: session.hostId,
        workspaceId: session.workspaceId || undefined,
        hiddenFromTabs: session.hiddenFromTabs === true || undefined,
        label: session.customName || session.hostLabel || session.id,
      })),
      hosts: hosts.map((host) => ({
        id: host.id,
        label: host.label,
        group: host.group,
        protocol: host.protocol,
      })),
      customGroups: [],
      groupConfigs: {},
      logViews: [],
      editorTabs: [],
      fixedItems: [
        { id: "vault", label: "Vaults" },
        { id: "sftp", label: "SFTP" },
      ],
    });
}

type Renderer = Awaited<ReturnType<typeof createDomRenderer>>;

type TreeProps = Record<string, unknown>;

function renderTree(
  WorkbenchSessionTree: Awaited<ReturnType<typeof loadComponentModule>>["WorkbenchSessionTree"],
  renderer: Renderer,
  props: TreeProps,
) {
  return renderer.render(
    React.createElement(WorkbenchSessionTree, props as never),
  );
}

test("workbench tree renders fixed entries, group badge and pruned sessions", async () => {
  const env = installDomEnvironment();
  const restoreEnv = installTreeEnvironmentMocks();

  try {
    const { WorkbenchSessionTree, buildSessionGroupTree } = await loadComponentModule();
    const buildSections = makeSectionsBuilder(buildSessionGroupTree);
    const renderer = await createDomRenderer(env.document);
    // Keep the tree shallow: the stubbed ResizeObserver yields a zero-height
    // viewport, so only the first rows of the virtual window are rendered.
    const hosts = [
      makeHost({ id: "h1", label: "web-01", group: "Prod" }),
    ];
    const sessions = [
      makeSession({ id: "s1", hostId: "h1", customName: "web-01 root" }),
      makeSession({ id: "s2", hostId: "h1", customName: "web-01 root 2" }),
    ];
    const sections = buildSections(sessions, hosts);

    await renderTree(WorkbenchSessionTree, renderer, {
      sections,
      // Host rows gate their sessions behind expansion, just like groups.
      expandedPaths: new Set(["Prod", "h1"]),
      fixedIds: new Set(["vault", "sftp"]),
      activeTabId: "s1",
      sessions,
      workspaces: [] as Workspace[],
      logViews: [],
      onTogglePath: noop,
      onActivateTab: noop,
      onActivateWorkspaceSession: noop,
      onCloseSession: noop,
      onCloseLogView: noop,
      onOpenQuickSwitcher: noop,
    });
    await flushEffects();

    assert.ok(
      renderer.container.querySelector('[data-section="workbench-tree-fixed-vault"]'),
      "vault fixed entry missing",
    );
    assert.ok(
      renderer.container.querySelector('[data-section="workbench-tree-fixed-sftp"]'),
      "sftp fixed entry missing",
    );
    // Prod is expanded, so Web and both hosts are visible; h1 shows its
    // multi-session count.
    const hostRow = renderer.container.querySelector(
      '[data-section="workbench-tree-host"]',
    );
    assert.ok(hostRow, "web-01 host row missing");
    assert.match(hostRow.textContent ?? "", /2/);
    assert.ok(
      renderer.container.querySelector('[data-tab-id="s1"][data-state="active"]'),
      "active session row not highlighted",
    );
    await renderer.unmount();
  } finally {
    restoreEnv();
  }
});

test("workbench tree interactions: group toggle, session activate, workspace activate", async () => {
  const env = installDomEnvironment();
  const restoreEnv = installTreeEnvironmentMocks();

  try {
    const { WorkbenchSessionTree, buildSessionGroupTree } = await loadComponentModule();
    const buildSections = makeSectionsBuilder(buildSessionGroupTree);
    const renderer = await createDomRenderer(env.document);
    const hosts = [makeHost({ id: "h1", label: "web-01", group: "Prod" })];
    const sessions = [
      makeSession({ id: "s1", hostId: "h1", customName: "web shell" }),
      makeSession({
        id: "s2",
        hostId: "h1",
        workspaceId: "ws1",
        customName: "ws shell",
      }),
    ];
    const workspaces = [{ id: "ws1", title: "Ops workspace" } as Workspace];
    const sections = buildSections(sessions, hosts);
    const toggled: string[] = [];
    const activated: string[] = [];
    const workspaceActivated: string[][] = [];

    const props = {
      sections,
      // The workspace node starts expanded so its session rows are visible.
      expandedPaths: new Set(["workspace:ws1"]),
      fixedIds: new Set(["vault", "sftp"]),
      activeTabId: "",
      sessions,
      workspaces,
      logViews: [],
      onTogglePath: (path: string) => toggled.push(path),
      onActivateTab: (tabId: string) => activated.push(tabId),
      onActivateWorkspaceSession: (workspaceId: string, sessionId: string) =>
        workspaceActivated.push([workspaceId, sessionId]),
      onCloseSession: noop,
      onCloseLogView: noop,
      onOpenQuickSwitcher: noop,
    };
    await renderTree(WorkbenchSessionTree, renderer, props);
    await flushEffects();

    // Collapsed Prod group row shows its aggregated session count; toggling
    // works from the row itself.
    const groupRow = renderer.container.querySelector(
      '[data-section="workbench-tree-group"]',
    );
    assert.ok(groupRow, "Prod group row missing");
    assert.match(groupRow.textContent ?? "", /1/);
    await clickElement(env, groupRow as Element);
    assert.deepEqual(toggled, ["Prod"]);

    // Workspace node label click activates the workspace tab; chevron toggles.
    const wsRow = renderer.container.querySelector(
      '[data-section="workbench-tree-group"][data-tab-id="ws1"]',
    );
    assert.ok(wsRow, "workspace node missing");
    assert.match(wsRow.textContent ?? "", /Ops workspace/);
    await clickElement(env, wsRow as Element);
    assert.deepEqual(activated, ["ws1"]);

    const chevron = wsRow?.querySelector("button");
    assert.ok(chevron, "workspace chevron missing");
    await clickElement(env, chevron as Element);
    assert.deepEqual(toggled, ["Prod", "workspace:ws1"]);

    // Workspace session click focuses the session inside its workspace.
    const wsSessionRow = renderer.container.querySelector(
      '[data-section="workbench-tree-session"][data-tab-id="s2"]',
    );
    assert.ok(wsSessionRow, "workspace session row missing");
    await clickElement(env, wsSessionRow as Element);
    assert.deepEqual(workspaceActivated, [["ws1", "s2"]]);
    await renderer.unmount();
  } finally {
    restoreEnv();
  }
});

test("workbench tree keeps the tree mounted across data updates", async () => {
  const env = installDomEnvironment();
  const restoreEnv = installTreeEnvironmentMocks();

  try {
    const { WorkbenchSessionTree, buildSessionGroupTree } = await loadComponentModule();
    const buildSections = makeSectionsBuilder(buildSessionGroupTree);
    const renderer = await createDomRenderer(env.document);
    const hosts = [makeHost({ id: "h1", label: "web-01", group: "Prod" })];
    const sessions = [makeSession({ id: "s1", hostId: "h1", customName: "web" })];
    const sections = buildSections(sessions, hosts);

    const props = {
      sections,
      expandedPaths: new Set<string>(),
      fixedIds: new Set(["vault", "sftp"]),
      activeTabId: "",
      sessions,
      workspaces: [] as Workspace[],
      logViews: [],
      onTogglePath: noop,
      onActivateTab: noop,
      onActivateWorkspaceSession: noop,
      onCloseSession: noop,
      onCloseLogView: noop,
      onOpenQuickSwitcher: noop,
    };
    await renderTree(WorkbenchSessionTree, renderer, props);
    await flushEffects();

    const treeRoot = renderer.container.querySelector(
      '[data-section="workbench-session-tree"]',
    );
    assert.ok(treeRoot, "tree container missing");

    // Simulate a layout-mode data refresh (new sections identity + new
    // expanded set): the tree must update in place, never remount.
    await renderTree(WorkbenchSessionTree, renderer, {
      ...props,
      sections: buildSections(sessions, hosts),
      expandedPaths: new Set(["Prod"]),
    });
    await flushEffects();

    const treeRootAfter = renderer.container.querySelector(
      '[data-section="workbench-session-tree"]',
    );
    assert.equal(treeRootAfter, treeRoot, "tree remounted on data update");
    await renderer.unmount();
  } finally {
    restoreEnv();
  }
});
