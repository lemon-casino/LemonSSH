import assert from "node:assert/strict";
import { test } from "node:test";
import { existsSync } from "node:fs";
import { mkdtemp, mkdir, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

// macOS /var is a system symlink; tests use the physical temp directory.
const tempRoot = await realpath(tmpdir());

import {
  appImageDesktop,
  appImageDesktopEntry,
  archMapping,
  attemptInstallerFormats,
  debControl,
  linuxHelperTargets,
  normalizeDesktopExec,
  nsisScript,
  readInstallerResources,
  rpmSpec,
  runAttempt,
  runInstaller,
  stageDarwinAppBundle,
  stageDebTree,
  stageDesktopResource,
  windowsInstallerFiles,
} from "./package-installer.mjs";

test("nsisScript assembles OutFile, File directive, shortcuts and uninstall", () => {
  const script = nsisScript({
    name: "LemonSSH",
    version: "1.2.3",
    exeFile: "C:\\build\\LemonSSH-1.2.3-windows-amd64.exe",
    outFile: "C:\\dist\\LemonSSH-1.2.3-windows-amd64-setup.exe",
  });
  assert.match(script, /Name "LemonSSH 1\.2\.3"/);
  assert.match(script, /OutFile "C:\\dist\\LemonSSH-1\.2\.3-windows-amd64-setup\.exe"/);
  assert.match(script, /InstallDir "\$PROGRAMFILES64\\LemonSSH"/);
  assert.match(script, /File "C:\\build\\LemonSSH-1\.2\.3-windows-amd64\.exe"/);
  assert.match(script, /CreateShortCut "\$DESKTOP\\LemonSSH\.lnk" "\$INSTDIR\\LemonSSH-1\.2\.3-windows-amd64\.exe"/);
  assert.match(script, /WriteUninstaller "\$INSTDIR\\Uninstall LemonSSH\.exe"/);
  assert.match(script, /Section "Uninstall"/);
});

test("nsisScript rejects missing fields", () => {
  assert.throws(
    () => nsisScript({ name: "LemonSSH", version: "1.2.3", exeFile: "", outFile: "out.exe" }),
    /required/,
  );
});

test("debControl writes Package, Version and Architecture", () => {
  const control = debControl({
    name: "LemonSSH",
    version: "1.2.3",
    arch: "amd64",
    maintainer: "LemonSSH Maintainers",
    description: "LemonSSH terminal",
  });
  assert.match(control, /^Package: lemonssh$/m);
  assert.match(control, /^Version: 1\.2\.3$/m);
  assert.match(control, /^Section: net$/m);
  assert.match(control, /^Priority: optional$/m);
  assert.match(control, /^Architecture: amd64$/m);
  assert.match(control, /^Depends:$/m);
  const arm = debControl({
    name: "lemonssh",
    version: "1.2.3",
    arch: "arm64",
    maintainer: "m",
    description: "d",
  });
  assert.match(arm, /^Architecture: arm64$/m);
});

test("debControl rejects unsupported architectures", () => {
  assert.throws(
    () => debControl({ name: "lemonssh", version: "1", arch: "386", maintainer: "m", description: "d" }),
    /unsupported goarch/,
  );
});

test("rpmSpec maps goarch to the rpm architecture and references the binary", () => {
  const spec = rpmSpec({
    name: "LemonSSH",
    version: "1.2.3",
    arch: "amd64",
    exeFile: "/in/LemonSSH-1.2.3-linux-amd64",
  });
  assert.match(spec, /^Name:\s+lemonssh$/m);
  assert.match(spec, /^Version:\s+1\.2\.3$/m);
  assert.match(spec, /^BuildArch:\s+x86_64$/m);
  assert.match(spec, /\/in\/LemonSSH-1\.2\.3-linux-amd64/);
  assert.match(spec, /%files/);
  const arm = rpmSpec({ name: "LemonSSH", version: "1.2.3", arch: "arm64", exeFile: "/in/x" });
  assert.match(arm, /^BuildArch:\s+aarch64$/m);
  assert.throws(
    () => rpmSpec({ name: "LemonSSH", version: "1.2.3", arch: "386", exeFile: "/in/x" }),
    /unsupported goarch/,
  );
});

test("appImageDesktop contains the required desktop entry keys", () => {
  const desktop = appImageDesktop({ name: "LemonSSH", exec: "LemonSSH", icon: "LemonSSH" });
  assert.match(desktop, /^\[Desktop Entry\]$/m);
  assert.match(desktop, /^Type=Application$/m);
  assert.match(desktop, /^Name=LemonSSH$/m);
  assert.match(desktop, /^Exec=LemonSSH$/m);
  assert.match(desktop, /^Icon=LemonSSH$/m);
  assert.match(desktop, /^MimeType=x-scheme-handler\/ssh;x-scheme-handler\/telnet;x-scheme-handler\/lemonssh;x-scheme-handler\/netcatty;$/m);
});

test("archMapping exposes nsis/deb/rpm strings and throws on unsupported", () => {
  assert.deepEqual(archMapping("amd64"), { nsis: "x64", deb: "amd64", rpm: "x86_64", appimage: "x86_64" });
  assert.deepEqual(archMapping("arm64"), { nsis: "arm64", deb: "arm64", rpm: "aarch64", appimage: "aarch64" });
  assert.throws(() => archMapping("386"), /unsupported goarch/);
  assert.throws(() => archMapping(undefined), /unsupported goarch/);
});

test("nsisScript registers the ssh/telnet/lemonssh/netcatty URL protocols under HKCU", () => {
  const script = nsisScript({
    name: "LemonSSH",
    version: "1.2.3",
    exeFile: "C:\\build\\LemonSSH-1.2.3-windows-amd64.exe",
    outFile: "C:\\dist\\LemonSSH-1.2.3-windows-amd64-setup.exe",
  });
  for (const scheme of ["ssh", "telnet", "lemonssh", "netcatty"]) {
    // Same registry surface internal/platform/deeplink/protocolreg.go writes
    // and DeepLinkService.GetOSProtocolStatus verifies: HKCU\Software\Classes,
    // the "URL Protocol" marker and shell\open\command = '"<exe>" "%1"'.
    assert.ok(script.includes(`WriteRegStr HKCU "Software\\Classes\\${scheme}" "" "URL:LemonSSH ${scheme} Protocol"`));
    assert.ok(script.includes(`WriteRegStr HKCU "Software\\Classes\\${scheme}" "URL Protocol" ""`));
    assert.ok(
      script.includes(
        `WriteRegStr HKCU "Software\\Classes\\${scheme}\\shell\\open\\command" "" '"$INSTDIR\\LemonSSH-1.2.3-windows-amd64.exe" "%1"'`,
      ),
    );
    assert.ok(script.includes(`DeleteRegKey HKCU "Software\\Classes\\${scheme}"`));
  }
  assert.match(script, /Section "Register URL protocols \(ssh:\/\/, telnet:\/\/, lemonssh:\/\/, netcatty:\/\/\)"/);
});

test("nsisScript installs helper files and removes them on uninstall", () => {
  const helperFiles = [
    "C:\\in\\mosh-client.exe",
    "C:\\in\\et.exe",
    "C:\\in\\LemonSSH-tool.exe",
    "C:\\in\\LemonSSH-mcp.exe",
  ];
  const script = nsisScript({
    name: "LemonSSH",
    version: "1.2.3",
    exeFile: "C:\\build\\LemonSSH-1.2.3-windows-amd64.exe",
    outFile: "C:\\dist\\LemonSSH-1.2.3-windows-amd64-setup.exe",
    helperFiles,
  });
  for (const helperFile of helperFiles) {
    // Split on both separators: path.basename keeps "C:\in\x.exe" whole on
    // POSIX hosts, while the generator must always emit the bare file name.
    const helperName = helperFile.split(/[\\/]/).pop();
    assert.ok(script.includes(`File "${helperFile}"`));
    assert.ok(script.includes(`Delete "$INSTDIR\\${helperName}"`));
  }
  // Without helpers only the main executable is File'd (backwards compatible).
  const plain = nsisScript({
    name: "LemonSSH",
    version: "1.2.3",
    exeFile: "C:\\build\\LemonSSH-1.2.3-windows-amd64.exe",
    outFile: "C:\\dist\\LemonSSH-1.2.3-windows-amd64-setup.exe",
  });
  assert.equal(plain.match(/  File "/g).length, 1);
});

test("windowsInstallerFiles keeps helper executables, DLLs and tools, drops manifests and licenses", () => {
  const resources = {
    helpers: [
      {
        kind: "mosh", path: "mosh-client.exe", destination: "mosh-client.exe",
        packagedFiles: [
          { path: "mosh-client.exe", destination: "mosh-client.exe" },
          { path: "mosh-client.exe.manifest.json", destination: "mosh-client.exe.manifest.json" },
        ],
      },
      {
        kind: "et", path: "et.exe", destination: "et.exe",
        packagedFiles: [
          { path: "et.exe", destination: "et.exe" },
          { path: "helper-loader.dll", destination: "helper-loader.dll" },
          { path: "licenses/et/EternalTerminal.txt", destination: "licenses/et/EternalTerminal.txt" },
        ],
      },
    ],
    tools: ["LemonSSH-tool.exe", "LemonSSH-mcp.exe"],
    protocolResources: [],
  };
  assert.deepEqual(windowsInstallerFiles({ resources }), [
    "LemonSSH-mcp.exe",
    "LemonSSH-tool.exe",
    "et.exe",
    "helper-loader.dll",
    "mosh-client.exe",
  ]);
  // Fallback when installer-resources.json is absent: discovered .exe helpers.
  assert.deepEqual(
    windowsInstallerFiles({ discoveredHelpers: [path.join("in", "mosh-client.exe"), path.join("in", "et")] }),
    ["mosh-client.exe"],
  );
});

test("linuxHelperTargets nests helpers by kind and flattens tools", () => {
  const resources = {
    helpers: [
      { kind: "mosh", path: "mosh-client", destination: "mosh-client" },
      { kind: "et", path: "et", destination: "et" },
    ],
    tools: ["LemonSSH-tool", "LemonSSH-mcp"],
    protocolResources: [],
  };
  assert.deepEqual(linuxHelperTargets({ resources }), [
    { source: "mosh-client", directory: "mosh", name: "mosh-client" },
    { source: "et", directory: "et", name: "et" },
    { source: "LemonSSH-tool", directory: ".", name: "LemonSSH-tool" },
    { source: "LemonSSH-mcp", directory: ".", name: "LemonSSH-mcp" },
  ]);
  // AppImage staging is isolated: flat like the portable zip layout.
  assert.deepEqual(
    linuxHelperTargets({ resources, kindLayout: false }).map((target) => target.directory),
    [".", ".", ".", "."],
  );
  // Fallback discovery when installer-resources.json is absent: pinned
  // helpers nest by kind, because a flat /usr/local/bin/mosh-client would
  // shadow the distro's /usr/bin/mosh-client on PATH (same decision as the
  // contract path); user-facing tools stay flat.
  assert.deepEqual(
    linuxHelperTargets({ discoveredHelpers: [path.join("in", "mosh-client"), path.join("in", "et"), path.join("in", "LemonSSH-tool")] }),
    [
      { source: "mosh-client", directory: "mosh", name: "mosh-client" },
      { source: "et", directory: "et", name: "et" },
      { source: "LemonSSH-tool", directory: ".", name: "LemonSSH-tool" },
    ],
  );
  // The isolated AppDir usr/bin stays flat even in the fallback.
  assert.deepEqual(
    linuxHelperTargets({ discoveredHelpers: [path.join("in", "mosh-client"), path.join("in", "et")], kindLayout: false }).map((target) => target.directory),
    [".", "."],
  );
});

test("readInstallerResources tolerates a missing manifest and parses a present one", async () => {
  const dir = await mkdtemp(path.join(tempRoot, "inst-res-"));
  assert.deepEqual(await readInstallerResources(dir), { helpers: [], tools: [], protocolResources: [] });
  await writeFile(
    path.join(dir, "installer-resources.json"),
    JSON.stringify({ helpers: [{ kind: "mosh" }], tools: ["LemonSSH-tool"], protocolResources: [{ source: "lemonssh.desktop", destination: "share/applications/lemonssh.desktop" }] }),
  );
  const resources = await readInstallerResources(dir);
  assert.equal(resources.helpers.length, 1);
  assert.deepEqual(resources.tools, ["LemonSSH-tool"]);
  assert.equal(resources.protocolResources[0].destination, "share/applications/lemonssh.desktop");
  await rm(dir, { recursive: true, force: true });
});

test("stageDebTree lays out control, binary, nested helpers, tools and the desktop file", async () => {
  const root = await mkdtemp(path.join(tempRoot, "inst-deb-"));
  const dist = path.join(root, "dist");
  const staging = path.join(root, "staging");
  await mkdir(dist);
  const binary = path.join(dist, "LemonSSH-1.2.3-linux-amd64");
  await writeFile(binary, "main");
  await writeFile(path.join(dist, "mosh-client"), "mosh");
  await writeFile(path.join(dist, "et"), "et");
  await writeFile(path.join(dist, "LemonSSH-tool"), "tool");
  await writeFile(path.join(dist, "LemonSSH-mcp"), "mcp");
  await writeFile(
    path.join(dist, "lemonssh.desktop"),
    "[Desktop Entry]\nExec=LemonSSH-1.2.3-linux-amd64 %u\nMimeType=x-scheme-handler/ssh;\n",
  );
  await writeFile(
    path.join(dist, "installer-resources.json"),
    JSON.stringify({
      helpers: [
        { kind: "mosh", path: "mosh-client", destination: "mosh-client" },
        { kind: "et", path: "et", destination: "et" },
      ],
      tools: ["LemonSSH-tool", "LemonSSH-mcp"],
      protocolResources: [{ source: "lemonssh.desktop", destination: "share/applications/lemonssh.desktop" }],
    }),
  );
  await stageDebTree({
    inDir: dist,
    staging,
    platform: { goos: "linux", goarch: "amd64", version: "1.2.3", binary },
    resources: await readInstallerResources(dist),
  });
  const control = await readFile(path.join(staging, "DEBIAN", "control"), "utf8");
  assert.match(control, /^Package: lemonssh$/m);
  assert.match(control, /^Architecture: amd64$/m);
  // The desktop resource lands at its declared prefix-relative destination,
  // and its versioned artifact Exec is normalized to the installed command
  // (/usr/local/bin/lemonssh): the staged tree must never ship an Exec that
  // points at a file the package does not install.
  const desktop = await readFile(path.join(staging, "share", "applications", "lemonssh.desktop"), "utf8");
  assert.match(desktop, /^Exec=lemonssh %u$/m);
  assert.match(desktop, /^MimeType=x-scheme-handler\/ssh;$/m);
  // Helpers resolve via exeDir/<kind>/<name> beside /usr/local/bin/lemonssh.
  for (const file of ["usr/local/bin/lemonssh", "usr/local/bin/mosh/mosh-client", "usr/local/bin/et/et", "usr/local/bin/LemonSSH-tool", "usr/local/bin/LemonSSH-mcp"]) {
    await readFile(path.join(staging, ...file.split("/")));
  }
  await rm(root, { recursive: true, force: true });
});

test("rpmSpec installs helpers, tools and the desktop file with %files entries", () => {
  const spec = rpmSpec({
    name: "LemonSSH",
    version: "1.2.3",
    arch: "amd64",
    exeFile: "/in/LemonSSH-1.2.3-linux-amd64",
    helperFiles: [
      { source: "/in/mosh-client", directory: "mosh", name: "mosh-client" },
      { source: "/in/et", directory: "et", name: "et" },
    ],
    toolFiles: [{ source: "/in/LemonSSH-tool", name: "LemonSSH-tool" }],
    desktopFile: { source: "/in/lemonssh.desktop", name: "lemonssh.desktop" },
  });
  assert.match(spec, /mkdir -p %{buildroot}%\{_bindir\}\/mosh/);
  assert.match(spec, /install -m 0755 "\/in\/mosh-client" %{buildroot}%\{_bindir\}\/mosh\/mosh-client/);
  assert.match(spec, /install -m 0755 "\/in\/et" %{buildroot}%\{_bindir\}\/et\/et/);
  assert.match(spec, /install -m 0755 "\/in\/LemonSSH-tool" %{buildroot}%\{_bindir\}\/LemonSSH-tool/);
  assert.match(spec, /install -m 0644 "\/in\/lemonssh\.desktop" %{buildroot}%\{_datadir\}\/applications\/lemonssh\.desktop/);
  assert.match(spec, /%attr\(0755,root,root\) %{_bindir}\/mosh\/mosh-client/);
  assert.match(spec, /%attr\(0644,root,root\) %{_datadir}\/applications\/lemonssh\.desktop/);
  // Backwards compatible shape when no extra resources are given.
  const plain = rpmSpec({ name: "LemonSSH", version: "1.2.3", arch: "amd64", exeFile: "/in/x" });
  assert.ok(!plain.includes("%{_datadir}"));
  assert.equal(plain.match(/%attr\(/g).length, 1);
});

test("normalizeDesktopExec retargets Exec at the installed command and preserves field codes", () => {
  const packaged = "[Desktop Entry]\nType=Application\nExec=LemonSSH-1.2.3-linux-amd64 %u\nMimeType=x-scheme-handler/ssh;\n";
  const normalized = normalizeDesktopExec(packaged, "lemonssh");
  // deb installs /usr/local/bin/lemonssh, rpm %{_bindir}/lemonssh — both are
  // the plain command name; the %u field code for ssh:// deep links survives.
  assert.match(normalized, /^Exec=lemonssh %u$/m);
  assert.match(normalized, /^Type=Application$/m);
  assert.match(normalized, /^MimeType=x-scheme-handler\/ssh;$/m);
  // Idempotent: normalizing an already-correct desktop file is a no-op.
  assert.equal(normalizeDesktopExec(normalized, "lemonssh"), normalized);
  // A desktop file without an Exec line comes back untouched.
  const bare = "[Desktop Entry]\nName=x\n";
  assert.equal(normalizeDesktopExec(bare, "lemonssh"), bare);
});

test("stageDesktopResource normalizes .desktop Exec and copies other resources verbatim", async () => {
  const root = await mkdtemp(path.join(tempRoot, "inst-desktop-"));
  const staged = path.join(root, "staged");
  await mkdir(staged, { recursive: true });
  // The shared helper behind the deb staging tree and the rpm spec staging:
  // a .desktop source is Exec-normalized, everything else copies byte-wise.
  const desktopSource = path.join(root, "lemonssh.desktop");
  await writeFile(desktopSource, "[Desktop Entry]\nExec=LemonSSH-1.2.3-linux-amd64 %u\n");
  const desktopStaged = path.join(staged, "lemonssh.desktop");
  await stageDesktopResource(desktopSource, desktopStaged, "lemonssh");
  assert.match(await readFile(desktopStaged, "utf8"), /^Exec=lemonssh %u$/m);
  const plistSource = path.join(root, "Info.plist");
  await writeFile(plistSource, "<plist/>");
  const plistStaged = path.join(staged, "Info.plist");
  await stageDesktopResource(plistSource, plistStaged, "lemonssh");
  assert.equal(await readFile(plistStaged, "utf8"), "<plist/>");
  await rm(root, { recursive: true, force: true });
});

test("appImageDesktopEntry adds the %u field code for ssh:// deep links", () => {
  const desktop = appImageDesktopEntry();
  assert.match(desktop, /^Exec=LemonSSH %u$/m);
  assert.match(desktop, /^Icon=LemonSSH$/m);
  assert.match(desktop, /^MimeType=x-scheme-handler\/ssh;x-scheme-handler\/telnet;x-scheme-handler\/lemonssh;x-scheme-handler\/netcatty;$/m);
  // The generic builder stays byte-exact in what it is given.
  assert.match(appImageDesktop({ name: "LemonSSH", exec: "LemonSSH", icon: "LemonSSH" }), /^Exec=LemonSSH$/m);
});

test("runAttempt wraps a staging crash as an honest ok:false entry", async () => {
  const entry = await runAttempt("deb", async () => {
    throw new Error("EACCES: copy boom");
  }, {});
  assert.deepEqual(entry, { format: "deb", ok: false, reason: "staging failed: EACCES: copy boom" });
  // Successful attempts pass through untouched.
  const ok = await runAttempt("zip", async () => ({ format: "zip", ok: true, file: "out.zip" }), {});
  assert.deepEqual(ok, { format: "zip", ok: true, file: "out.zip" });
});

test("attemptInstallerFormats keeps going after a throwing format and reports in order", async () => {
  const seen = [];
  const attempts = [
    ["zip", async () => {
      throw new Error("copy boom");
    }],
    ["deb", async (context) => ({ format: "deb", ok: true, file: context.inDir })],
  ];
  const formats = await attemptInstallerFormats({ inDir: "in" }, attempts, (entry) => seen.push(entry.format));
  assert.deepEqual(seen, ["zip", "deb"]);
  assert.equal(formats[0].format, "zip");
  assert.equal(formats[0].ok, false);
  assert.match(formats[0].reason, /staging failed: copy boom/);
  assert.deepEqual(formats[1], { format: "deb", ok: true, file: "in" });
});

test("runInstaller writes installers.json and removes the work dir after the run", async () => {
  const root = await mkdtemp(path.join(tempRoot, "inst-main-"));
  const dist = path.join(root, "dist");
  const out = path.join(root, "out");
  await mkdir(dist);
  await writeFile(path.join(dist, "LemonSSH-1.2.3-linux-amd64"), "main");
  await runInstaller(["--in", dist, "--out", out]);
  const summary = JSON.parse(await readFile(path.join(out, "installers.json"), "utf8"));
  assert.equal(summary.platforms.length, 1);
  assert.equal(summary.platforms[0].goos, "linux");
  assert.deepEqual(
    summary.platforms[0].formats.map((format) => format.format),
    ["zip", "nsis", "deb", "rpm", "appimage"],
  );
  for (const format of summary.platforms[0].formats) {
    assert.equal(typeof format.ok, "boolean");
    if (!format.ok) assert.equal(typeof format.reason, "string");
  }
  // Symmetric cleanup: the staging work dir is gone once the run finishes,
  // whether formats built, recorded honest skips, or failed staging.
  assert.equal(existsSync(path.join(out, ".installer-work")), false);
  await rm(root, { recursive: true, force: true });
});

test("stageDarwinAppBundle maps the flat staging into a real .app bundle", async () => {
  const root = await mkdtemp(path.join(tempRoot, "inst-mac-"));
  const dist = path.join(root, "dist");
  const stageDir = path.join(root, "bundle");
  await mkdir(dist, { recursive: true });
  await mkdir(path.join(dist, "licenses", "et"), { recursive: true });
  const plist =
    `<?xml version="1.0" encoding="UTF-8"?>\n<plist version="1.0"><dict>` +
    `<key>CFBundleExecutable</key><string>LemonSSH-1.2.3-darwin-amd64</string>` +
    `<key>CFBundleURLSchemes</key><array><string>ssh</string><string>telnet</string><string>lemonssh</string><string>netcatty</string></array>` +
    `</dict></plist>\n`;
  const binary = path.join(dist, "LemonSSH-1.2.3-darwin-amd64");
  await writeFile(binary, "main");
  await writeFile(path.join(dist, "Info.plist"), plist);
  await writeFile(path.join(dist, "mosh-client"), "mosh");
  await writeFile(path.join(dist, "mosh-client.manifest.json"), "{}");
  await writeFile(path.join(dist, "et"), "et");
  await writeFile(path.join(dist, "licenses", "et", "EternalTerminal.txt"), "license");
  await writeFile(path.join(dist, "LemonSSH-tool"), "tool");
  await writeFile(path.join(dist, "LemonSSH-mcp"), "mcp");
  await writeFile(
    path.join(dist, "installer-resources.json"),
    JSON.stringify({
      helpers: [
        {
          kind: "mosh", path: "mosh-client", destination: "Contents/MacOS/mosh-client",
          packagedFiles: [
            { path: "mosh-client", destination: "Contents/MacOS/mosh-client" },
            { path: "mosh-client.manifest.json", destination: "Contents/MacOS/mosh-client.manifest.json" },
          ],
        },
        {
          kind: "et", path: "et", destination: "Contents/MacOS/et",
          packagedFiles: [
            { path: "et", destination: "Contents/MacOS/et" },
            { path: "licenses/et/EternalTerminal.txt", destination: "Contents/Resources/licenses/et/EternalTerminal.txt" },
          ],
        },
      ],
      tools: ["LemonSSH-tool", "LemonSSH-mcp"],
      protocolResources: [{ source: "Info.plist", destination: "Contents/Info.plist" }],
    }),
  );
  const bundle = await stageDarwinAppBundle({
    inDir: dist,
    stageDir,
    platform: { goos: "darwin", goarch: "amd64", version: "1.2.3", binary },
    resources: await readInstallerResources(dist),
  });
  assert.equal(path.basename(bundle), "LemonSSH.app");
  // Info.plist sits inside Contents/, not flat next to the executable.
  for (const file of [
    "Contents/Info.plist",
    "Contents/PkgInfo",
    "Contents/MacOS/LemonSSH-1.2.3-darwin-amd64",
    "Contents/MacOS/mosh-client",
    "Contents/MacOS/mosh-client.manifest.json",
    "Contents/MacOS/et",
    "Contents/MacOS/LemonSSH-tool",
    "Contents/MacOS/LemonSSH-mcp",
    "Contents/Resources/licenses/et/EternalTerminal.txt",
  ]) {
    await readFile(path.join(bundle, ...file.split("/")));
  }
  await rm(root, { recursive: true, force: true });
});

test("stageDarwinAppBundle verifies scheme declarations, CFBundleExecutable and the plist presence", async () => {
  const root = await mkdtemp(path.join(tempRoot, "inst-mac-neg-"));
  const dist = path.join(root, "dist");
  await mkdir(dist, { recursive: true });
  const binary = path.join(dist, "LemonSSH-1.2.3-darwin-amd64");
  await writeFile(binary, "main");
  const platform = { goos: "darwin", goarch: "amd64", version: "1.2.3", binary };
  const stage = () => stageDarwinAppBundle({ inDir: dist, stageDir: path.join(root, "out"), platform, resources: { helpers: [], tools: [], protocolResources: [] } });
  // No Info.plist at all: honest staging failure, not a plist-less bundle.
  await assert.rejects(stage, /Info\.plist/);
  // A scheme missing from CFBundleURLSchemes is rejected.
  await writeFile(
    path.join(dist, "Info.plist"),
    `<plist><dict><key>CFBundleExecutable</key><string>LemonSSH-1.2.3-darwin-amd64</string>` +
    `<key>CFBundleURLSchemes</key><array><string>ssh</string><string>telnet</string></array></dict></plist>`,
  );
  await assert.rejects(stage, /lemonssh/); // first missing scheme from the required list
  // CFBundleExecutable must name the staged executable.
  await writeFile(
    path.join(dist, "Info.plist"),
    `<plist><dict><key>CFBundleExecutable</key><string>Other</string>` +
    `<key>CFBundleURLSchemes</key><array><string>ssh</string><string>telnet</string><string>lemonssh</string><string>netcatty</string></array></dict></plist>`,
  );
  await assert.rejects(stage, /CFBundleExecutable/);
  await rm(root, { recursive: true, force: true });
});
