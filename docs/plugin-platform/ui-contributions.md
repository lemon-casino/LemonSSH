# Plugin UI contributions (declarative)

Status: implemented for settings, declarative views (settings location),
menus and keybindings. There is still no plugin-owned view runtime: every
user-visible surface is rendered by LemonSSH's own components from the
validated schema.

Plugins cannot ship UI code. Everything user-visible is declared in the
manifest, validated by the host, and rendered by LemonSSH's own components.
There is no path that injects plugin HTML/JS/CSS into the renderer.

## Declarative schema

`internal/plugin/ui` validates the manifest `ui` block:

- `settings`: form fields with `id`, `type` (`text` | `number` | `boolean` |
  `select` | `password`), `label`, optional `description`, `default`,
  `required`, and `options` for selects. IDs must match
  `^[a-z0-9._-]{1,128}$` (lowercase alphanumerics plus `-`, `.` and `_`),
  labels
  and descriptions pass an injection-vector check (`<script`, `javascript:`,
  `onerror=`, template interpolation, …), and selects must declare options.
- `views`: `list` or `card` definitions with `id`, `type`, `title`, optional
  `columns`, `bindings`, `location` and `visible`. The only supported
  `location` is `settings` (anything else fails validation); `visible`
  defaults to true. The same ID-uniqueness and text-injection rules apply to
  view titles, columns and binding keys, and views are capped at 64
  columns/bindings.
- `menus`: `id`, `command`, `location` (`commandPalette` | `application` |
  `host/context` | `terminal/context` | `terminal/toolbar` | `statusBar`),
  optional `alt` (alternate command), `title` override, `group`, `order` and
  `visible` (default true). The referenced command must exist in the
  manifest's `contributions` (`type: "command"`); the manifest package
  cross-validates this at install time, so a menu that can never execute is
  rejected instead of surfacing as a dead entry.
- `keybindings`: `command` (declared contribution command), `key` plus
  optional `mac` / `linux` / `windows` overrides, optional `args` (pre-validated
  JSON) and `enabled` (default true). Accelerators are restricted to
  modifier/key ASCII tokens joined by `+`.

The frontend reads this schema through `PluginService.UISchema` and renders
it with host-owned controls in Settings → Plugins
(`components/settings/tabs/SettingsPluginsTab.tsx`): switches, selects, text,
password (OS-keyring sealed on write), number, keybinding, font, list/table and
path pickers. Every write goes through `PluginService.SetSetting`, which
validates the value against the declared type before storage; undeclared
setting IDs are rejected.

The Go host additionally aggregates the schema-derived contributions of every
enabled plugin through `PluginService.UIContributions`
(`internal/plugin/host.Host.UIContributions` → `ui.CollectContributions`),
which resolves the nil-means-default rules (views and menus visible,
keybindings enabled, views default to the `settings` location) and filters
menu/keybinding entries whose command is not declared. The Wails renderer
consumes the per-plugin schema through the plugin bridge
(`infrastructure/runtime/wails/pluginBridge.ts`).

## Commands and menus

A manifest may declare `contributions` of `type: "command"`. The bridge lists
commands and schema-declared menus for palette-style surfaces: the
QuickSwitcher renders `commandPalette` menus (grouped and ordered, falling
back to the command id as title).

Command execution tries the live native companion first
(`NativeRunning` + `CallNative`); when no companion process is running, the
bridge falls through to the lemonssh-wasm-abi v1 dispatch channel
(`CallPlugin` with method `command.execute`). A plugin-declared failure
surfaces as a visible `Plugin command … failed: <message>` error; plugins
without any handler surface a clear error instead of failing silently.

Menus are declared for one of six designed locations — `commandPalette`,
`application`, `host/context`, `terminal/context`, `terminal/toolbar` and
`statusBar`. `PluginService.UIContributions` delivers every visible entry
through the contributions snapshot, so each mounting surface (the
QuickSwitcher for `commandPalette` entries, and the application, context
menu, toolbar and status bar surfaces for theirs) receives exactly the
entries declared for its location.

## Keybindings

Schema-declared keybindings are surfaced in the contributions snapshot and
registered by `usePluginViewLifecycle`, which matches normalized keyboard
events against the declared accelerator (with platform overrides) and invokes
the bound command through the same execution path as menus. Disable a binding
with `enabled: false`; it stays listed but no longer matches keystrokes.

## Views

Declarative views with `location: "settings"` are live:

- Settings → Plugins renders each enabled plugin's visible views inline via
  `DeclarativePluginHost` (list rows keyed by `columns`, cards keyed by
  bindings). The row's Open button scrolls to the rendered section.
- Opening a view from any other surface (for example the QuickSwitcher) goes
  through `openPluginView` on the plugin bridge, which resolves the declaring
  enabled plugin, rejects unknown views and non-`settings` locations with a
  visible error, and tracks the instance (`closePluginView` emits the
  view-closed event; bounds/visibility are host-rendered no-ops). The host
  renders the content with `DeclarativePluginViewSurface` — schema structure
  plus data, never plugin markup.

### View data bindings

`ViewDef.bindings` name the data keys a view displays. The renderer pulls them
through the plugin bridge's `getPluginViewData(pluginId, viewId, bindings)`,
which sends one `view.data` dispatch request
(`{"viewId": …, "bindings": […]}`) over the lemonssh-wasm-abi v1 channel. A
successful response result is a plain object mapping binding keys to display
values (list rows are objects keyed by the view's declared columns). The
plugin's declared non-secret settings values are the base layer of the merge
and the fallback when the dispatch fails, omits a binding, or the plugin does
not implement `view.data`; transport failures degrade to settings instead of
blanking the view. The SDK ships canonical encoders
(`buildPluginViewDataRequest` / `parsePluginViewDataResult`), and the shipped
`hello-lemonssh` example demonstrates the settings fallback with a card view
bound to its `com.lemonssh.hello.greeting` setting.

## Lifecycle rules

- Only plugins in the inventory's `enabled` state contribute UI
  (`getPluginContributions` filters on state, as does `UIContributions`).
- Disabling or uninstalling a plugin removes its settings UI, view, menu and
  keybinding entries immediately: the bridge emits a contributions-changed
  event, the settings page refreshes, the plugin's open view instances are
  closed with a `plugin-disabled` event, and the broker revokes the plugin's
  grants.
- Settings values are user-owned records keyed by plugin and setting ID; the
  host never returns password values to the renderer, only a
  "stored securely" indicator.
