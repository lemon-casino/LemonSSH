import { installElectronRuntimeClient } from "./electron/electronRuntimeClient";
import { installWailsRuntimeClient } from "./wails/wailsRuntimeClient";
import { createProfileClient, getRawText } from "./profile/profileClient";
import { getActiveRuntimeClient } from "./runtimeClient";
import { configureHostProfileClient } from "../persistence/hostStorageAdapter";
import { localStorageAdapter } from "../persistence/localStorageAdapter";
import { hydrateLocalStorageFromProfile, listHydrationKeys } from "../persistence/hostStorageHydrate";

// Runtime selection bootstrap (P1-02). Installs the Wails RuntimeClient when
// the bundle runs under the Wails shell and the Electron adapter otherwise.
// Electron remains the default dev/release shell, so the Electron path must
// behave exactly as before this module existed.

export let hydrateReady: Promise<void> = Promise.resolve();

export function installRuntimeClient(): void {
  if (installWailsRuntimeClient()) {
    const client = createProfileClient();
    configureHostProfileClient(client);
    installRendererErrorLogging();
    hydrateReady = hydrateWailsProfile(client);
    return;
  }
  // Electron remains the stable release shell; localStorage is canonical
  // until P2-07's cutover gate moves a domain to the Go profile store.
  configureHostProfileClient(undefined);
  installElectronRuntimeClient();
}

let rendererErrorLoggingInstalled = false;

/**
 * Forwards renderer crashes into the Go logs folder: window errors,
 * unhandled rejections, and console.error. Throttled per message so a
 * repeating failure cannot flood the log file.
 */
function installRendererErrorLogging(): void {
  if (rendererErrorLoggingInstalled || typeof window === "undefined") return;
  rendererErrorLoggingInstalled = true;
  const seen = new Map<string, number>();
  const forward = (line: string): void => {
    const now = Date.now();
    const last = seen.get(line) ?? 0;
    if (now - last < 5_000) return;
    if (seen.size > 200) seen.clear();
    seen.set(line, now);
    try {
      void getActiveRuntimeClient()?.transitionBridge.appendDiagnosticLog?.(line);
    } catch {
      // diagnostics must never break the flow
    }
  };
  window.addEventListener("error", (event) => {
    forward(`window error: ${event.message} @ ${event.filename}:${event.lineno}:${event.colno}`);
  });
  window.addEventListener("unhandledrejection", (event) => {
    forward(`unhandled rejection: ${String(event.reason)}`);
  });
  const originalError = console.error.bind(console);
  console.error = (...args: unknown[]) => {
    forward(`console.error: ${args.map((item) => (item instanceof Error ? item.message : String(item))).join(" ")}`);
    originalError(...args);
  };
}

async function hydrateWailsProfile(client: ReturnType<typeof createProfileClient>): Promise<void> {
  const reader = {
    getRawText: (domain: string, key: string) => getRawText(client, domain, key),
    domainKeys: client.domainKeys ? (domain: string) => client.domainKeys!(domain) : undefined,
  };
  const local = {
    readString: (key: string) => localStorageAdapter.readString(key),
    writeString: (key: string, value: string) => localStorageAdapter.writeString(key, value),
  };
  for (const domain of ["settings", "vault"] as const) {
    const keys = await listHydrationKeys(reader, [], domain);
    await hydrateLocalStorageFromProfile(reader, local, domain, keys);
  }
}
