import React from "react";
import { useI18n } from "../../../../application/i18n/I18nProvider";
import { Switch } from "../../../ui/switch";
import { cn } from "../../../../lib/utils";
import type { GrokRuntime } from "../../../../infrastructure/ai/types";
import type { AgentPathInfo } from "./types";
import { AgentExecutablePathField } from "./AgentExecutablePathField";

export const CopilotCliCard: React.FC<{
  pathInfo: AgentPathInfo | null;
  isResolvingPath: boolean;
  customPath: string;
  onCustomPathChange: (path: string) => void;
  onSelectDirectory: () => void;
  onRecheckPath: () => void;
  onResetPath?: () => void;
  i18nPrefix?: "ai.copilot" | "ai.cursor" | "ai.opencode" | "ai.grok";
  allowEmptyCheck?: boolean;
  showCustomPathInput?: boolean;
  /** Grok only: ACP (default) vs headless streaming-json. */
  grokRuntime?: GrokRuntime;
  onGrokRuntimeChange?: (runtime: GrokRuntime) => void;
}> = ({
  pathInfo,
  isResolvingPath,
  customPath,
  onCustomPathChange,
  onSelectDirectory,
  onRecheckPath,
  onResetPath,
  i18nPrefix = "ai.copilot",
  allowEmptyCheck = false,
  showCustomPathInput = true,
  grokRuntime = "acp",
  onGrokRuntimeChange,
}) => {
  const { t } = useI18n();
  const found = pathInfo?.available;
  const showGrokRuntime = i18nPrefix === "ai.grok" && typeof onGrokRuntimeChange === "function";

  const statusText = isResolvingPath
    ? t(`${i18nPrefix}.detecting`)
    : found
      ? t(`${i18nPrefix}.detected`)
      : t(`${i18nPrefix}.notFound`);

  const statusClassName = isResolvingPath
    ? "text-muted-foreground"
    : found
      ? "text-emerald-500"
      : "text-amber-500";

  return (
    <div className="rounded-lg border bg-card p-4 space-y-3">
      <div className="flex items-start justify-between gap-4">
        <p className="min-w-0 text-xs text-muted-foreground leading-5">
          {t(`${i18nPrefix}.description`)}
        </p>
        <div className={cn("text-xs font-medium shrink-0", statusClassName)}>
          {statusText}
        </div>
      </div>

      {found && (
        <div className="flex items-center gap-2 text-xs">
          <span className="text-muted-foreground">{t(`${i18nPrefix}.path`)}</span>
          <span className="font-mono text-foreground truncate">{pathInfo.path}</span>
          {pathInfo.version && (
            <>
              <span className="text-muted-foreground">|</span>
              <span className="text-muted-foreground">{pathInfo.version}</span>
            </>
          )}
        </div>
      )}

      {!isResolvingPath && (
        <div className="space-y-2">
          {!found && (
            <p className="text-xs text-amber-500">
              {t(`${i18nPrefix}.notFoundHint`)}
            </p>
          )}
          {showCustomPathInput ? (
            <AgentExecutablePathField
              i18nPrefix={i18nPrefix}
              customPath={customPath}
              onCustomPathChange={onCustomPathChange}
              onSelectDirectory={onSelectDirectory}
              onRecheckPath={onRecheckPath}
              onResetPath={onResetPath}
              allowEmptyCheck={allowEmptyCheck}
            />
          ) : null}
        </div>
      )}

      {showGrokRuntime && found && (
        <div className="border-t border-border/40 pt-3 flex items-start justify-between gap-4">
          <div className="min-w-0 space-y-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium">{t("ai.grok.runtime.acp.title")}</span>
              <span className="rounded border border-primary/30 bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                {t("ai.grok.runtime.acp.default")}
              </span>
            </div>
            <p className="text-xs text-muted-foreground leading-5">
              {t("ai.grok.runtime.acp.description")}
            </p>
            {grokRuntime === "streaming-json" && (
              <p className="text-xs text-muted-foreground leading-5">
                {t("ai.grok.runtime.streamingJson.hint")}
              </p>
            )}
          </div>
          <Switch
            checked={grokRuntime === "acp"}
            aria-label={t("ai.grok.runtime.acp.title")}
            onCheckedChange={(checked) =>
              onGrokRuntimeChange?.(checked ? "acp" : "streaming-json")
            }
          />
        </div>
      )}
    </div>
  );
};
