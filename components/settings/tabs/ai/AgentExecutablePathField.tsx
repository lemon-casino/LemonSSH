import React from "react";
import { FolderOpen, RefreshCw, RotateCcw } from "lucide-react";
import { useI18n } from "../../../../application/i18n/I18nProvider";
import { Button } from "../../../ui/button";

export const AgentExecutablePathField: React.FC<{
  i18nPrefix: string;
  customPath: string;
  onCustomPathChange: (path: string) => void;
  onSelectDirectory: () => void;
  onRecheckPath: () => void;
  onResetPath?: () => void;
  allowEmptyCheck?: boolean;
  disabled?: boolean;
}> = ({
  i18nPrefix,
  customPath,
  onCustomPathChange,
  onSelectDirectory,
  onRecheckPath,
  onResetPath,
  allowEmptyCheck = false,
  disabled = false,
}) => {
  const { t } = useI18n();
  return (
    <div className="flex items-center gap-2">
      <input
        type="text"
        value={customPath}
        onChange={(event) => onCustomPathChange(event.target.value)}
        placeholder={t(`${i18nPrefix}.customPathPlaceholder`)}
        disabled={disabled}
        className="flex-1 h-8 rounded-md border border-input bg-background px-3 text-sm font-mono placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:opacity-50"
      />
      <Button variant="outline" size="sm" onClick={onSelectDirectory} disabled={disabled}>
        <FolderOpen size={14} className="mr-1.5" />
        {t("ai.agent.directory")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={onRecheckPath}
        disabled={disabled || (!allowEmptyCheck && !customPath.trim())}
      >
        <RefreshCw size={14} className="mr-1.5" />
        {t(`${i18nPrefix}.check`)}
      </Button>
      {onResetPath ? (
        <Button variant="ghost" size="sm" onClick={onResetPath} disabled={disabled || !customPath.trim()}>
          <RotateCcw size={14} className="mr-1.5" />
          {t(`${i18nPrefix}.resetPath`)}
        </Button>
      ) : null}
    </div>
  );
};
