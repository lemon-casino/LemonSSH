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
};

/**
 * Vault navigation items (hosts, keys, proxies, port forwarding, scripts,
 * notes, known hosts, logs). Rendered inside the classic vault sidebar and
 * reusable by other shells (e.g. a future workbench menu bar); owns only the
 * nav list itself, not the surrounding sidebar chrome.
 */
export function VaultNavItems({
  currentSection,
  onSelectSection,
  sidebarCollapsed,
  t,
}: VaultNavItemsProps) {
  return (
    <div
      className={cn("space-y-1", sidebarCollapsed ? "px-1.5" : "px-2.5")}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <RippleButton
            variant={currentSection === "hosts" ? "secondary" : "ghost"}
            className={cn(
              "w-full h-10",
              sidebarCollapsed
                ? "justify-center p-0"
                : "justify-start gap-3",
              currentSection === "hosts" &&
                "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
            )}
            onClick={() => onSelectSection("hosts")}
          >
            <LayoutGrid size={16} className="flex-shrink-0" />
            {!sidebarCollapsed && t("vault.nav.hosts")}
          </RippleButton>
        </TooltipTrigger>
        {sidebarCollapsed && (
          <TooltipContent side="right">
            {t("vault.nav.hosts")}
          </TooltipContent>
        )}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <RippleButton
            variant={currentSection === "keys" ? "secondary" : "ghost"}
            className={cn(
              "w-full h-10",
              sidebarCollapsed
                ? "justify-center p-0"
                : "justify-start gap-3",
              currentSection === "keys" &&
                "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
            )}
            onClick={() => onSelectSection("keys")}
          >
            <Key size={16} className="flex-shrink-0" />
            {!sidebarCollapsed && t("vault.nav.keychain")}
          </RippleButton>
        </TooltipTrigger>
        {sidebarCollapsed && (
          <TooltipContent side="right">
            {t("vault.nav.keychain")}
          </TooltipContent>
        )}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <RippleButton
            variant={currentSection === "proxies" ? "secondary" : "ghost"}
            className={cn(
              "w-full h-10",
              sidebarCollapsed
                ? "justify-center p-0"
                : "justify-start gap-3",
              currentSection === "proxies" &&
                "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
            )}
            onClick={() => onSelectSection("proxies")}
          >
            <Globe size={16} className="flex-shrink-0" />
            {!sidebarCollapsed && t("vault.nav.proxies")}
          </RippleButton>
        </TooltipTrigger>
        {sidebarCollapsed && (
          <TooltipContent side="right">
            {t("vault.nav.proxies")}
          </TooltipContent>
        )}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <RippleButton
            variant={currentSection === "port" ? "secondary" : "ghost"}
            className={cn(
              "w-full h-10",
              sidebarCollapsed
                ? "justify-center p-0"
                : "justify-start gap-3",
              currentSection === "port" &&
                "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
            )}
            onClick={() => onSelectSection("port")}
          >
            <Plug size={16} className="flex-shrink-0" />
            {!sidebarCollapsed && t("vault.nav.portForwarding")}
          </RippleButton>
        </TooltipTrigger>
        {sidebarCollapsed && (
          <TooltipContent side="right">
            {t("vault.nav.portForwarding")}
          </TooltipContent>
        )}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <RippleButton
            variant={
              currentSection === "snippets" ? "secondary" : "ghost"
            }
            className={cn(
              "w-full h-10",
              sidebarCollapsed
                ? "justify-center p-0"
                : "justify-start gap-3",
              currentSection === "snippets" &&
                "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
            )}
            onClick={() => onSelectSection("snippets")}
          >
            <FileCode size={16} className="flex-shrink-0" />
            {!sidebarCollapsed && t("vault.nav.scripts")}
          </RippleButton>
        </TooltipTrigger>
        {sidebarCollapsed && (
          <TooltipContent side="right">
            {t("vault.nav.scripts")}
          </TooltipContent>
        )}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <RippleButton
            variant={currentSection === "notes" ? "secondary" : "ghost"}
            className={cn(
              "w-full h-10",
              sidebarCollapsed
                ? "justify-center p-0"
                : "justify-start gap-3",
              currentSection === "notes" &&
                "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
            )}
            onClick={() => onSelectSection("notes")}
          >
            <NotebookText size={16} className="flex-shrink-0" />
            {!sidebarCollapsed && t("vault.nav.notes")}
          </RippleButton>
        </TooltipTrigger>
        {sidebarCollapsed && (
          <TooltipContent side="right">
            {t("vault.nav.notes")}
          </TooltipContent>
        )}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <RippleButton
            variant={
              currentSection === "knownhosts" ? "secondary" : "ghost"
            }
            className={cn(
              "w-full h-10",
              sidebarCollapsed
                ? "justify-center p-0"
                : "justify-start gap-3",
              currentSection === "knownhosts" &&
                "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
            )}
            onClick={() => onSelectSection("knownhosts")}
          >
            <BookMarked size={16} className="flex-shrink-0" />
            {!sidebarCollapsed && t("vault.nav.knownHosts")}
          </RippleButton>
        </TooltipTrigger>
        {sidebarCollapsed && (
          <TooltipContent side="right">
            {t("vault.nav.knownHosts")}
          </TooltipContent>
        )}
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <RippleButton
            variant={currentSection === "logs" ? "secondary" : "ghost"}
            className={cn(
              "w-full h-10",
              sidebarCollapsed
                ? "justify-center p-0"
                : "justify-start gap-3",
              currentSection === "logs" &&
                "bg-foreground/10 text-foreground hover:bg-foreground/15 border-border/40",
            )}
            onClick={() => onSelectSection("logs")}
          >
            <Activity size={16} className="flex-shrink-0" />
            {!sidebarCollapsed && t("vault.nav.logs")}
          </RippleButton>
        </TooltipTrigger>
        {sidebarCollapsed && (
          <TooltipContent side="right">
            {t("vault.nav.logs")}
          </TooltipContent>
        )}
      </Tooltip>
    </div>
  );
}
