#!/usr/bin/env node

import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(scriptDir, "../..");
const outputDir = path.join(rootDir, "testdata/migration/electron");
const checkOnly = process.argv.includes("--check");

const { ALL_CAPABILITIES } = require("../../electron/capabilities/catalog/index.cjs");
const { listCliCapabilities } = require("../../electron/capabilities/adapters/cliAdapter.cjs");
const {
  AGENT_KINDS,
  listAgentToolSpecs,
  listMcpTools,
} = require("../../electron/capabilities/codegen/toolSurfaces.cjs");
const {
  TERMINAL_SUSTAINED_OUTPUT_WORKLOAD,
  makeTerminalSustainedOutputChunks,
} = require("../terminal-sustained-output-workload.cjs");

const STATIC_SOURCE_PATHS = Object.freeze([
  "electron/capabilities/constants.cjs",
  "electron/capabilities/registry.cjs",
  "electron/capabilities/schemas/toolInputs.cjs",
  "electron/plugins/constants.cjs",
  "electron/plugins/contractValidator.cjs",
  "electron/plugins/generated/plugin-contract.schema.json",
  "electron/plugins/jsonBoundary.cjs",
  "infrastructure/ai/harness/types.ts",
  "infrastructure/config/terminalFlowConstants.json",
  "infrastructure/config/storageKeys.ts",
  "packages/plugin-contract/schema/plugin-contract.schema.json",
  "domain/sessionRestore.ts",
  "scripts/migration/export-electron-contract-fixtures.mjs",
  "scripts/terminal-sustained-output-workload.cjs",
  "scripts/xterm-keyword-highlight-throughput.live.test.cjs",
]);

const SOURCE_DIRECTORIES = Object.freeze([
  "electron/capabilities/adapters",
  "electron/capabilities/catalog",
  "electron/capabilities/codegen",
  "types/global",
]);

const FIXTURE_NAMES = Object.freeze([
  "agent-tool-specs.json",
  "bridge-contract-index.json",
  "capability-catalog.json",
  "cli-capabilities.json",
  "mcp-tool-specs.json",
  "plugin-contract-baseline.json",
  "plugin-contract-fixtures.json",
  "runtime-owner-index.json",
  "storage-key-candidates.json",
  "runtime-contract-fixtures.json",
  "runtime-contract-fixtures.typecheck.ts",
  "terminal-flow-constants.json",
  "terminal-sustained-output-workload.json",
]);

function sha256(value) {
  return crypto.createHash("sha256").update(value).digest("hex");
}

function compareCodePoints(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function normalizeTextLineEndings(value) {
  return value.replace(/\r\n?/g, "\n");
}

function readRepoText(relativePath) {
  return normalizeTextLineEndings(fs.readFileSync(path.join(rootDir, relativePath), "utf8"));
}

function sortValue(value) {
  if (Array.isArray(value)) return value.map(sortValue);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value)
      .sort(([left], [right]) => compareCodePoints(left, right))
      .map(([key, entry]) => [key, sortValue(entry)]),
  );
}

function renderJson(value) {
  return `${JSON.stringify(sortValue(value), null, 2)}\n`;
}

function normalizeCapabilities() {
  return ALL_CAPABILITIES
    .map((capability) => ({
      id: capability.id,
      domain: capability.domain,
      status: capability.status,
      description: capability.description,
      policy: capability.policy,
      surfaces: capability.surfaces,
      ...(capability.agentKinds ? { agentKinds: capability.agentKinds } : {}),
    }))
    .sort((left, right) => compareCodePoints(left.id, right.id));
}

function sortByCapabilityId(values) {
  return [...values].sort((left, right) => (
    compareCodePoints(left.capabilityId, right.capabilityId)
      || compareCodePoints(String(left.toolName || left.mcpTool || ""),
        String(right.toolName || right.mcpTool || ""),
      )
  ));
}

function listFilesRecursively(relativeDirectory) {
  const absoluteDirectory = path.join(rootDir, relativeDirectory);
  const files = [];
  const visit = (absolutePath) => {
    for (const entry of fs.readdirSync(absolutePath, { withFileTypes: true })) {
      const child = path.join(absolutePath, entry.name);
      if (entry.isDirectory()) visit(child);
      else if (entry.isFile() && /\.(?:cjs|d\.ts)$/.test(entry.name)) {
        files.push(path.relative(rootDir, child).replaceAll("\\", "/"));
      }
    }
  };
  visit(absoluteDirectory);
  return files;
}

