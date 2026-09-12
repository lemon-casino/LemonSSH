import React, { memo, useEffect, useMemo } from 'react';

import { toEditorTabId, useActiveTabId } from '../state/activeTabStore';
import type { EditorTabChrome } from '../state/editorTabStore';
import type { LogView } from '../state/logViewState';
import { useWorkbenchTreeExpanded, useWorkbenchTreeWidth } from '../state/workbenchSessionTreeStore';
import { useI18n } from '../i18n/I18nProvider';
import { WorkbenchSessionTree } from '../../components/workbench/WorkbenchSessionTree';
import type { GroupConfig, Host, TerminalSession, Workspace } from '../../types';
import { resolveSessionTabTitle } from '../../domain/sessionTabTitle';
import {
  buildSessionGroupTree,
  getSessionTreeAncestorIds,
  type BuildSessionGroupTreeOptions,
} from '../../domain/sessionGroupTree';
import type { DynamicTabTitleMode, KeyBinding } from '../../domain/models';
import type { SplitHint } from '../../domain/workspace';
import type { TopTabInsertionTarget } from '../state/terminalDragData';
import { WORKSPACE_SESSION_DRAG_TYPE, hasWorkspaceSessionDrag, getWorkspaceSessionDragId } from '../state/terminalDragData';
import { resolveWorkspaceSessionTabDropTarget } from '../../components/TopTabs';
import { appendHostFromWorkspaceDrop, resolveFocusSidebarDragKind } from '../../domain/focusSidebarHostDrop';
import { useSettingsChromeStore } from '../state/settingsChromeStore';
import { useShortcutModifierHeld } from '../state/useShortcutModifierHeld';
import { buildTabShortcutNumberById } from './tabShortcutTargets';
import { getAppHostTreeLayerStyle } from './AppHostTreeLayer';

interface AppWorkbenchSessionLayerProps {
  enabled: boolean;
  switchTabKeyBinding: Pick<KeyBinding, 'mac' | 'pc'> | null;
  onStartSessionDrag: (id: string) => void;
  onEndSessionDrag: () => void;
  onReorderTabs: (id: string, target: string, position: 'before' | 'after') => void;
  onRemoveSessionFromWorkspace: (id: string, target?: TopTabInsertionTarget) => void;
  onAppendHostToWorkspace: (workspaceId: string, hostId: string) => void;
  onAddSessionToWorkspace: (workspaceId: string, sessionId: string, hint: SplitHint) => void;
  hosts: Host[];
  customGroups: string[];
  groupConfigs: GroupConfig[];
  sessions: TerminalSession[];
  workspaces: Workspace[];
  editorTabs: readonly EditorTabChrome[];
  logViews: readonly LogView[];
  orderedTabs: readonly string[];
  showSftpTab: boolean;
  dynamicTabTitleMode: DynamicTabTitleMode;
  onActivateTab: (tabId: string) => void;
  onActivateWorkspaceSession: (workspaceId: string, sessionId: string) => void;
  onCloseSession: (sessionId: string, e?: React.MouseEvent) => void;
  onCloseLogView: (logViewId: string) => void;
  onOpenQuickSwitcher: () => void;
  onRenameSession: (sessionId: string) => void;
  onCopySession?: (sessionId: string) => void;
  onCopySessionToNewWindow?: (sessionId: string) => void;
  onReconnectSession: (sessionId: string) => void;
  onEditHost?: (host: Host) => void;
  onRenameWorkspace: (workspaceId: string) => void;
  onCopyWorkspace: (workspaceId: string) => void;
  onCloseWorkspace: (workspaceId: string) => void;
}

