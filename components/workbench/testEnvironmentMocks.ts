/**
 * Mocks for module-scope storage reads. The workbench tree transitively
 * imports terminalHostTreeStore, which constructs its store (and reads
 * localStorage) at module load, so these must be installed before the first
 * dynamic import of the component graph.
 */
export function installTreeEnvironmentMocks() {
  const store = new Map<string, string>();
  const previousLocalStorage = (globalThis as Record<string, unknown>).localStorage;
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => {
        store.set(key, value);
      },
      removeItem: (key: string) => {
        store.delete(key);
      },
    },
  });
  const previousResizeObserver = globalThis.ResizeObserver;
  Object.defineProperty(globalThis, "ResizeObserver", {
    configurable: true,
    writable: true,
    value: class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  });
  return () => {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      value: previousLocalStorage,
    });
    Object.defineProperty(globalThis, "ResizeObserver", {
      configurable: true,
      writable: true,
      value: previousResizeObserver,
    });
  };
}