function listSourcePaths() {
  const bridgeSources = listFilesRecursively("types/global")
    .filter((relativePath) => relativePath.includes("netcatty-bridge-"));
  return [...new Set([
    ...STATIC_SOURCE_PATHS,
    ...SOURCE_DIRECTORIES.flatMap(listFilesRecursively),
    ...bridgeSources.flatMap(collectTypeScriptDeclarationClosure),
  ])].sort(compareCodePoints);
}

function resolveTypeScriptImport(importerPath, specifier) {
  if (!specifier.startsWith(".")) return null;
  const unresolved = path.resolve(rootDir, path.dirname(importerPath), specifier);
  const candidates = [
    unresolved,
    `${unresolved}.ts`,
    `${unresolved}.tsx`,
    `${unresolved}.d.ts`,
    path.join(unresolved, "index.ts"),
    path.join(unresolved, "index.tsx"),
    path.join(unresolved, "index.d.ts"),
  ];
  const match = candidates.find((candidate) => fs.existsSync(candidate) && fs.statSync(candidate).isFile());
  return match ? path.relative(rootDir, match).replaceAll("\\", "/") : null;
}

function collectTypeScriptDeclarationClosure(entryPath) {
  const visited = new Set();
  const visit = (relativePath) => {
    if (visited.has(relativePath)) return;
    visited.add(relativePath);
    const source = readRepoText(relativePath);
    const specifiers = [...source.matchAll(/(?:import|export)\s+(?:type\s+)?[^;]*?from\s+["']([^"']+)["']/g)]
      .map((match) => match[1]);
    for (const specifier of specifiers) {
      const resolved = resolveTypeScriptImport(relativePath, specifier);
      if (resolved) visit(resolved);
    }
  };
  visit(entryPath);
  return [...visited].sort(compareCodePoints);
}

