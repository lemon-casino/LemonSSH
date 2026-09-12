import React, { memo, useCallback, useMemo } from "react";
import {
  ChevronRight,
  FileText,
  Folder,
  FolderLock,
  Menu,
  Plus,
  X,
} from "lucide-react";

import { useI18n } from "../../application/i18n/I18nProvider";
import {
  useTerminalHostTreeOpen,
  useToggleTerminalHostTree,
} from "../../application/state/terminalHostTreeStore";
import type { LogView } from "../../application/state/logViewState";
import type { TerminalSession, Workspace } from "../../types";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import { FixedSizeVirtualList } from "../ui/FixedSizeVirtualList";
import { TREE_ROW_HEIGHT } from "../sftp/SftpPaneTreeNode";
import {
  flattenSessionGroupTree,
  type SessionGroupTreeSections,
} from "../../domain/sessionGroupTree";

/** Left indent step; capped so deep group paths never push labels out (R8). */
const INDENT_STEP = 16;
const MAX_INDENT_STEPS = 6;
const WORKSPACE_NODE_PREFIX = "workspace:";

export function resolveWorkbenchTreeIndent(depth: number): number {
  return Math.min(depth, MAX_INDENT_STEPS) * INDENT_STEP;
}

const SECTION_HEADER_KEYS: Record<string, string> = {
  ungrouped: "workbench.tree.section.ungrouped",
  localTerminals: "workbench.tree.section.local",
  workspaces: "workbench.tree.section.workspaces",
  logs: "workbench.tree.section.logs",
  editors: "workbench.tree.section.editors",
  others: "workbench.tree.section.others",
};

interface TreeRow {
  node: {
    id: string;
    label: string;
    type: "group" | "host" | "session" | "section";
    sessionCount?: number;
    sessionId?: string;
    sessionIds?: string[];
  };
  depth: number;
}

function SessionStatusDot({ session }: { session: TerminalSession | undefined }) {
  if (!session || session.status === "disconnected") {
    return <span className="w-2 h-2 shrink-0 rounded-full bg-muted-foreground/30" />;
  }
  if (session.status === "connecting") {
    return <span className="w-2 h-2 shrink-0 rounded-full bg-amber-400 animate-pulse" />;
  }
  return <span className="w-2 h-2 shrink-0 rounded-full bg-emerald-500" />;
}

