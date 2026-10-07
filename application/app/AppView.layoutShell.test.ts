import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const here = dirname(fileURLToPath(import.meta.url));
const appViewSource = readFileSync(join(here, "AppView.tsx"), "utf8");
const layerSource = readFileSync(
  join(here, "AppWorkbenchSessionLayer.tsx"),
  "utf8",
);
const treeSource = readFileSync(
  join(here, "../../components/workbench/WorkbenchSessionTree.tsx"),
  "utf8",
);
const vaultViewSource = readFileSync(
  join(here, "../../components/VaultView.tsx"),
  "utf8",
);
const vaultViewLayoutSource = readFileSync(
  join(here, "../../components/vault/VaultViewLayout.tsx"),
  "utf8",
);
const vaultNavStoreSource = readFileSync(
  join(here, "../state/vaultNavStore.ts"),
  "utf8",
);

test("AppView swaps only the shell and never keys content by layout mode", () => {
  assert.match(appViewSource, /layoutMode === 'classic' \? \(/);
  assert.match(appViewSource, /<WorkbenchChrome/);
  // The layout switch must live in the chrome slot only; content nodes must
  // not be re-keyed or conditionally wrapped per layout.
  assert.doesNotMatch(appViewSource, /key=\{layoutMode\}/);
  assert.doesNotMatch(
    appViewSource,
    /layoutMode === 'workbench' &&\s*\n?\s*<AppHostTreeLayer/,
  );
});

test("workbench session layer stays mounted and toggles via visibility", () => {
  // Always rendered as a flex sibling of the content container; the enabled
  // prop drives visibility/width, never conditional mounting.
  assert.match(
    appViewSource,
    /<AppWorkbenchSessionLayer\s*\n\s*enabled=\{layoutMode === 'workbench'\}/,
  );
  assert.match(
    layerSource,
    /getAppHostTreeLayerStyle\(surfaceVisible\)/,
    "session layer must reuse the visibility style helper",
  );
});

test("workbench merges the host tree into the session tree and disables the floating overlay", () => {
  // In workbench mode the host tree must not open as a second floating
  // column next to the session tree; the merged sidebar tree lists every
  // host with its sessions and the host toolbar sits above the tree.
  assert.match(
    appViewSource,
    /enabled=\{showHostTreeSidebar && layoutMode !== 'workbench'\}/,
    "overlay host tree must be disabled in workbench mode",
  );
  assert.match(
    layerSource,
    /includeAllHosts: true/,
    "merged tree must list session-less hosts as connect entries",
  );
  assert.match(
    layerSource,
    /TerminalHostTreeToolbar/,
    "host toolbar must render above the merged tree",
  );
  assert.doesNotMatch(
    appViewSource,
    /layoutMode === 'workbench' &&\s*\n?\s*<AppHostTreeLayer/,
  );
  assert.match(
    appViewSource,
    /surfaceVisible=\{layoutMode === 'workbench' \? true : undefined\}/,
    "workbench new-host overlay must stay visible on vault/sftp tabs",
  );
  assert.match(
    layerSource,
    /onNewHost=\{onNewHost\}/,
    "merged tree group menus must receive the new-host opener",
  );
});

test("workbench session layer owns the activeTabId subscription, not AppView", () => {
  assert.match(layerSource, /useActiveTabId\(\)/);
  assert.match(layerSource, /memo\(/);
});

test("vault sidebar is hidden via prop in workbench mode, not unmounted", () => {
  assert.match(appViewSource, /showSidebar=\{layoutMode === 'classic'\}/);
  assert.match(vaultViewLayoutSource, /showSidebar === false && "hidden"/);
});

test("workbench menu bar drives sections through vaultNavStore", () => {
  assert.match(vaultViewSource, /registerVaultNav\(\{ setCurrentSection: selectVaultSection \}\)/);
  assert.match(vaultViewSource, /syncVaultNavSection\(currentSection\)/);
  assert.match(vaultNavStoreSource, /export function syncVaultNavSection/);
});

test("workbench tree does not bypass the persistence adapters", () => {
  assert.doesNotMatch(
    treeSource,
    /localStorage\./,
    "tree must not touch localStorage directly",
  );
  assert.match(
    treeSource,
    /FixedSizeVirtualList/,
    "tree rows must render through the shared virtual list",
  );
});
