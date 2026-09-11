import assert from "node:assert/strict";
import { test } from "node:test";

import {
  appImageDesktop,
  archMapping,
  debControl,
  nsisScript,
  rpmSpec,
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
    maintainer: "Netcatty Maintainers",
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
});

test("archMapping exposes nsis/deb/rpm strings and throws on unsupported", () => {
  assert.deepEqual(archMapping("amd64"), { nsis: "x64", deb: "amd64", rpm: "x86_64", appimage: "x86_64" });
  assert.deepEqual(archMapping("arm64"), { nsis: "arm64", deb: "arm64", rpm: "aarch64", appimage: "aarch64" });
  assert.throws(() => archMapping("386"), /unsupported goarch/);
  assert.throws(() => archMapping(undefined), /unsupported goarch/);
});
