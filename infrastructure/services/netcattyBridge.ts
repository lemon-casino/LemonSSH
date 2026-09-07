import { getActiveRuntimeClient } from "../runtime/runtimeClient";
import { getElectronBridge } from "../runtime/electron/electronRuntimeClient";

export class BridgeUnavailableError extends Error {
  constructor(message = "Netcatty bridge unavailable") {
    super(message);
    this.name = "BridgeUnavailableError";
  }
}

// Transition facade (P1-01). Resolves the active RuntimeClient when a shell
// adapter registered itself and falls back to the Electron bridge exactly as
// before. Callers migrate to RuntimeClient domain ports slice by slice; this
// object is deleted with the Electron path.

export const netcattyBridge = {
  get(): NetcattyBridge | undefined {
    return getActiveRuntimeClient()?.transitionBridge ?? getElectronBridge();
  },

  require(): NetcattyBridge {
    const bridge = this.get();
    if (!bridge) throw new BridgeUnavailableError();
    return bridge;
  },
};