function appWorkbenchSessionLayerAreEqual(
  prev: AppWorkbenchSessionLayerProps,
  next: AppWorkbenchSessionLayerProps,
): boolean {
  return prev.switchTabKeyBinding === next.switchTabKeyBinding
    && prev.onStartSessionDrag === next.onStartSessionDrag
    && prev.onEndSessionDrag === next.onEndSessionDrag
    && prev.onReorderTabs === next.onReorderTabs
    && prev.onRemoveSessionFromWorkspace === next.onRemoveSessionFromWorkspace
    && prev.onAppendHostToWorkspace === next.onAppendHostToWorkspace
    && prev.onAddSessionToWorkspace === next.onAddSessionToWorkspace
    && prev.enabled === next.enabled
    && prev.hosts === next.hosts
    && prev.customGroups === next.customGroups
    && prev.groupConfigs === next.groupConfigs
    && prev.sessions === next.sessions
    && prev.workspaces === next.workspaces
    && prev.editorTabs === next.editorTabs
    && prev.logViews === next.logViews
    && prev.orderedTabs === next.orderedTabs
    && prev.showSftpTab === next.showSftpTab
    && prev.dynamicTabTitleMode === next.dynamicTabTitleMode
    && prev.onActivateTab === next.onActivateTab
    && prev.onActivateWorkspaceSession === next.onActivateWorkspaceSession
    && prev.onCloseSession === next.onCloseSession
    && prev.onCloseLogView === next.onCloseLogView
    && prev.onOpenQuickSwitcher === next.onOpenQuickSwitcher
    && prev.onRenameSession === next.onRenameSession
    && prev.onCopySession === next.onCopySession
    && prev.onCopySessionToNewWindow === next.onCopySessionToNewWindow
    && prev.onReconnectSession === next.onReconnectSession
    && prev.onEditHost === next.onEditHost
    && prev.onRenameWorkspace === next.onRenameWorkspace
    && prev.onCopyWorkspace === next.onCopyWorkspace
    && prev.onCloseWorkspace === next.onCloseWorkspace;
}

