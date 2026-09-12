import React, { memo, useMemo } from 'react';

import { toEditorTabId, useActiveTabId } from '../state/activeTabStore';
import type { EditorTabChrome } from '../state/editorTabStore';
import type { LogView } from '../state/logViewState';
import { useWorkbenchTreeExpanded } from '../state/workbenchSessionTreeStore';
import { useI18n } from '../i18n/I18nProvider';
import { WorkbenchSessionTree } from '../../components/workbench/WorkbenchSessionTree';
import type { GroupConfig, Host, TerminalSession, Workspace } from '../../types';
import { resolveSessionTabTitle } from '../../domain/sessionTabTitle';
import {
  buildSessionGroupTree,
  type BuildSessionGroupTreeOptions,
} from '../../domain/sessionGroupTree';
import type { DynamicTabTitleMode } from '../../domain/models';
import { getAppHostTreeLayerStyle } from './AppHostTreeLayer';

/** Default sidebar width; interactive resizing lands with the P4 pass. */
const WORKBENCH_SESSION_TREE_WIDTH = 240;

interface AppWorkbenchSessionLayerProps {
  enabled: boolean;
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
}

function appWorkbenchSessionLayerAreEqual(
  prev: AppWorkbenchSessionLayerProps,
  next: AppWorkbenchSessionLayerProps,
): boolean {
  return prev.enabled === next.enabled
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
    && prev.onOpenQuickSwitcher === next.onOpenQuickSwitcher;
}

const AppWorkbenchSessionLayerInner: React.FC<AppWorkbenchSessionLayerProps> = ({
  enabled,
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
}) => {
  // Leaf-layer subscription: AppView must never read activeTabId itself.
  const activeTabId = useActiveTabId();
  const { expandedPaths, togglePath } = useWorkbenchTreeExpanded();
  const { t } = useI18n();
  const surfaceVisible = enabled;

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

  const fixedIds = useMemo(
    () => new Set(showSftpTab ? ['vault', 'sftp'] : ['vault']),
    [showSftpTab],
  );

  return (
    <div
      className="relative shrink-0 min-h-0"
      data-section="app-workbench-session-layer"
      style={{ width: surfaceVisible ? WORKBENCH_SESSION_TREE_WIDTH : 0 }}
    >
      <div
        className="absolute inset-0 flex min-h-0 bg-secondary border-r border-border/60"
        data-section="app-workbench-session-tree"
        style={getAppHostTreeLayerStyle(surfaceVisible)}
      >
        <WorkbenchSessionTree
          sections={sections}
          expandedPaths={expandedPaths}
          fixedIds={fixedIds}
          activeTabId={activeTabId}
          sessions={sessions}
          workspaces={workspaces}
          logViews={logViews as LogView[]}
          onTogglePath={togglePath}
          onActivateTab={onActivateTab}
          onActivateWorkspaceSession={onActivateWorkspaceSession}
          onCloseSession={onCloseSession}
          onCloseLogView={onCloseLogView}
          onOpenQuickSwitcher={onOpenQuickSwitcher}
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
