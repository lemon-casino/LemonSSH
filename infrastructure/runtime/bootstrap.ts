import { installElectronRuntimeClient } from "./electron/electronRuntimeClient";
import { installWailsRuntimeClient } from "./wails/wailsRuntimeClient";

// Runtime selection bootstrap (P1-02). Installs the Wails RuntimeClient when
// the bundle runs under the Wails shell and the Electron adapter otherwise.
// Electron remains the default dev/release shell, so the Electron path must
// behave exactly as before this module existed.

export function installRuntimeClient(): void {
  if (installWailsRuntimeClient()) return;
  installElectronRuntimeClient();
}
