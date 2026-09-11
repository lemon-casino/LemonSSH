import assert from "node:assert/strict";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import { JSDOM } from "jsdom";

test("WebView2 file and folder drops return through the Go runtime entry point", async () => {
  // Execute the installed runtime, including its real DOM drop listener.
  const bundle = await build({
    entryPoints: [fileURLToPath(new URL("./wailsRuntimeClient.ts", import.meta.url))],
    bundle: true, write: false, format: "iife", globalName: "adapter", platform: "browser",
  });
  const dom = new JSDOM('<div data-file-drop-target="remote-pane"><span>folder</span></div>', {
    url: "http://wails.localhost", runScripts: "outside-only",
  });
  const { window } = dom;
  const target = window.document.querySelector("span")!;
  window.document.elementFromPoint = () => target;
  const paths = ["C:\\Users\\Alice\\report.pdf", "C:\\Users\\Alice\\Documents"];
  const nativeMessages: string[] = [];
  const received: unknown[] = [];
  const errors: unknown[] = [];
  window.addEventListener("error", (event) => { errors.push(event.error); event.preventDefault(); });
  // The Go runtime core creates this empty namespace before navigation completes.
  window.eval('window._wails = { flags: { enableFileDrop: true } }; window.wails = {};');
  Object.assign(window, {
    chrome: { webview: {
      postMessage() {},
      postMessageWithAdditionalObjects(message: string, files: File[]) {
        nativeMessages.push(message);
        assert.equal(files.length, paths.length);
        // Exact callback used by Go's InitiateFrontendDropProcessing after GetPath.
        window.eval(`window.wails.Window.HandlePlatformFileDrop(${JSON.stringify(paths)}, 10, 20);`);
      },
    } },
    fetch: async (_url: unknown, options?: { body?: string }) => {
      if (!options?.body) return { ok: false };
      const request = JSON.parse(options.body);
      assert.equal(request.object, 6);
      assert.equal(request.method, 50);
      window.eval(`window._wails.dispatchWailsEvent(${JSON.stringify({
        name: "netcatty:files-dropped", data: [request.args],
      })});`);
      return { ok: true, status: 200, headers: new Headers(), text: async () => "null", json: async () => null };
    },
  });
  try {
    window.eval(bundle.outputFiles[0].text);
    window.eval('adapter.installWailsRuntimeClient(); window.dropBridge = adapter.createWailsRuntimeClient().transitionBridge;');
    Object.assign(window, { receiveDrop: (payload: unknown) => { received.push(payload); } });
    window.eval('window.dropBridge.onFilesDropped(window.receiveDrop);');
    const files = [new window.File(["pdf"], "report.pdf"), new window.File([], "Documents")];
    const drop = new window.Event("drop", { bubbles: true, cancelable: true });
    Object.assign(drop, {
      clientX: 10, clientY: 20,
      dataTransfer: { types: ["Files"], items: files.map(file => ({ kind: "file", getAsFile: () => file })), files },
    });
    target.dispatchEvent(drop);
    assert.deepEqual(errors, [], "the native callback must resolve to the loaded runtime");
    assert.deepEqual(nativeMessages, ["file:drop:10:20"]);
    assert.equal(received.length, 1, "one external drop must produce one upload event");
    const payload = JSON.parse(JSON.stringify(received[0]));
    assert.deepEqual(payload.filenames, paths);
    assert.equal(payload.elementDetails.attributes["data-file-drop-target"], "remote-pane");
    assert.equal(payload.x, 10);
    assert.equal(payload.y, 20);
  } finally {
    dom.window.close();
  }
});
