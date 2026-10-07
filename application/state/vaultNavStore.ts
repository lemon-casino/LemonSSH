import { useSyncExternalStore } from "react";

export type VaultSection =
  | "hosts"
  | "keys"
  | "proxies"
  | "port"
  | "snippets"
  | "notes"
  | "knownhosts"
  | "logs";

type VaultNavState = {
  currentSection: VaultSection;
};

type VaultNavActions = {
  setCurrentSection: (section: VaultSection) => void;
};

const DEFAULT_STATE: VaultNavState = Object.freeze({ currentSection: "hosts" });

let state: VaultNavState = DEFAULT_STATE;
const listeners = new Set<() => void>();
let actions: VaultNavActions | null = null;

const noopActions: VaultNavActions = { setCurrentSection: () => {} };

function notify() {
  for (const listener of listeners) listener();
}

export function registerVaultNav(next: VaultNavActions | null) {
  actions = next;
  notify();
}

export function setVaultNavSection(section: VaultSection) {
  if (actions) actions.setCurrentSection(section);
  else {
    state = { ...state, currentSection: section };
    notify();
  }
}

/**
 * Mirrors the VaultView-owned section into the store without invoking the
 * registered action, so external chrome (workbench menu bar) stays in sync
 * with section changes that originate inside VaultView.
 */
export function syncVaultNavSection(section: VaultSection) {
  if (state.currentSection === section) return;
  state = { ...state, currentSection: section };
  notify();
}

export function useVaultNavState(): VaultNavState {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    () => state,
    () => DEFAULT_STATE,
  );
}

export function useVaultNavActions(): VaultNavActions {
  return actions ?? noopActions;
}
