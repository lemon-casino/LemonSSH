import { useEffect, useMemo, useRef, type RefObject } from "react";
import { netcattyBridge } from "../../infrastructure/services/netcattyBridge";
import { logger } from "../../lib/logger";

export function isNativeFileDrop(data: Pick<DataTransfer, "types">): boolean {
  return data.types.includes("Files") && !!netcattyBridge.get()?.onFilesDropped;
}

// Wails resolves real paths after the DOM drop bubbles to its runtime listener.
export function useNativeFileDrop(
  containerRef: RefObject<HTMLElement | null>,
  ownerKey: string | undefined,
  onDrop: (paths: string[], target: Element | null) => Promise<void>,
): void {
  const targetId = useMemo(() => ownerKey ? crypto.randomUUID() : "", [ownerKey]);
  const onDropRef = useRef(onDrop);
  onDropRef.current = onDrop;
  useEffect(() => {
    const container = containerRef.current;
    const bridge = netcattyBridge.get();
    if (!ownerKey || !container || !bridge?.onFilesDropped) return;
    container.setAttribute("data-file-drop-target", targetId);
    const unsubscribe = bridge.onFilesDropped((payload) => {
      if (!container.isConnected || payload.filenames.length === 0) return;
      const claimed = payload.elementDetails?.attributes?.["data-file-drop-target"];
      const hit = container.ownerDocument.elementFromPoint(payload.x, payload.y);
      const inside = !!hit && container.contains(hit);
      if (claimed ? claimed !== targetId : !inside) return;
      void onDropRef.current(payload.filenames, inside ? hit : null)
        .catch((error) => logger.error("Native file drop failed", error));
    });
    return () => {
      unsubscribe();
      container.removeAttribute("data-file-drop-target");
    };
  }, [containerRef, ownerKey, targetId]);
}
