import { Events } from "@wailsio/runtime";
import { Terminal } from "@xterm/xterm";
import { WebglAddon } from "@xterm/addon-webgl";
import * as monaco from "monaco-editor";
import "@xterm/xterm/css/xterm.css";
import {
  ProbeService,
  type ProbeEvent,
  type WindowStatus,
} from "../bindings/github.com/binaricat/netcatty/experiments/wails-shell-probe";

const params = new URLSearchParams(location.search);
const currentRole = params.get("role") || "main";
const currentToken = params.get("token") || "pending";
const windowList = document.querySelector<HTMLDivElement>("#window-list")!;
const eventList = document.querySelector<HTMLOListElement>("#events")!;
const checks = document.querySelector<HTMLDivElement>("#checks")!;

document.querySelector("#identity")!.textContent = `${currentRole} / ${currentToken}`;

function button(label: string, action: () => Promise<unknown>) {
  const element = document.createElement("button");
  element.type = "button";
  element.textContent = label;
  element.addEventListener("click", () => void action().then(refresh).catch(showError));
  return element;
}

function renderWindows(windows: WindowStatus[]) {
  windowList.replaceChildren();
  for (const status of windows) {
    const row = document.createElement("article");
    row.className = "window-row";
    const summary = document.createElement("div");
    const roleName = document.createElement("strong");
    roleName.textContent = status.role;
    const state = document.createElement("span");
    state.textContent = `${status.open ? "open" : "closed"} / handle ${status.nativeHandleReady ? "ready" : "pending"}`;
    summary.append(roleName, state);
    const actions = document.createElement("div");
    actions.className = "actions";
    actions.append(
      button(status.open ? "Focus" : "Open", () => status.open
        ? ProbeService.FocusWindow(currentRole, currentToken, status.role)
        : ProbeService.OpenWindow(currentRole, currentToken, status.role)),
      button("Reload", () => ProbeService.ReloadWindow(currentRole, currentToken, status.role)),
      button(status.closeVeto ? "Disable veto" : "Enable veto", () => ProbeService.SetCloseVeto(currentRole, currentToken, status.role, !status.closeVeto)),
      button("Native dialog", () => ProbeService.ShowNativeDialog(currentRole, currentToken, status.role)),
      button("Close", () => ProbeService.CloseWindow(currentRole, currentToken, status.role)),
    );
    row.append(summary, actions);
    windowList.append(row);
  }
}

function renderEvents(events: ProbeEvent[]) {
  eventList.replaceChildren(...events.slice(-30).reverse().map((event) => {
    const item = document.createElement("li");
    item.textContent = `${event.kind}: ${event.detail}`;
    return item;
  }));
}

async function refresh() {
  const snapshot = await ProbeService.Snapshot(currentRole, currentToken);
  document.querySelector("#identity")!.textContent = `${snapshot.platform} / ${currentRole} / ${currentToken}`;
  renderWindows(snapshot.windows ?? []);
  renderEvents(snapshot.events ?? []);
}

function showError(error: unknown) {
  const item = document.createElement("li");
  item.textContent = `error: ${String(error)}`;
  eventList.prepend(item);
}

function addCheck(name: string, passed: boolean, detail: string) {
  const row = document.createElement("div");
  row.className = `check ${passed ? "pass" : "fail"}`;
  row.innerHTML = `<strong>${name}</strong><span>${passed ? "PASS" : "FAIL"}</span><small>${detail}</small>`;
  checks.append(row);
}

async function runWebViewChecks() {
  const canvas = document.createElement("canvas");
  const webgl = canvas.getContext("webgl2") || canvas.getContext("webgl");
  addCheck("WebGL", Boolean(webgl), webgl ? String(webgl.getParameter(webgl.VERSION)) : "No context");

  try {
    const module = await WebAssembly.instantiate(new Uint8Array([0, 97, 115, 109, 1, 0, 0, 0]));
    addCheck("WASM", Boolean(module.instance), "Minimal module instantiated");
  } catch (error) {
    addCheck("WASM", false, String(error));
  }

  addCheck("Clipboard API", Boolean(navigator.clipboard), navigator.clipboard ? "Available" : "Unavailable");
  addCheck("IME events", "CompositionEvent" in window, "CompositionEvent" in window ? "Available" : "Unavailable");
}

const terminal = new Terminal({ cols: 52, rows: 8, convertEol: true, theme: { background: "#111418", foreground: "#d7dde5" } });
terminal.open(document.querySelector<HTMLDivElement>("#terminal")!);
try {
  terminal.loadAddon(new WebglAddon());
  terminal.writeln("WebGL renderer loaded");
} catch (error) {
  terminal.writeln(`DOM fallback: ${String(error)}`);
}
terminal.writeln("Unicode: Netcatty / 终端 / terminal");

monaco.editor.create(document.querySelector<HTMLDivElement>("#editor")!, {
  value: "// Monaco loaded inside Wails\nconst shell = 'probe';\n",
  language: "typescript",
  minimap: { enabled: false },
  automaticLayout: true,
  theme: "vs-dark",
});

document.querySelector<HTMLButtonElement>("#clipboard")!.addEventListener("click", async () => {
  try {
    const value = `Netcatty clipboard probe ${Date.now()}`;
    await navigator.clipboard.writeText(value);
    addCheck("Clipboard round-trip", await navigator.clipboard.readText() === value, "User-gesture read/write");
  } catch (error) {
    addCheck("Clipboard round-trip", false, String(error));
  }
});

document.querySelector<HTMLButtonElement>("#refresh")!.addEventListener("click", () => void refresh());
Events.On("probe:event", () => void refresh());
void runWebViewChecks();
void refresh();
