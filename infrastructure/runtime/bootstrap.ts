import { installWailsRuntimeClient } from "./wails/wailsRuntimeClient";
import { createProfileClient } from "./profile/profileClient";
import { getActiveRuntimeClient } from "./runtimeClient";
import { configureHostProfileClient, hydrateHostProfile } from "../persistence/hostStorageAdapter";
// LemonSSH has one desktop runtime. The frontend must fail early when it is
// loaded outside the Wails host instead of silently selecting a legacy shell.

export let hydrateReady: Promise<void> = Promise.resolve();

export function installRuntimeClient(): void {
  if (!installWailsRuntimeClient()) {
    throw new Error("LemonSSH requires the Wails runtime");
  }
  const client = createProfileClient();
  configureHostProfileClient(client);
  installRendererErrorLogging();
  hydrateReady = hydrateHostProfile();
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
