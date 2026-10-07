import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { resolveCloseAction, shouldPromptForCloseBehavior } from "./closeBehavior.ts";

test("first close prompts until a habit is chosen", () => {
  assert.equal(shouldPromptForCloseBehavior(null), true);
  assert.equal(shouldPromptForCloseBehavior(undefined), true);
  assert.equal(shouldPromptForCloseBehavior("minimize"), false);
  assert.equal(shouldPromptForCloseBehavior("quit"), false);
  assert.equal(resolveCloseAction(null), "prompt");
  assert.equal(resolveCloseAction("minimize"), "minimize");
  assert.equal(resolveCloseAction("quit"), "quit");
});

test("window close button asks once then follows the chosen habit", () => {
  const source = readFileSync(join(process.cwd(), "components/top-tabs/TopTabItems.tsx"), "utf8");
  assert.match(source, /resolveCloseAction/);
  assert.match(source, /closeBehaviorPrompt/);
  assert.match(source, /setCloseBehavior/);
});

test("asking each time is not inferred from the old close-to-tray flag", () => {
  const source = readFileSync(join(process.cwd(), "application/state/useSettingsState.ts"), "utf8");
  assert.match(source, /STORAGE_KEY_CLOSE_BEHAVIOR/);
  assert.doesNotMatch(
    source,
    /if \(readStoredString\(STORAGE_KEY_CLOSE_TO_TRAY\) !== null\)/,
  );
});

test("settings window close-behavior changes sync to the main window", () => {
  const source = readFileSync(join(process.cwd(), "application/state/settingsIpcSync.ts"), "utf8");
  assert.match(source, /STORAGE_KEY_CLOSE_BEHAVIOR/);
  assert.match(source, /setCloseBehavior/);
  // Wails windows share localStorage, so the storage-event path must carry the
  // change too; otherwise the main window keeps the old close behavior.
  const storageSync = readFileSync(join(process.cwd(), "application/state/settingsStorageSync.ts"), "utf8");
  assert.match(storageSync, /STORAGE_KEY_CLOSE_BEHAVIOR/);
  assert.match(storageSync, /setCloseBehavior\(e\.newValue\)/);
});

test("every hook dependency is declared in its parameter destructuring", () => {
  const source = readFileSync(join(process.cwd(), "application/state/settingsIpcSync.ts"), "utf8");
  const paramsStart = source.indexOf("}: UseSettingsIpcSyncParams) {");
  assert.notEqual(paramsStart, -1, "expected the hook parameter destructuring");
  const destructuring = source.slice(0, paramsStart);
  const declared = new Set(
    destructuring
      .split("\n")
      .map((line) => line.trim().replace(/,$/, ""))
      .filter((name) => /^[A-Za-z_][A-Za-z0-9_]*$/.test(name)),
  );
  assert.ok(declared.has("setCloseBehavior"), "setCloseBehavior must be destructured, not just listed in deps");
});

test("Wails quitApp terminates the process instead of hiding the window", () => {
  const source = readFileSync(join(process.cwd(), "infrastructure/runtime/wails/wailsRuntimeClient.ts"), "utf8");
  assert.match(source, /quitApp:[\s\S]*bindings\.tray\?\.Quit/);
  assert.doesNotMatch(source, /quitApp:[\s\S]*Application\.Quit\(\)/);
});

test("settings habits tab owns the close-behavior switch", () => {
  const settingsPage = readFileSync(join(process.cwd(), "components/SettingsPage.tsx"), "utf8");
  const habitsTab = readFileSync(join(process.cwd(), "components/settings/tabs/SettingsHabitsTab.tsx"), "utf8");
  assert.match(settingsPage, /value="habits"/);
  assert.match(settingsPage, /settings\.tab\.habits/);
  assert.match(habitsTab, /settings.habits.closeBehavior/);
  assert.match(habitsTab, /setCloseBehavior/);
  assert.doesNotMatch(
    readFileSync(join(process.cwd(), "components/settings/tabs/SettingsSystemTab.tsx"), "utf8"),
    /closeToTray/,
  );
});
