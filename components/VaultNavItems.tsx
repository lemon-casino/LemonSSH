import type React from "react";
import {
  Activity,
  BookMarked,
  FileCode,
  Globe,
  Key,
  LayoutGrid,
  NotebookText,
  Plug,
} from "lucide-react";
import type { I18nContextValue } from "../application/i18n/I18nProvider";
import { cn } from "../lib/utils";
import type { VaultSection } from "./VaultView";
import { RippleButton } from "./ui/ripple";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

export type { VaultSection };

type VaultNavItemsProps = {
  /** Currently active vault section. */
  currentSection: VaultSection;
  /**
   * Section selection callback; the caller owns section state and any side
   * effects (e.g. clearing the selected host group when leaving "hosts").
   */
  onSelectSection: (section: VaultSection) => void;
  /** Collapsed rail hides labels and renders right-side tooltips instead. */
  sidebarCollapsed: boolean;
  /** I18n translator. */
  t: I18nContextValue["t"];
  /**
   * `vertical` (default) is the classic sidebar rail; `horizontal` is the
   * compact menu-bar row used by the workbench chrome.
   */
  orientation?: "vertical" | "horizontal";
};

/**
 * Vault navigation items (hosts, keys, proxies, port forwarding, scripts,
 * notes, known hosts, logs). Rendered inside the classic vault sidebar and
 * reusable by other shells (e.g. the workbench menu bar); owns only the
 * nav list itself, not the surrounding sidebar chrome.
 */
export function VaultNavItems({
  currentSection,
  onSelectSection,
  sidebarCollapsed,
  t,
  orientation = "vertical",
}: VaultNavItemsProps) {
  const horizontal = orientation === "horizontal";
  const showLabel = horizontal || !sidebarCollapsed;
  const sections: Array<{
    id: VaultSection;
    label: string;
    icon: React.ReactNode;
  }> = [
    { id: "hosts", label: t("vault.nav.hosts"), icon: <LayoutGrid size={16} className="flex-shrink-0" /> },
    { id: "keys", label: t("vault.nav.keychain"), icon: <Key size={16} className="flex-shrink-0" /> },
    { id: "proxies", label: t("vault.nav.proxies"), icon: <Globe size={16} className="flex-shrink-0" /> },
    { id: "port", label: t("vault.nav.portForwarding"), icon: <Plug size={16} className="flex-shrink-0" /> },
    { id: "snippets", label: t("vault.nav.scripts"), icon: <FileCode size={16} className="flex-shrink-0" /> },
    { id: "notes", label: t("vault.nav.notes"), icon: <NotebookText size={16} className="flex-shrink-0" /> },
    { id: "knownhosts", label: t("vault.nav.knownHosts"), icon: <BookMarked size={16} className="flex-shrink-0" /> },
    { id: "logs", label: t("vault.nav.logs"), icon: <Activity size={16} className="flex-shrink-0" /> },
  ];

  if (horizontal) {
    return (
      <div className="flex items-center gap-0.5 min-w-0 overflow-hidden app-no-drag">
        {sections.map((section) => {
          const active = currentSection === section.id;
          return (
            <Tooltip key={section.id}>
              <TooltipTrigger asChild>
                <button
                  data-section={`workbench-menu-${section.id}`}
                  data-state={active ? "active" : "inactive"}
                  onClick={() => onSelectSection(section.id)}
                  className={cn(
                    "h-7 px-2.5 rounded-md text-xs font-semibold whitespace-nowrap flex items-center gap-1.5 transition-colors",
                    active
                      ? "bg-foreground/10 text-foreground"
                      : "text-muted-foreground hover:text-foreground hover:bg-foreground/5",
                  )}
                >
                  {section.icon}
                  <span className="max-w-[9rem] truncate">{section.label}</span>
                </button>
              </TooltipTrigger>
              <TooltipContent side="bottom">{section.label}</TooltipContent>
            </Tooltip>
          );
        })}
      </div>
    );
  }

  return (
    <div
      className={cn("space-y-1", sidebarCollapsed ? "px-1.5" : "px-2.5")}
    >
      {sections.map((section) => (
        <Tooltip key={section.id}>
          <TooltipTrigger asChild>
            <RippleButton
              variant={currentSection === section.id ? "secondary" : "ghost"}
              className={cn(
                "w-full h-10",
                sidebarCollapsed
                  ? "justify-center p-0"
                  : "justify-start gap-3",
                currentSection === section.id &&
                  "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
              )}
              onClick={() => onSelectSection(section.id)}
            >
              {section.icon}
              {showLabel && section.label}
            </RippleButton>
          </TooltipTrigger>
          {sidebarCollapsed && (
            <TooltipContent side="right">{section.label}</TooltipContent>
          )}
        </Tooltip>
      ))}
    </div>
  );
}
