#!/usr/bin/env node
"use strict";

// Dumps the Electron capability catalog to the fixture consumed by
// internal/capability's parity test (W05 semantic diff). Run after any
// intentional change to electron/capabilities/catalog and commit the
// regenerated JSON together with the Go catalog port.
//
//   node scripts/dump-capability-catalog.cjs

const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");
const { ALL_CAPABILITIES } = require("../electron/capabilities/catalog/index.cjs");

const outputPath = path.join(
  __dirname,
  "../testdata/ai/catalog/electron-catalog.json",
);

function gitCommit() {
  try {
    return execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim();
  } catch {
    return "unknown";
  }
}

const payload = {
  source: "electron/capabilities/catalog (ALL_CAPABILITIES, catalog order)",
  dumpedFromCommit: gitCommit(),
  capabilities: ALL_CAPABILITIES.map((capability) => ({
    id: capability.id,
    domain: capability.domain,
    status: capability.status,
    description: capability.description,
    policy: {
      write: capability.policy.write,
      sensitiveRead: capability.policy.sensitiveRead,
      longRunning: capability.policy.longRunning,
      requiresChatSession: capability.policy.requiresChatSession,
      bypassesObserverBlock: capability.policy.bypassesObserverBlock,
      bypassesApproval: capability.policy.bypassesApproval,
      bypassesChatCancel: capability.policy.bypassesChatCancel,
    },
    surfaces: Object.fromEntries(
      Object.entries(capability.surfaces || {}).map(([surface, binding]) => [
        surface,
        {
          rpcMethod: binding.rpcMethod ?? null,
          mcpTool: binding.mcpTool ?? null,
          toolName: binding.toolName ?? null,
          command: Array.isArray(binding.command) ? [...binding.command] : null,
          confirmInConfirmMode:
            binding.confirmInConfirmMode === undefined
              ? null
              : binding.confirmInConfirmMode,
        },
      ]),
    ),
    agentKinds: Array.isArray(capability.agentKinds) ? [...capability.agentKinds] : null,
  })),
};

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(payload, null, 2)}\n`, "utf8");
process.stdout.write(
  `Wrote ${payload.capabilities.length} capabilities to ${outputPath}\n`,
);