const AppWorkbenchSessionLayerInner: React.FC<AppWorkbenchSessionLayerProps> = ({
  enabled,
  switchTabKeyBinding,
  onStartSessionDrag,
  onEndSessionDrag,
  onReorderTabs,
  onRemoveSessionFromWorkspace,
  onAppendHostToWorkspace,
  onAddSessionToWorkspace,
  hosts,
  customGroups,
  groupConfigs,
  sessions,
  workspaces,
  editorTabs,
  logViews,
  orderedTabs,
  showSftpTab,
  dynamicTabTitleMode,
  onActivateTab,
  onActivateWorkspaceSession,
  onCloseSession,
  onCloseLogView,
  onOpenQuickSwitcher,
  onRenameSession,
  onCopySession,
  onCopySessionToNewWindow,
  onReconnectSession,
  onEditHost,
  onRenameWorkspace,
  onCopyWorkspace,
  onCloseWorkspace,
}) => {
  // Leaf-layer subscription: AppView must never read activeTabId itself.
  const activeTabId = useActiveTabId();
  const { width, resize } = useWorkbenchTreeWidth();
  const { expandedPaths, togglePath, ensurePathExpanded } = useWorkbenchTreeExpanded();
  const { t } = useI18n();
  const surfaceVisible = enabled;
  const { hotkeyScheme, showTabNumberBadges, shellOnlyTabNumberShortcuts } = useSettingsChromeStore();
  const modifierHeld = useShortcutModifierHeld(hotkeyScheme === 'mac' ? switchTabKeyBinding?.mac ?? null : hotkeyScheme === 'pc' ? switchTabKeyBinding?.pc ?? null : null, hotkeyScheme);
  const shortcutNumbers = useMemo(() => showTabNumberBadges && hotkeyScheme !== 'disabled' && modifierHeld
    ? buildTabShortcutNumberById({ showSftpTab, shellOnlyTabNumberShortcuts, orderedTabs, editorTabIds: editorTabs.map(tab => toEditorTabId(tab.id)) })
    : undefined, [showTabNumberBadges, hotkeyScheme, modifierHeld, showSftpTab, shellOnlyTabNumberShortcuts, orderedTabs, editorTabs]);

  const getRowDragProps = (tabId: string): React.HTMLAttributes<HTMLDivElement> => ({
    draggable: true,
    onDragStart: e => {
      e.dataTransfer.effectAllowed = 'move';
      const session = sessions.find(item => item.id === tabId);
      if (session?.workspaceId) {
        e.dataTransfer.setData(WORKSPACE_SESSION_DRAG_TYPE, tabId);
      } else {
        e.dataTransfer.setData('tab-reorder-id', tabId);
      }
      if (session) e.dataTransfer.setData('session-id', tabId);
      onStartSessionDrag(tabId);
    },
    onDragEnd: onEndSessionDrag,
    onDragOver: e => {
      const types = Array.from(e.dataTransfer.types);
      if (types.includes('tab-reorder-id') || hasWorkspaceSessionDrag(e.dataTransfer) || (workspaces.some(ws => ws.id === tabId) && resolveFocusSidebarDragKind({ types }) === 'host-append')) {
        e.preventDefault();
        e.dataTransfer.dropEffect = resolveFocusSidebarDragKind({ types }) === 'host-append' ? 'copy' : 'move';
      }
    },
    onDrop: e => {
      const workspace = workspaces.find(ws => ws.id === tabId);
      if (workspace && appendHostFromWorkspaceDrop({ types: e.dataTransfer.types, getData: type => e.dataTransfer.getData(type), workspaceId: tabId, onAppendHostToWorkspace })) {
        e.preventDefault();
        e.stopPropagation();
        onEndSessionDrag();
        return;
      }
      const id = e.dataTransfer.getData('tab-reorder-id') || getWorkspaceSessionDragId(e.dataTransfer);
      if (!id || id === tabId) return;
      e.preventDefault();
      e.stopPropagation();
      const rect = e.currentTarget.getBoundingClientRect();
      const position = e.clientY < rect.top + rect.height / 2 ? 'before' : 'after';
      const source = sessions.find(session => session.id === id);
      const targetSession = sessions.find(session => session.id === tabId);
      const targetTabId = targetSession?.workspaceId || tabId;
      if (workspace && source && !source.workspaceId && e.clientY >= rect.top + rect.height / 4 && e.clientY <= rect.bottom - rect.height / 4) {
        onAddSessionToWorkspace(tabId, id, { direction: 'horizontal', position: 'right' });
      } else if (source?.workspaceId) {
        onRemoveSessionFromWorkspace(id, resolveWorkspaceSessionTabDropTarget({ targetTabId, position, draggedSessionId: id, draggedWorkspaceId: source.workspaceId, workspaces }));
      } else if (id !== targetTabId) {
        onReorderTabs(id, targetTabId, position);
      }
      onEndSessionDrag();
    },
  });

  const hostById = useMemo(
    () => new Map(hosts.map((host) => [host.id, host])),
    [hosts],
  );

  const sections = useMemo(() => {
    const tabIndexById = new Map(orderedTabs.map((tabId, index) => [tabId, index]));
    const visibleSessions = sessions
      .filter((session) => session.hiddenFromTabs !== true)
      // Session order follows the tab bar order (plan D5).
      .map((session, index) => ({
        session,
        index: tabIndexById.get(session.id) ?? orderedTabs.length + index,
      }))
      .sort((a, b) => a.index - b.index)
      .map(({ session }) => ({
        id: session.id,
        hostId: session.hostId,
        workspaceId: session.workspaceId || undefined,
        hiddenFromTabs: session.hiddenFromTabs === true || undefined,
        label: resolveSessionTabTitle(session, dynamicTabTitleMode),
      }));

    const options: BuildSessionGroupTreeOptions = {
      sessions: visibleSessions,
      hosts: hosts.map((host) => ({
        id: host.id,
        label: host.label,
        group: host.group,
        protocol: host.protocol,
      })),
      customGroups: customGroups.map((group) => ({ group })),
      groupConfigs: Object.fromEntries(
        groupConfigs.map((config) => [config.path, { order: config.order }]),
      ),
      logViews: logViews.map((logView) => ({
        id: logView.id,
        label: logView.log.hostname,
      })),
      editorTabs: editorTabs.map((tab) => ({
        id: toEditorTabId(tab.id),
        label: tab.fileName,
      })),
      fixedItems: showSftpTab
        ? [
            { id: 'vault', label: t('topTabs.vaults') },
            { id: 'sftp', label: 'SFTP' },
          ]
        : [{ id: 'vault', label: t('topTabs.vaults') }],
    };
    return buildSessionGroupTree(options);
  }, [
    orderedTabs,
    sessions,
    dynamicTabTitleMode,
    hosts,
    customGroups,
    groupConfigs,
    logViews,
    editorTabs,
    showSftpTab,
    t,
  ]);

  useEffect(() => {
    if (enabled) getSessionTreeAncestorIds(sections, activeTabId).forEach(ensurePathExpanded);
  }, [enabled, sections, activeTabId, ensurePathExpanded]);

  const fixedIds = useMemo(
    () => new Set(showSftpTab ? ['vault', 'sftp'] : ['vault']),
    [showSftpTab],
  );

  return (
    <div
      className="relative shrink-0 min-h-0"
      data-section="app-workbench-session-layer"
      style={{ width: surfaceVisible ? width : 0 }}
    >
      <div
        className="absolute inset-0 flex min-h-0 bg-secondary border-r border-border/60"
        data-section="app-workbench-session-tree"
        style={getAppHostTreeLayerStyle(surfaceVisible)}
      >
        <WorkbenchSessionTree
          sections={sections}
          shortcutNumbers={shortcutNumbers}
          getRowDragProps={getRowDragProps}
          expandedPaths={expandedPaths}
          fixedIds={fixedIds}
          activeTabId={activeTabId}
          sessions={sessions}
          workspaces={workspaces}
          logViews={logViews as LogView[]}
          hostById={hostById}
          onTogglePath={togglePath}
          onActivateTab={onActivateTab}
          onActivateWorkspaceSession={onActivateWorkspaceSession}
          onCloseSession={onCloseSession}
          onCloseLogView={onCloseLogView}
          onRenameSession={onRenameSession}
          onCopySession={onCopySession}
          onCopySessionToNewWindow={onCopySessionToNewWindow}
          onReconnectSession={onReconnectSession}
          onEditHost={onEditHost}
          onRenameWorkspace={onRenameWorkspace}
          onCopyWorkspace={onCopyWorkspace}
          onCloseWorkspace={onCloseWorkspace}
          onOpenQuickSwitcher={onOpenQuickSwitcher}
        />
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label={t('workbench.tree.section.workspaces')}
          aria-valuemin={180}
          aria-valuemax={480}
          aria-valuenow={width}
          tabIndex={enabled ? 0 : -1}
          className="absolute right-0 inset-y-0 w-1 cursor-col-resize app-no-drag hover:bg-primary/40 touch-none"
          onPointerDown={e => { e.preventDefault(); e.currentTarget.setPointerCapture(e.pointerId); }}
          onPointerMove={e => { if (e.currentTarget.hasPointerCapture(e.pointerId)) resize(e.clientX - e.currentTarget.parentElement!.getBoundingClientRect().left); }}
          onPointerUp={e => { if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId); }}
          onKeyDown={e => { if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') { e.preventDefault(); resize(width + (e.key === 'ArrowLeft' ? -10 : 10)); } }}
        />
      </div>
    </div>
  );
};

export const AppWorkbenchSessionLayer = memo(
  AppWorkbenchSessionLayerInner,
  appWorkbenchSessionLayerAreEqual,
);
AppWorkbenchSessionLayer.displayName = 'AppWorkbenchSessionLayer';