export function WorkbenchSessionTreeRow({
  row,
  fixedIds,
  activeTabId,
  sessionById,
  logViewsById,
  workspaceTitleById,
  expandedPaths,
  onTogglePath,
  onActivateTab,
  onActivateWorkspaceSession,
  onCloseSession,
  onCloseLogView,
  t,
}: {
  row: TreeRow;
  fixedIds: ReadonlySet<string>;
  activeTabId: string;
  sessionById: Map<string, TerminalSession>;
  logViewsById: Map<string, LogView>;
  workspaceTitleById: Map<string, string>;
  expandedPaths: ReadonlySet<string>;
  onTogglePath: (path: string) => void;
  onActivateTab: (tabId: string) => void;
  onActivateWorkspaceSession: (workspaceId: string, sessionId: string) => void;
  onCloseSession: (sessionId: string, e?: React.MouseEvent) => void;
  onCloseLogView: (logViewId: string) => void;
  t: (key: string) => string;
}) {
  const { node, depth } = row;
  const indent = resolveWorkbenchTreeIndent(depth);

  // Fixed root entries (Vaults / SFTP) map 1:1 to root tabs.
  if (node.type === "section" && fixedIds.has(node.id)) {
    const isActive = activeTabId === node.id;
    return (
      <button
        data-section={`workbench-tree-fixed-${node.id}`}
        data-state={isActive ? "active" : "inactive"}
        onClick={() => onActivateTab(node.id)}
        className={cn(
          "w-full flex items-center gap-2 px-2 rounded-md text-xs font-semibold cursor-pointer transition-colors",
          isActive
            ? "bg-foreground/10 text-foreground"
            : "text-muted-foreground hover:text-foreground hover:bg-foreground/5",
        )}
        style={{ marginLeft: indent, height: TREE_ROW_HEIGHT }}
      >
        {node.id === "vault" ? (
          <FolderLock size={14} className="shrink-0" />
        ) : (
          <Folder size={14} className="shrink-0" />
        )}
        <span className="truncate">{node.label}</span>
      </button>
    );
  }

  // Named section headers; the top-level session groups speak for themselves.
  if (node.type === "section") {
    const headerKey = SECTION_HEADER_KEYS[node.id];
    if (!headerKey) return null;
    return (
      <div
        className="px-2 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/70 select-none"
        style={{ marginLeft: indent, height: TREE_ROW_HEIGHT }}
      >
        {t(headerKey)}
      </div>
    );
  }

  if (node.type === "group" || node.type === "host") {
    const isWorkspaceNode = node.id.startsWith(WORKSPACE_NODE_PREFIX);
    const workspaceId = isWorkspaceNode
      ? node.id.slice(WORKSPACE_NODE_PREFIX.length)
      : null;
    const expanded = expandedPaths.has(node.id);
    const label = isWorkspaceNode
      ? workspaceTitleById.get(workspaceId ?? "") ?? node.label
      : node.label;
    const showCount = node.type === "group"
      ? (node.sessionCount ?? 0) > 0
      : (node.sessionIds?.length ?? 0) > 1;

    return (
      <div
        data-section={node.type === "group" ? "workbench-tree-group" : "workbench-tree-host"}
        data-tab-id={isWorkspaceNode ? workspaceId : undefined}
        data-state={expanded ? "expanded" : "collapsed"}
        className="w-full flex items-center gap-1 px-2 rounded-md text-xs font-medium text-foreground/80 hover:bg-foreground/5 cursor-pointer select-none"
        style={{ marginLeft: indent, height: TREE_ROW_HEIGHT }}
        onClick={() => {
          // Workspace nodes double as tab shortcuts; plain groups toggle.
          if (isWorkspaceNode && workspaceId) {
            onActivateTab(workspaceId);
            return;
          }
          onTogglePath(node.id);
        }}
      >
        <button
          aria-label={label}
          className="p-0.5 rounded cursor-pointer hover:bg-foreground/10 shrink-0"
          onClick={(e) => {
            e.stopPropagation();
            onTogglePath(node.id);
          }}
        >
          <ChevronRight
            size={12}
            className={cn("transition-transform", expanded && "rotate-90")}
          />
        </button>
        <span className="truncate flex-1 text-left">{label}</span>
        {showCount && (
          <span className="shrink-0 px-1.5 rounded-full bg-foreground/10 text-[10px] text-muted-foreground">
            {node.type === "group" ? node.sessionCount : node.sessionIds?.length}
          </span>
        )}
      </div>
    );
  }

  // Session-like leaf rows: real sessions, log views and editor tabs.
  const sessionId = node.sessionId ?? node.id;
  const logView = logViewsById.get(sessionId);
  const session = sessionById.get(sessionId);
  const isActive = activeTabId === sessionId;

  return (
    <div
      data-section="workbench-tree-session"
      data-tab-id={sessionId}
      data-state={isActive ? "active" : "inactive"}
      className={cn(
        "w-full flex items-center gap-2 pl-2 pr-1 rounded-md text-xs font-semibold cursor-pointer select-none group/leaf",
        isActive
          ? "bg-foreground/10 text-foreground"
          : "text-muted-foreground hover:text-foreground hover:bg-foreground/5",
      )}
      style={{ marginLeft: indent, height: TREE_ROW_HEIGHT }}
      onClick={() => {
        if (session?.workspaceId) {
          onActivateWorkspaceSession(session.workspaceId, session.id);
          return;
        }
        onActivateTab(sessionId);
      }}
    >
      {logView ? (
        <FileText size={12} className="shrink-0" />
      ) : (
        <SessionStatusDot session={session} />
      )}
      <span className="truncate flex-1">{node.label}</span>
      {logView && (
        <button
          aria-label={t("tabs.closeLogViewAria")}
          className="p-1 rounded-full cursor-pointer opacity-0 group-hover/leaf:opacity-100 hover:bg-destructive/10 hover:text-destructive transition-opacity"
          onClick={(e) => {
            e.stopPropagation();
            onCloseLogView(logView.id);
          }}
        >
          <X size={10} />
        </button>
      )}
      {session && !session.workspaceId && (
        <button
          aria-label={t("tabs.closeSessionAria")}
          className="p-1 rounded-full cursor-pointer opacity-0 group-hover/leaf:opacity-100 hover:bg-destructive/10 hover:text-destructive transition-opacity"
          onClick={(e) => {
            e.stopPropagation();
            onCloseSession(session.id, e);
          }}
        >
          <X size={10} />
        </button>
      )}
    </div>
  );
}

