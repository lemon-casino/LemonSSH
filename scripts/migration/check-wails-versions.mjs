#!/usr/bin/env node
// Wails version drift guard (W02): the Go module, the npm runtime and the
// wails3 generator pins must stay on one validated release combination.
// A mixed combination builds bindings with a different runtime than the
// binary links, which is exactly the alpha.63/beta.12 drift this guards.
import { readFileSync } from "node:fs";

const normalize = (version) => String(version).replace(/^v/, "");

const problems = [];
const goMod = readFileSync("go.mod", "utf8");
const wailsGoModule = goMod.match(/github\.com\/wailsapp\/wails\/v3 (v?\d+\.\d+\.\S+)/)?.[1];
if (!wailsGoModule) {
  problems.push("go.mod has no github.com/wailsapp/wails/v3 require");
}

const pkg = JSON.parse(readFileSync("package.json", "utf8"));
const wailsNpmRuntime =
  pkg.dependencies?.["@wailsio/runtime"] ?? pkg.devDependencies?.["@wailsio/runtime"];
if (!wailsNpmRuntime) {
  problems.push("package.json has no @wailsio/runtime dependency");
}

const generatorPins = new Set();
for (const value of Object.values({ ...pkg.dependencies, ...pkg.devDependencies, ...pkg.scripts })) {
  for (const match of String(value).matchAll(/wails3@(v?\d+\.\d+\.\S+)/g)) {
    generatorPins.add(match[1]);
  }
}
if (generatorPins.size === 0) {
  problems.push("no wails3@version pin found in package.json scripts");
}

if (wailsGoModule && wailsNpmRuntime && normalize(wailsGoModule) !== normalize(wailsNpmRuntime)) {
  problems.push(`go module ${wailsGoModule} != npm runtime ${wailsNpmRuntime}`);
}
for (const pin of generatorPins) {
  if (wailsGoModule && normalize(pin) !== normalize(wailsGoModule)) {
    problems.push(`generator pin ${pin} != go module ${wailsGoModule}`);
  }
}

if (problems.length > 0) {
  console.error(`Wails version drift detected:\n  ${problems.join("\n  ")}`);
  process.exit(1);
}
console.log(`Wails versions aligned: ${wailsGoModule} (go module, npm runtime, generator pins)`);