function buildBridgeContractIndex() {
  return listFilesRecursively("types/global")
    .filter((relativePath) => relativePath.includes("netcatty-bridge-"))
    .map((relativePath) => {
      const source = readRepoText(relativePath);
      const methods = [...source.matchAll(/^\s{4}([A-Za-z_$][\w$]*)\??\s*\(/gm)]
        .map((match) => match[1])
        .sort(compareCodePoints);
      const declarations = collectTypeScriptDeclarationClosure(relativePath).map((sourcePath) => {
        const declaration = readRepoText(sourcePath);
        return {
          source: sourcePath,
          sourceSha256: sha256(declaration),
          declaration,
        };
      });
      return {
        source: relativePath,
        sourceSha256: sha256(source),
        declaration: source,
        declarations,
        methods,
      };
    })
    .sort((left, right) => compareCodePoints(left.source, right.source));
}

function buildStorageKeyCandidates() {
  const source = readRepoText("infrastructure/config/storageKeys.ts");
  const literalByName = new Map();
  const entries = [];
  const definitionPattern = /^export const (STORAGE_KEY_[A-Z0-9_]+)\s*=\s*([^;]+);/gm;
  for (const match of source.matchAll(definitionPattern)) {
    const name = match[1];
    const expression = match[2].replace(/\s+/g, " ").trim();
    const literal = expression.match(/^['"]([^'"]+)['"]$/)?.[1];
    const alias = expression.match(/^(STORAGE_KEY_[A-Z0-9_]+)$/)?.[1];
    const value = literal ?? (alias ? literalByName.get(alias) : undefined);
    if (value) literalByName.set(name, value);
    entries.push({ name, expression, ...(value ? { value } : {}), ...(alias ? { alias } : {}) });
  }
  return entries.sort((left, right) => compareCodePoints(left.name, right.name));
}

function buildPluginContractBaseline() {
  const schema = JSON.parse(readRepoText(
    "packages/plugin-contract/schema/plugin-contract.schema.json",
  ));
  const definitionNames = [
    "JsonValueLimits",
    "WireIntegerLimits",
    "RpcLimits",
    "StreamLimits",
    "TerminalInterceptorLimits",
    "ImporterLimits",
    "SyncLimits",
  ];
  const limits = Object.fromEntries(definitionNames.map((name) => {
    const value = schema.$defs?.[name]?.const;
    if (!value) throw new Error(`Plugin contract is missing ${name}.const`);
    return [name, value];
  }));
  return {
    schemaId: schema.$id,
    rootRef: schema.$ref,
    title: schema.title,
    limits,
  };
}

function buildPluginContractFixtures() {
  const manifest = {
    manifestVersion: 1,
    id: "com.netcatty.fixture",
    name: "migration-fixture",
    version: "0.1.0",
    publisher: "netcatty-fixture",
    engines: {
      netcatty: ">=0.0.0",
      api: ">=0.1.0-internal <0.2.0",
    },
    main: { browser: "dist/browser.js" },
  };
  return {
    validManifest: manifest,
    invalidManifestTraversal: {
      ...manifest,
      main: { browser: "../escape.js" },
    },
    rpcRequest: {
      jsonrpc: "2.0",
      id: "rpc-fixture-1",
      method: "plugin.storage.get",
      params: { key: "fixture-key" },
      deadlineMs: 30000,
      cancellationId: "cancel-fixture-1",
    },
    rpcSuccess: {
      jsonrpc: "2.0",
      id: "rpc-fixture-1",
      result: { value: "fixture-value" },
    },
    rejectedPackagePaths: [
      "../escape.js",
      "/absolute.js",
      "C:/drive.js",
      "dist\\backslash.js",
      "CON.txt",
      "dist/trailing. ",
    ],
  };
}

function buildRuntimeContractFixtures() {
  const base = {
    sessionId: "chat-session-fixture-1",
    chatSessionId: "chat-session-fixture-1",
    backend: "catty",
    timestamp: 1724361600000,
    turnId: "turn-fixture-1",
  };
  return {
    agentEvents: [
      { ...base, id: "event-1", type: "turn_start", backendLabel: "catty" },
      {
        ...base,
        id: "event-2",
        type: "tool_call",
        toolCallId: "tool-call-fixture-1",
        toolName: "session_environment",
        args: {},
      },
      {
        ...base,
        id: "event-3",
        type: "tool_result",
        toolCallId: "tool-call-fixture-1",
        toolName: "session_environment",
        result: "{\"ok\":true,\"platform\":\"fixture\"}",
      },
      {
        ...base,
        id: "event-4",
        type: "usage",
        promptTokens: 120,
        completionTokens: 30,
        totalTokens: 150,
        estimated: false,
      },
      { ...base, id: "event-5", type: "turn_end", reason: "completed" },
    ],
    sessionRestore: {
      version: 1,
      savedAt: 1724361600000,
      activeTabId: "session-fixture-1",
      tabOrder: ["session-fixture-1"],
      sessions: [{
        id: "session-fixture-1",
        hostId: "host-fixture-1",
        hostLabel: "Synthetic Host",
        hostname: "example.invalid",
        username: "fixture-user",
        status: "disconnected",
        protocol: "ssh",
        port: 22,
        shellType: "posix",
        restoreState: "restored-disconnected",
      }],
      workspaces: [],
    },
  };
}

function buildTerminalSustainedOutputWorkload() {
  const chunks = makeTerminalSustainedOutputChunks();
  const hash = crypto.createHash("sha256");
  const chunkSizeCounts = new Map();
  let totalBytes = 0;
  let totalChars = 0;

  for (const chunk of chunks) {
    const bytes = Buffer.byteLength(chunk, TERMINAL_SUSTAINED_OUTPUT_WORKLOAD.encoding);
    totalBytes += bytes;
    totalChars += chunk.length;
    hash.update(chunk, TERMINAL_SUSTAINED_OUTPUT_WORKLOAD.encoding);
    chunkSizeCounts.set(bytes, (chunkSizeCounts.get(bytes) ?? 0) + 1);
  }

  return {
    formatVersion: 1,
    workload: TERMINAL_SUSTAINED_OUTPUT_WORKLOAD,
    summary: {
      chunkCount: chunks.length,
      totalBytes,
      totalChars,
      chunkSizeDistribution: [...chunkSizeCounts.entries()]
        .sort(([left], [right]) => left - right)
        .map(([bytes, count]) => ({ bytes, count })),
      payloadSha256: hash.digest("hex"),
    },
  };
}

function buildRuntimeOwnerIndex() {
  return [
    {
      area: "app-shell",
      owners: ["electron/main.cjs", "electron/bridges/windowManager.cjs", "electron/preload.cjs"],
      tests: ["electron/appWindowLifecycle.test.cjs", "electron/preloadAiSdkAgent.test.cjs"],
    },
    {
      area: "terminal-data-plane",
      owners: [
        "electron/bridges/terminalOutputChannel.cjs",
        "electron/preload/terminalOutputPorts.cjs",
        "electron/bridges/terminalWorkerManager.cjs",
      ],
      tests: [
        "electron/preloadTerminalOutputPorts.test.cjs",
        "electron/terminalWorker/runtime.test.cjs",
        "scripts/terminal-output-stall.live.test.cjs",
      ],
    },
    {
      area: "terminal-runtime",
      owners: ["electron/bridges/terminalBridge.cjs", "electron/terminalWorker/process.cjs"],
      tests: ["electron/terminalWorker/process.test.cjs", "electron/terminalWorker/runtime.test.cjs"],
    },
    {
      area: "ssh",
      owners: ["electron/bridges/sshBridge.cjs", "electron/bridges/sshConnectionPool.cjs"],
      tests: [
        "electron/bridges/sshBridge.hostKeyChain.test.cjs",
        "electron/bridges/sshBridge.connectionReuse.test.cjs",
        "electron/bridges/sshConnectionPool.test.cjs",
      ],
    },
    {
      area: "sftp-transfer",
      owners: [
        "electron/bridges/sftpBridge.cjs",
        "electron/bridges/transferBridge.cjs",
        "application/state/sftp/transferRuntime.ts",
      ],
      tests: [
        "electron/bridges/sftpBridge.hostKeyVerification.test.cjs",
        "electron/bridges/sftpBridge.sessionBackedUpload.test.cjs",
        "electron/bridges/transferBridge.test.cjs",
        "application/state/sftp/transferRuntime.test.ts",
      ],
    },
    {
      area: "profile-persistence",
      owners: [
        "infrastructure/persistence/localStorageAdapter.ts",
        "application/state/useVaultState.ts",
        "application/state/useSessionState.ts",
      ],
      tests: [
        "infrastructure/persistence/localStorageAdapter.test.ts",
        "application/state/sessionRestoreStorage.test.ts",
        "application/state/useVaultState.snippetsDelete.test.ts",
      ],
    },
    {
      area: "capabilities-mcp-cli",
      owners: [
        "electron/capabilities/catalog/index.cjs",
        "electron/bridges/mcpServerBridge.cjs",
        "electron/cli/netcatty-tool-cli.cjs",
      ],
      tests: [
        "electron/capabilities/catalog/integrity.test.cjs",
        "electron/capabilities/codegen/toolSurfaces.test.cjs",
        "electron/capabilities/adapters/cliAdapter.test.cjs",
      ],
    },
    {
      area: "agent-runtime",
      owners: [
        "infrastructure/ai/harness/agentRuntime.ts",
        "infrastructure/ai/harness/contextManager.ts",
        "electron/bridges/aiBridge.cjs",
      ],
      tests: [
        "infrastructure/ai/harness/agentRuntime.test.ts",
        "infrastructure/ai/harness/contextManager.test.ts",
        "application/state/useAIChatStreaming.architecture.test.ts",
      ],
    },
    {
      area: "plugin-runtime",
      owners: [
        "packages/plugin-contract/schema/plugin-contract.schema.json",
        "electron/plugins/runtimeSupervisor.cjs",
        "electron/plugins/permissionEngine.cjs",
      ],
      tests: [
        "electron/plugins/runtimeSupervisor.test.cjs",
        "electron/plugins/permissionEngine.test.cjs",
        "electron/plugins/packageStore.test.cjs",
      ],
    },
    {
      area: "cloud-sync",
      owners: [
        "infrastructure/services/CloudSyncManager.ts",
        "domain/convergentSync",
        "electron/bridges/cloudSyncBridge.cjs",
      ],
      tests: [
        "application/convergentSyncMigration.test.ts",
        "infrastructure/services/EncryptionService.convergentSync.test.ts",
      ],
    },
  ];
}

function buildFixtures() {
  const runtimeContracts = buildRuntimeContractFixtures();
  const fixtures = {
    "agent-tool-specs.json": {
      sidebar: sortByCapabilityId(listAgentToolSpecs(AGENT_KINDS.SIDEBAR)),
      global: sortByCapabilityId(listAgentToolSpecs(AGENT_KINDS.GLOBAL)),
    },
    "bridge-contract-index.json": buildBridgeContractIndex(),
    "capability-catalog.json": normalizeCapabilities(),
    "cli-capabilities.json": listCliCapabilities()
      .sort((left, right) => compareCodePoints(left.command.join(" "), right.command.join(" "))),
    "mcp-tool-specs.json": sortByCapabilityId(listMcpTools()),
    "plugin-contract-baseline.json": buildPluginContractBaseline(),
    "plugin-contract-fixtures.json": buildPluginContractFixtures(),
    "runtime-owner-index.json": buildRuntimeOwnerIndex(),
    "storage-key-candidates.json": buildStorageKeyCandidates(),
    "runtime-contract-fixtures.json": runtimeContracts,
    "runtime-contract-fixtures.typecheck.ts": [
      "// Generated by scripts/migration/export-electron-contract-fixtures.mjs.",
      "// This file proves the synthetic events remain assignable to the canonical union.",
      "import type { AgentEvent } from '../../../infrastructure/ai/harness/types';",
      "",
      `export const agentEvents = ${JSON.stringify(runtimeContracts.agentEvents, null, 2)} satisfies AgentEvent[];`,
      "",
    ].join("\n"),
    "terminal-flow-constants.json": JSON.parse(readRepoText(
      "infrastructure/config/terminalFlowConstants.json",
    )),
    "terminal-sustained-output-workload.json": buildTerminalSustainedOutputWorkload(),
  };
  return Object.fromEntries(Object.entries(fixtures).map(([name, value]) => [
    name,
    typeof value === "string" ? value : renderJson(value),
  ]));
}

function buildManifest(fixtures) {
  const sourcePaths = listSourcePaths();
  return renderJson({
    formatVersion: 1,
    source: "electron-runtime-baseline",
    sourceFiles: Object.fromEntries(sourcePaths.map((relativePath) => [
      relativePath,
      sha256(readRepoText(relativePath)),
    ])),
    fixtures: Object.fromEntries(FIXTURE_NAMES.map((name) => [
      name,
      sha256(fixtures[name]),
    ])),
  });
}

function assertNoFixtureSecrets(outputs) {
  const forbidden = [
    "NETCATTY_FIXTURE_SECRET",
    "BEGIN OPENSSH PRIVATE KEY",
    "AKIAIOSFODNN7EXAMPLE",
  ];
  for (const [name, content] of Object.entries(outputs)) {
    for (const marker of forbidden) {
      if (content.includes(marker)) {
        throw new Error(`${name} contains forbidden secret marker ${marker}`);
      }
    }
  }
}

function checkFile(name, expected) {
  const filePath = path.join(outputDir, name);
  if (!fs.existsSync(filePath)) return `${name} is missing`;
  const actual = fs.readFileSync(filePath, "utf8");
  return actual === expected ? null : `${name} is stale`;
}

// Companion artifacts generated by sibling migration scripts live in the same
// directory; the exporter only owns the fixture set above.
const KNOWN_COMPANION_FILES = new Set(["data-inventory.json", "plugin-v1-data-retention.json"]);

function checkUnexpectedEntries(expectedNames) {
  if (!fs.existsSync(outputDir)) return [];
  const expected = new Set([...expectedNames, ...KNOWN_COMPANION_FILES]);
  return fs.readdirSync(outputDir, { withFileTypes: true })
    .filter((entry) => !expected.has(entry.name))
    .map((entry) => `${entry.name} is an unexpected generated ${entry.isDirectory() ? "directory" : "file"}`);
}

const fixtures = buildFixtures();
const outputs = { ...fixtures, "manifest.json": buildManifest(fixtures) };
assertNoFixtureSecrets(outputs);

if (checkOnly) {
  const failures = [
    ...Object.entries(outputs)
      .map(([name, content]) => checkFile(name, content))
      .filter(Boolean),
    ...checkUnexpectedEntries(Object.keys(outputs)),
  ];
  if (failures.length > 0) {
    process.stderr.write(
      `${failures.join("\n")}\nRegenerate expected files with node scripts/migration/export-electron-contract-fixtures.mjs\n`
      + "Remove unexpected entries explicitly; generation never deletes files.\n",
    );
    process.exitCode = 1;
  } else {
    process.stdout.write(`Electron migration fixtures are current (${Object.keys(outputs).length} files).\n`);
  }
} else {
  fs.mkdirSync(outputDir, { recursive: true });
  for (const [name, content] of Object.entries(outputs)) {
    fs.writeFileSync(path.join(outputDir, name), content, "utf8");
  }
  process.stdout.write(`Wrote ${Object.keys(outputs).length} Electron migration fixtures to ${outputDir}.\n`);
}
