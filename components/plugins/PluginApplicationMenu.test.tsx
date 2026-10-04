import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";

import {
  PluginApplicationMenu,
  PluginApplicationMenuContent,
  PluginApplicationMenuItems,
  shouldRenderPluginApplicationMenu,
} from "./PluginApplicationMenu.tsx";
import type { PluginMenuItem } from "../../application/state/usePluginMenuItems.ts";
import { TooltipProvider } from "../ui/tooltip.tsx";

const componentSource = readFileSync(new URL("./PluginApplicationMenu.tsx", import.meta.url), "utf8");

function applicationMenuPlugin(): LemonSSHPluginContributionSnapshot["plugins"][number] {
  return {
    id: "com.example.appmenu",
    version: "1.0.0",
    displayName: "App Menu Plugin",
    description: "",
    commands: [{ id: "com.example.appmenu.deploy", title: "Deploy", enabled: true }],
    keybindings: [],
    menus: [
      {
        id: "com.example.appmenu:menu:0",
        command: "com.example.appmenu.deploy",
        alt: "com.example.appmenu.deploy.alternate",
        location: "application",
        title: "Deploy fleet",
        visible: true,
        enabled: true,
        shortcut: "ctrl+alt+d",
      },
      {
        id: "com.example.appmenu:menu:disabled",
        command: "com.example.appmenu.deploy",
        location: "application",
        title: "Disabled action",
        visible: true,
        enabled: false,
      },
      {
        // Another surface: must never leak into the application menu.
        id: "com.example.appmenu:menu:status",
        command: "com.example.appmenu.deploy",
        location: "statusBar",
        title: "Status only",
        visible: true,
        enabled: true,
      },
    ],
    settings: [],
    views: [],
  };
}

function menuFixture(menuIndex: number): PluginMenuItem {
  const menu = applicationMenuPlugin().menus[menuIndex];
  return { ...menu, pluginId: "com.example.appmenu" } as PluginMenuItem;
}

test("empty application contributions render no menu shell", () => {
  assert.equal(shouldRenderPluginApplicationMenu([]), false);
  assert.equal(
    renderToStaticMarkup(
      React.createElement(TooltipProvider, null, React.createElement(PluginApplicationMenu)),
    ),
    "",
  );
  const emptyContent = renderToStaticMarkup(
    React.createElement(PluginApplicationMenuContent, {
      items: [],
      open: false,
      onOpenChange: () => {},
      label: "Plugins",
      onExecute: () => {},
    }),
  );
  assert.equal(emptyContent, "");
});

test("application menus keep placement, icon and shortcut passthrough", () => {
  const markup = renderToStaticMarkup(
    React.createElement(PluginApplicationMenuItems, {
      items: [menuFixture(0)],
      label: "Plugins",
      onExecute: () => {},
    }),
  );

  // The item list renders with title, shortcut and stable item hooks.
  assert.match(markup, /Deploy fleet/u);
  assert.match(markup, /ctrl\+alt\+d/u);
  assert.match(markup, /data-plugin-menu-item="com\.example\.appmenu:menu:0"/u);
  assert.match(markup, /role="menuitem"/u);
  assert.match(markup, /aria-label="Plugins"/u);
});

test("disabled entries stay unclickable", () => {
  const markup = renderToStaticMarkup(
    React.createElement(PluginApplicationMenuItems, {
      items: [menuFixture(1)],
      label: "Plugins",
      onExecute: () => {},
    }),
  );
  assert.match(markup, /disabled/u);
});

test("the mount reuses the shared plugin command execution chain", () => {
  // Components consume the application hook only: no second native dispatch shape.
  assert.match(componentSource, /usePluginMenuItems\('application'/u);
  assert.match(componentSource, /executeCommand\(/u);
  assert.doesNotMatch(componentSource, /CallNative|CallPlugin|lemonsshBridge/u);
  // The empty case never keeps the trigger or panel mounted.
  assert.match(componentSource, /if \(!shouldRenderPluginApplicationMenu\(items\)\) return null;/u);
});
