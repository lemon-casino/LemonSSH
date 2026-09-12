import assert from "node:assert/strict";
import test from "node:test";
import React from "react";

import {
  createDomRenderer,
  dispatchDomEvent,
  flushEffects,
  installDomEnvironment,
} from "./test-support/renderReactDom.tsx";
import { TooltipProvider } from "./ui/tooltip.tsx";
import { VaultNavItems } from "./VaultNavItems.tsx";

const t = ((key: string) => key) as never as (key: string) => string;

function renderNav(props: Record<string, unknown>) {
  return React.createElement(
    TooltipProvider,
    null,
    React.createElement(VaultNavItems, props as never),
  );
}

const SECTION_LABEL_KEYS = [
  "vault.nav.hosts",
  "vault.nav.keychain",
  "vault.nav.proxies",
  "vault.nav.portForwarding",
  "vault.nav.scripts",
  "vault.nav.notes",
  "vault.nav.knownHosts",
  "vault.nav.logs",
] as const;

function clickElement(env: ReturnType<typeof installDomEnvironment>, element: Element) {
  return dispatchDomEvent(
    element,
    new env.window.MouseEvent("click", { bubbles: true, cancelable: true }),
  );
}

test("VaultNavItems vertical rail renders all sections and reports selections", async () => {
  const env = installDomEnvironment();
  const renderer = await createDomRenderer(env.document);
  const selected: string[] = [];

  await renderer.render(
    renderNav({
      currentSection: "hosts",
      onSelectSection: (section: string) => selected.push(section),
      sidebarCollapsed: false,
      t,
    }),
  );
  await flushEffects();

  const buttons = Array.from(
    renderer.container.querySelectorAll("button"),
  );
  const labels = buttons
    .map((button) => button.textContent?.trim())
    .filter(Boolean);
  for (const key of SECTION_LABEL_KEYS) {
    assert.ok(labels.includes(key), `missing label for ${key}`);
  }

  const keysButton = buttons.find((button) =>
    button.textContent?.includes("vault.nav.keychain"),
  );
  assert.ok(keysButton, "keychain button missing");
  await clickElement(env, keysButton as Element);
  assert.deepEqual(selected, ["keys"]);
  await renderer.unmount();
});

test("VaultNavItems horizontal menu bar marks the active section and follows controlled updates", async () => {
  const env = installDomEnvironment();
  const renderer = await createDomRenderer(env.document);
  const selected: string[] = [];

  await renderer.render(
    renderNav({
      orientation: "horizontal",
      currentSection: "hosts",
      onSelectSection: (section: string) => selected.push(section),
      sidebarCollapsed: false,
      t,
    }),
  );
  await flushEffects();

  const hosts = renderer.container.querySelector(
    '[data-section="workbench-menu-hosts"]',
  );
  const logs = renderer.container.querySelector(
    '[data-section="workbench-menu-logs"]',
  );
  assert.ok(hosts && logs, "horizontal menu items missing");
  assert.equal(hosts.getAttribute("data-state"), "active");
  assert.equal(logs.getAttribute("data-state"), "inactive");

  await clickElement(env, logs);
  assert.deepEqual(selected, ["logs"]);

  // Controlled mode: the caller owns section state; a new currentSection
  // prop must move the active highlight.
  await renderer.render(
    renderNav({
      orientation: "horizontal",
      currentSection: "logs",
      onSelectSection: (section: string) => selected.push(section),
      sidebarCollapsed: false,
      t,
    }),
  );
  await flushEffects();
  const logsAfter = renderer.container.querySelector(
    '[data-section="workbench-menu-logs"]',
  );
  assert.equal(logsAfter?.getAttribute("data-state"), "active");
  await renderer.unmount();
});
