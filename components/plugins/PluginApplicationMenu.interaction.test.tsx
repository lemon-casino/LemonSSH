import test from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

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

function installDomGlobals(window: JSDOM["window"]): () => void {
  const previousGlobals = new Map<string, PropertyDescriptor | undefined>();
  const installGlobal = (key: string, value: unknown) => {
    previousGlobals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value,
    });
  };

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }

  installGlobal("window", window);
  installGlobal("document", window.document);
  installGlobal("HTMLElement", window.HTMLElement);
  installGlobal("HTMLInputElement", window.HTMLInputElement);
  installGlobal("HTMLTextAreaElement", window.HTMLTextAreaElement);
  installGlobal("Element", window.Element);
  installGlobal("SVGElement", window.SVGElement);
  installGlobal("Node", window.Node);
  installGlobal("NodeFilter", window.NodeFilter);
  installGlobal("MutationObserver", window.MutationObserver);
  installGlobal("CustomEvent", window.CustomEvent);
  installGlobal("Event", window.Event);
  installGlobal("MouseEvent", window.MouseEvent);
  installGlobal("KeyboardEvent", window.KeyboardEvent);
  installGlobal("getComputedStyle", window.getComputedStyle.bind(window));
  installGlobal("requestAnimationFrame", window.requestAnimationFrame.bind(window));
  installGlobal("cancelAnimationFrame", window.cancelAnimationFrame.bind(window));
  installGlobal("ResizeObserver", ResizeObserverStub);
  installGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return () => {
    for (const [key, descriptor] of previousGlobals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
  };
}

test("application plugin menus mount into the top bar and execute through the bridge", async () => {
  const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
    pretendToBeVisual: true,
    url: "http://localhost",
  });
  const restore = installDomGlobals(dom.window);

  // Components load only after the DOM globals exist (see SftpPaneToolbar.interaction.test.ts).
  const React = (await import("react")).default;
  const { act } = await import("react");
  const { createRoot } = await import("react-dom/client");
  const { I18nProvider } = await import("../../application/i18n/I18nProvider.tsx");
  const { PluginApplicationMenu } = await import("./PluginApplicationMenu.tsx");
  const { TooltipProvider } = await import("../ui/tooltip.tsx");

  const executed: Array<{ command: string; args: unknown; context: unknown }> = [];
  const fakeBridge = {
    getPluginRuntimeStatus: async () => ({ available: true, experimental: true }),
    getPluginContributions: async () => ({
      locale: "en",
      plugins: [applicationMenuPlugin()],
    }),
    executePluginCommand: async (command: string, args?: unknown, context?: unknown) => {
      executed.push({ command, args, context });
      return null;
    },
    onPluginContributionsChanged: () => () => {},
  };
  (dom.window as { lemonssh?: unknown }).lemonssh = fakeBridge;

  const rootNode = dom.window.document.getElementById("root");
  assert.ok(rootNode);
  const root = createRoot(rootNode);

  const flush = async () => {
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
  };

  const click = async (element: Element) => {
    await act(async () => {
      element.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await Promise.resolve();
    });
  };

  try {
    await act(async () => {
      root.render(
        React.createElement(
          I18nProvider,
          { locale: "en" },
          React.createElement(
            TooltipProvider,
            null,
            React.createElement(PluginApplicationMenu),
          ),
        ),
      );
    });
    await flush();

    // Mounted exactly once, straight into the shared utility row styling.
    const trigger = rootNode.querySelector('[data-section="plugin-application-menu-trigger"]');
    assert.ok(trigger, "expected the application plugin menu trigger to mount");
    assert.equal(trigger?.getAttribute("aria-label"), "Plugins");

    // Open: only the application-surface entries render (statusBar stays out).
    // The popover portals to document.body, so query from the document.
    await click(trigger);
    const menu = dom.window.document.querySelector('[data-section="plugin-application-menu"]');
    assert.ok(menu, "expected the plugin application menu panel to open");
    const items = [...menu.querySelectorAll("[data-plugin-menu-item]")];
    assert.deepEqual(items.map((item) => item.getAttribute("data-plugin-menu-item")), [
      "com.example.appmenu:menu:0",
      "com.example.appmenu:menu:disabled",
    ]);
    assert.match(items[0]?.textContent ?? "", /Deploy fleet/);
    assert.match(items[0]?.textContent ?? "", /ctrl\+alt\+d/u);
    assert.equal((items[1] as HTMLButtonElement).disabled, true);

    // Clicking the disabled entry must not reach the command chain.
    await click(items[1]);
    assert.equal(executed.length, 0);

    // Clicking the entry executes the plugin command with the application surface context.
    await click(items[0]);
    assert.deepEqual(executed, [{
      command: "com.example.appmenu.deploy",
      args: undefined,
      context: { "lemonssh.surface": "application" },
    }]);

    // Execution closes the menu again (no lingering empty panel).
    await flush();
    assert.equal(
      dom.window.document.querySelector('[data-section="plugin-application-menu"]'),
      null,
      "menu panel should close after executing a command",
    );
  } finally {
    await act(async () => {
      root.unmount();
    });
    restore();
  }
});

test("plugins without application menus leave the top bar untouched", async () => {
  const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
    pretendToBeVisual: true,
    url: "http://localhost",
  });
  const restore = installDomGlobals(dom.window);

  const React = (await import("react")).default;
  const { act } = await import("react");
  const { createRoot } = await import("react-dom/client");
  const { I18nProvider } = await import("../../application/i18n/I18nProvider.tsx");
  const { PluginApplicationMenu } = await import("./PluginApplicationMenu.tsx");
  const { TooltipProvider } = await import("../ui/tooltip.tsx");

  const plugin = applicationMenuPlugin();
  const fakeBridge = {
    getPluginRuntimeStatus: async () => ({ available: true, experimental: true }),
    getPluginContributions: async () => ({
      locale: "en",
      plugins: [{
        ...plugin,
        menus: plugin.menus.filter((menu) => menu.location !== "application"),
      }],
    }),
    executePluginCommand: async () => null,
    onPluginContributionsChanged: () => () => {},
  };
  (dom.window as { lemonssh?: unknown }).lemonssh = fakeBridge;

  const rootNode = dom.window.document.getElementById("root");
  assert.ok(rootNode);
  const root = createRoot(rootNode);

  try {
    await act(async () => {
      root.render(
        React.createElement(
          I18nProvider,
          { locale: "en" },
          React.createElement(
            TooltipProvider,
            null,
            React.createElement(PluginApplicationMenu),
          ),
        ),
      );
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    assert.equal(
      rootNode.querySelector('[data-section="plugin-application-menu-trigger"]'),
      null,
      "no plugin application menu trigger expected without application contributions",
    );
  } finally {
    await act(async () => {
      root.unmount();
    });
    restore();
  }
});
