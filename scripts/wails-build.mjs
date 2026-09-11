#!/usr/bin/env node
// Wails production build (Windows GUI host build). Stamps the package.json
// version into the binary so the window title and Health/Version report the
// release, not the skeleton placeholder.
import { spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import process from "node:process";

function run(command) {
  // npm is npm.cmd on Windows; shell keeps resolution and quoting uniform.
  const result = spawnSync(command, { stdio: "inherit", shell: true });
  if (result.status !== 0 || result.error) {
    throw new Error(`${command} failed with status ${result.status}`);
  }
}

const pkg = JSON.parse(await readFile("package.json", "utf8"));
const version = String(pkg.version ?? "0.0.0").replace(/"/g, "");

run("npm run build");
run("node scripts/wails-prepare-frontend.mjs");
run(
  `go build -trimpath "-ldflags=-s -w -X main.version=${version} -H windowsgui" -o bin/LemonSSH.exe ./cmd/netcatty`,
);
console.log(`[wails-build] built bin/LemonSSH.exe (version ${version})`);
