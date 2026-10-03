import { useSyncExternalStore, useCallback } from 'react';
import { TerminalTheme } from '../../domain/models';
import { TERMINAL_THEMES } from '../../infrastructure/config/terminalThemes';
import { STORAGE_KEY_CUSTOM_THEMES } from '../../infrastructure/config/storageKeys';
import { hostStorageAdapter as localStorageAdapter } from '../../infrastructure/persistence/hostStorageAdapter';

/**
 * Custom Theme Store - manages user-created terminal themes
 * Uses useSyncExternalStore pattern (same as fontStore)
 * Persists through the host storage adapter. There is no live IPC channel in
 * the Wails shell: cross-window updates arrive through the settings
 * storage-sync manifest (settingsStorageSync.ts), which reloads this store
 * whenever the stored key changes in this or another window.
 */
type Listener = () => void;

class CustomThemeStore {
    private themes: TerminalTheme[] = [];
    private listeners = new Set<Listener>();
    /** Cached merged array for stable useSyncExternalStore snapshots */
    private cachedAllThemes: TerminalTheme[] | null = null;
    /** Raw JSON of the last loaded/persisted list; skips own-write echoes. */
    private lastSyncedRaw: string | null = null;

    constructor() {
        this.loadFromStorage();
    }

    /** Reload themes from storage. Called after sync apply and by settingsStorageSync when the key changes. */
    loadFromStorage = () => {
        try {
            const raw = localStorageAdapter.readString(STORAGE_KEY_CUSTOM_THEMES);
            if (raw === this.lastSyncedRaw) return;
            const parsed = raw === null ? [] : JSON.parse(raw);
            if (!Array.isArray(parsed)) return;
            this.themes = parsed.map((t: TerminalTheme) => ({ ...t, isCustom: true }));
            this.lastSyncedRaw = raw;
        } catch {
            // ignore corrupt data
        }
        this.notify();
    };

    private saveToStorage = () => {
        try {
            const raw = JSON.stringify(this.themes);
            if (localStorageAdapter.writeString(STORAGE_KEY_CUSTOM_THEMES, raw)) {
                this.lastSyncedRaw = raw;
            }
        } catch {
            // storage full or unavailable
        }
    };

    private notify = () => {
        this.cachedAllThemes = null; // invalidate cache on any mutation
        this.listeners.forEach(listener => listener());
    };

    subscribe = (listener: Listener): (() => void) => {
        this.listeners.add(listener);
        return () => this.listeners.delete(listener);
    };

    // ---- Getters (stable references for useSyncExternalStore) ----

    getCustomThemes = (): TerminalTheme[] => this.themes;

    /** Returns all themes: built-in + custom (cached for snapshot stability) */
    getAllThemes = (): TerminalTheme[] => {
        if (!this.cachedAllThemes) {
            this.cachedAllThemes = [...TERMINAL_THEMES, ...this.themes];
        }
        return this.cachedAllThemes;
    };

    /** Find a theme by ID across both built-in and custom */
    getThemeById = (id: string): TerminalTheme | undefined => {
        return TERMINAL_THEMES.find(t => t.id === id) || this.themes.find(t => t.id === id);
    };

    // ---- Mutations ----

    addTheme = (theme: TerminalTheme) => {
        this.themes = [...this.themes, { ...theme, isCustom: true }];
        this.saveToStorage();
        this.notify();
    };

    updateTheme = (id: string, updates: Partial<TerminalTheme>) => {
        this.themes = this.themes.map(t =>
            t.id === id ? { ...t, ...updates, isCustom: true } : t
        );
        this.saveToStorage();
        this.notify();
    };

    deleteTheme = (id: string) => {
        this.themes = this.themes.filter(t => t.id !== id);
        this.saveToStorage();
        this.notify();
    };

    replaceThemes = (themes: TerminalTheme[]) => {
        this.themes = themes.map((theme) => ({ ...theme, colors: { ...theme.colors }, isCustom: true }));
        this.saveToStorage();
        this.notify();
    };
}

// Singleton
export const customThemeStore = new CustomThemeStore();

// ============== Hooks ==============

/** Get custom themes only */
export const useCustomThemes = (): TerminalTheme[] => {
    return useSyncExternalStore(
        customThemeStore.subscribe,
        customThemeStore.getCustomThemes
    );
};

/** Theme mutation actions */
export const useCustomThemeActions = () => {
    const addTheme = useCallback((theme: TerminalTheme) => {
        customThemeStore.addTheme(theme);
    }, []);

    const updateTheme = useCallback((id: string, updates: Partial<TerminalTheme>) => {
        customThemeStore.updateTheme(id, updates);
    }, []);

    const deleteTheme = useCallback((id: string) => {
        customThemeStore.deleteTheme(id);
    }, []);

    const replaceThemes = useCallback((themes: TerminalTheme[]) => {
        customThemeStore.replaceThemes(themes);
    }, []);

    return { addTheme, updateTheme, deleteTheme, replaceThemes };
};