interface WorkbenchSessionTreeProps {
  sections: SessionGroupTreeSections;
  expandedPaths: Set<string>;
  /** Ids of fixed entries that map 1:1 to root tabs (vault / sftp). */
  fixedIds: ReadonlySet<string>;
  activeTabId: string;
  sessions: TerminalSession[];
  workspaces: Workspace[];
  logViews: LogView[];
  onTogglePath: (path: string) => void;
  onActivateTab: (tabId: string) => void;
  onActivateWorkspaceSession: (workspaceId: string, sessionId: string) => void;
  onCloseSession: (sessionId: string, e?: React.MouseEvent) => void;
  onCloseLogView: (logViewId: string) => void;
  onOpenQuickSwitcher: () => void;
}

const WorkbenchSessionTreeInner: React.FC<WorkbenchSessionTreeProps> = ({
  sections,
  expandedPaths,
  fixedIds,
  activeTabId,
  sessions,
  workspaces,
  logViews,
  onTogglePath,
  onActivateTab,
  onActivateWorkspaceSession,
  onCloseSession,
  onCloseLogView,
  onOpenQuickSwitcher,
}) => {
  const { t } = useI18n();
  const hostTreeOpen = useTerminalHostTreeOpen();
  const toggleHostTree = useToggleTerminalHostTree();

  const rows = useMemo(
    () => flattenSessionGroupTree(sections, expandedPaths),
    [sections, expandedPaths],
  );

  const sessionById = useMemo(
    () => new Map(sessions.map((session) => [session.id, session])),
    [sessions],
  );
  const logViewsById = useMemo(
    () => new Map(logViews.map((logView) => [logView.id, logView])),
    [logViews],
  );
  const workspaceTitleById = useMemo(
    () => new Map(workspaces.map((workspace) => [workspace.id, workspace.title])),
    [workspaces],
  );

  const renderRow = useCallback((row: TreeRow) => (
    <WorkbenchSessionTreeRow
      row={row}
      fixedIds={fixedIds}
      activeTabId={activeTabId}
      sessionById={sessionById}
      logViewsById={logViewsById}
      workspaceTitleById={workspaceTitleById}
      expandedPaths={expandedPaths}
      onTogglePath={onTogglePath}
      onActivateTab={onActivateTab}
      onActivateWorkspaceSession={onActivateWorkspaceSession}
      onCloseSession={onCloseSession}
      onCloseLogView={onCloseLogView}
      t={t}
    />
  ), [
    fixedIds,
    activeTabId,
    sessionById,
    logViewsById,
    workspaceTitleById,
    expandedPaths,
    onTogglePath,
    onActivateTab,
    onActivateWorkspaceSession,
    onCloseSession,
    onCloseLogView,
    t,
  ]);

  return (
    <div className="flex flex-col min-h-0 w-full" data-section="workbench-session-tree">
      <div className="flex-1 min-h-0 px-1">
        {rows.length === 0 ? (
          <div className="h-full flex items-center justify-center px-4 text-xs text-muted-foreground/70 text-center select-none">
            {t("workbench.tree.empty")}
          </div>
        ) : (
          <FixedSizeVirtualList
            items={rows}
            itemHeight={TREE_ROW_HEIGHT}
            getItemKey={(row) => row.node.id}
            renderItem={renderRow}
          />
        )}
      </div>
      <div className="shrink-0 flex items-center gap-1 px-2 py-1.5 border-t border-border/60 app-no-drag">
        <Button
          variant="ghost"
          size="sm"
          className="flex-1 justify-start gap-2 h-7 text-xs"
          onClick={onOpenQuickSwitcher}
        >
          <Plus size={14} />
          {t("workbench.tree.newSession")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0"
          data-state={hostTreeOpen ? "active" : "inactive"}
          onClick={toggleHostTree}
          aria-label={hostTreeOpen ? t("terminal.layer.hostTree.collapse") : t("terminal.layer.hostTree.expand")}
        >
          <Menu size={14} />
        </Button>
      </div>
    </div>
  );
};

export const WorkbenchSessionTree = memo(
  WorkbenchSessionTreeInner,
  (prev, next) =>
    prev.sections === next.sections &&
    prev.expandedPaths === next.expandedPaths &&
    prev.fixedIds === next.fixedIds &&
    prev.activeTabId === next.activeTabId &&
    prev.sessions === next.sessions &&
    prev.workspaces === next.workspaces &&
    prev.logViews === next.logViews &&
    prev.onTogglePath === next.onTogglePath &&
    prev.onActivateTab === next.onActivateTab &&
    prev.onActivateWorkspaceSession === next.onActivateWorkspaceSession &&
    prev.onCloseSession === next.onCloseSession &&
    prev.onCloseLogView === next.onCloseLogView &&
    prev.onOpenQuickSwitcher === next.onOpenQuickSwitcher,
);
WorkbenchSessionTree.displayName = "WorkbenchSessionTree";
