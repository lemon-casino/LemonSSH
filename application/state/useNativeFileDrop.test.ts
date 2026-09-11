import assert from "node:assert/strict";
import test from "node:test";
import { createElement } from "react";
import { act, create, type ReactTestRenderer } from "react-test-renderer";
import { JSDOM } from "jsdom";
import { createElectronRuntimeClient } from "../../infrastructure/runtime/electron/electronRuntimeClient";
import { getActiveRuntimeClient, setActiveRuntimeClient } from "../../infrastructure/runtime/runtimeClient";
import { useSftpPaneDragAndSelect } from "../../components/sftp/hooks/useSftpPaneDragAndSelect";

test("native drops reach only the intended pane and folder once, and stale targets are removed", async () => {
  const previousClient = getActiveRuntimeClient();
  const dom = new JSDOM('<div id="left"><div data-entry-type="directory" data-entry-name="assets" data-entry-path="/srv/nested/assets"></div></div><div id="right"></div>');
  const document = dom.window.document;
  const containers = [document.getElementById("left")!, document.getElementById("right")!];
  document.elementFromPoint = () => containers[0].firstElementChild;
  type DropCallback = Parameters<NonNullable<NetcattyBridge["onFilesDropped"]>>[0];
  const listeners = new Set<DropCallback>();
  const uploaded: unknown[] = [];
  let domUploads = 0;
  let stopped = 0;
  const handlers: ReturnType<typeof useSftpPaneDragAndSelect>[] = [];
  setActiveRuntimeClient(createElectronRuntimeClient({
    onFilesDropped: (callback: DropCallback) => {
      listeners.add(callback);
      return () => { listeners.delete(callback); };
    },
  } as NetcattyBridge));
  function Probe({ index, connectionId }: { index: number; connectionId: string }) {
    const result = useSftpPaneDragAndSelect({
      side: index === 0 ? "left" : "right",
      pane: { selectedFiles: new Set(), connection: { id: connectionId, currentPath: "/srv" } },
      sortedDisplayFiles: [], draggedFiles: null,
      onDragStart() {}, onReceiveFromOtherPane() {}, onMoveEntriesToPath: async () => {},
      onUploadExternalFiles: async () => { domUploads++; },
      onUploadExternalPaths: async (paths, target) => { uploaded.push({ index, paths, target }); },
      onOpenEntry() {}, onRangeSelect() {}, onToggleSelection() {},
    });
    handlers[index] = result;
    return createElement("div", { ref: result.paneContainerRef, "data-index": index });
  }
  const render = (connectionId: string) => createElement("div", null,
    createElement(Probe, { index: 0, connectionId }),
    createElement(Probe, { index: 1, connectionId: "other" }));
  const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const previousAct = actEnvironment.IS_REACT_ACT_ENVIRONMENT;
  actEnvironment.IS_REACT_ACT_ENVIRONMENT = true;
  let renderer: ReactTestRenderer | undefined;
  try {
    await act(async () => { renderer = create(render("original"), {
      createNodeMock: (element) => containers[element.props["data-index"]],
    }); });
    const targetId = containers[0].getAttribute("data-file-drop-target");
    assert.ok(targetId);
    assert.notEqual(targetId, containers[1].getAttribute("data-file-drop-target"));
    const event = {
      dataTransfer: { types: ["Files"], items: [{}], files: [{}] },
      preventDefault() {}, stopPropagation() { stopped++; },
    } as unknown as Parameters<typeof handlers[0]["handlePaneDrop"]>[0];
    await act(async () => { await handlers[0].handlePaneDrop(event); });
    assert.equal(stopped, 0, "Wails must receive the bubbling DOM drop");
    assert.equal(domUploads, 0, "DOM upload must yield to the native event");
    const payload = { filenames: ["C:\\real.pdf"], x: 10, y: 20,
      elementDetails: { attributes: { "data-file-drop-target": targetId! } } };
    await act(async () => { for (const callback of listeners) callback(payload); });
    assert.deepEqual(uploaded, [{ index: 0, paths: ["C:\\real.pdf"], target: "/srv/nested/assets" }]);
    await act(async () => {
      for (const callback of listeners) callback({ filenames: ["C:\\by-point.pdf"], x: 10, y: 20 });
    });
    assert.deepEqual(uploaded[1], { index: 0, paths: ["C:\\by-point.pdf"], target: "/srv/nested/assets" });
    await act(async () => { renderer!.update(render("replacement")); });
    await act(async () => { for (const callback of listeners) callback(payload); });
    assert.equal(uploaded.length, 2, "late native events must not upload to a different connection");
    await act(async () => { renderer!.unmount(); });
    renderer = undefined;
    assert.equal(listeners.size, 0);
    assert.equal(containers[0].hasAttribute("data-file-drop-target"), false);
  } finally {
    if (renderer) await act(async () => { renderer!.unmount(); });
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = previousAct;
    setActiveRuntimeClient(previousClient);
    dom.window.close();
  }
});
