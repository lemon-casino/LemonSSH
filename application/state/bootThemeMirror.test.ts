import assert from "node:assert/strict";
import test from "node:test";

test("applyThemeTokens mirrors the resolved scheme palette for the boot splash", async () => {
  const { installDomEnvironment } = await import("../../components/test-support/renderReactDom.tsx");
  const dom = installDomEnvironment();
  const store = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => { store.set(k, v); },
      removeItem: (k: string) => { store.delete(k); },
    },
  });
  const { applyThemeTokens } = await import("./settingsStateDefaults.ts");
  const { LIGHT_UI_THEMES } = await import("../../infrastructure/config/uiThemes.ts");
  const tokens = LIGHT_UI_THEMES[0].tokens;

  applyThemeTokens("system", "light", tokens, "theme", "");

  const mirror = JSON.parse(store.get("netcatty_boot_theme_v1") ?? "null");
  assert.ok(mirror, "mirror written");
  assert.equal(mirror.light.background, tokens.background);
  assert.equal(mirror.light.accent, tokens.accent);

  // Second apply for the dark scheme merges without clobbering light.
  const { DARK_UI_THEMES } = await import("../../infrastructure/config/uiThemes.ts");
  applyThemeTokens("dark", "dark", DARK_UI_THEMES[0].tokens, "theme", "");
  const merged = JSON.parse(store.get("netcatty_boot_theme_v1") ?? "null");
  assert.ok(merged.light && merged.dark);
});
