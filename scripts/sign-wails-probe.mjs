#!/usr/bin/env node
// Honest Windows Authenticode probe for Wails qualification artifacts.
// Never claims a signed installer. Absent signtool / certificate is a
// documented pending state, not a pass.

import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

export function resolveSignTool(env = process.env, which = whichSignTool) {
  if (env.SIGNTOOL_PATH && existsSync(env.SIGNTOOL_PATH)) return env.SIGNTOOL_PATH;
  return which("signtool") ?? null;
}

function whichSignTool(name) {
  const result = spawnSync(process.platform === "win32" ? "where" : "which", [name], { encoding: "utf8" });
  if (result.status !== 0) return null;
  const first = result.stdout.split(/\r?\n/).map((line) => line.trim()).find(Boolean);
  return first || null;
}

export function signingStatus({ artifactPath, certThumbprint, signToolPath }) {
  if (!artifactPath) {
    return { signed: false, reason: "missing-artifact" };
  }
  if (!existsSync(artifactPath)) {
    return { signed: false, reason: "missing-artifact" };
  }
  if (!signToolPath) {
    return { signed: false, reason: "signtool-unavailable" };
  }
  if (!certThumbprint) {
    return { signed: false, reason: "certificate-unavailable" };
  }
  return { signed: false, reason: "not-invoked", artifactPath, signToolPath };
}

export function parseArgs(argv) {
  const args = { artifact: undefined, cert: undefined };
  for (let index = 0; index < argv.length; index++) {
    if (argv[index] === "--artifact") args.artifact = argv[++index];
    else if (argv[index] === "--cert") args.cert = argv[++index];
    else throw new Error(`unknown argument ${argv[index]}`);
  }
  return args;
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  const args = parseArgs(process.argv.slice(2));
  const status = signingStatus({
    artifactPath: args.artifact,
    certThumbprint: args.cert ?? process.env.WINDOWS_CERT_THUMBPRINT,
    signToolPath: resolveSignTool(),
  });
  console.log(JSON.stringify(status));
  process.exit(0);
}
