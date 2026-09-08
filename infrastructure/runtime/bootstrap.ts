import { installElectronRuntimeClient } from "./electron/electronRuntimeClient";
import { installWailsRuntimeClient } from "./wails/wailsRuntimeClient";
import { createProfileClient } from "./profile/profileClient";
import { configureHostProfileClient } from "../persistence/hostStorageAdapter";

// Runtime selection bootstrap (P1-02). Installs the Wails RuntimeClient when
// the bundle runs under the Wails shell and the Electron adapter otherwise.
// Electron remains the default dev/release shell, so the Electron path must
// behave exactly as before this module existed.

export function installRuntimeClient(): void {
  if (installWailsRuntimeClient()) {
    configureHostProfileClient(createProfileClient());
    return;
  }
  // Electron remains the stable release shell; localStorage is canonical
  // until P2-07's cutover gate moves a domain to the Go profile store.
  configureHostProfileClient(undefined);
  installElectronRuntimeClient();
}
