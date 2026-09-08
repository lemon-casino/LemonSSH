import assert from "node:assert/strict";
import { AsyncLocalStorage } from "node:async_hooks";
import childProcess from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { checkMigrationDocs } from "./check-wails-migration-docs.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(scriptDir, "../..");
const fixtureLedgerContext = new AsyncLocalStorage();

function maxLedgerNumber(root) {
  const source = fs.readFileSync(
    path.join(root, "docs/migrations/wails-v3/migration-ledger.md"),
    "utf8",
  );
  return Math.max(
    0,
    ...[...source.matchAll(/^## WV3-L(\d{3}) -/gm)].map((match) => Number(match[1])),
  );
}

function ledgerId(offset = 1) {
  const baseLedgerNumber = fixtureLedgerContext.getStore();
  assert.notEqual(baseLedgerNumber, undefined, "ledgerId must be used inside withFixture");
  return `WV3-L${String(baseLedgerNumber + offset).padStart(3, "0")}`;
}

function createFixtureRoot() {
  const fixtureRoot = fs.mkdtempSync(path.join(os.tmpdir(), "netcatty-wails-docs-"));
  const target = path.join(fixtureRoot, "docs/migrations/wails-v3");
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.cpSync(path.join(rootDir, "docs/migrations/wails-v3"), target, { recursive: true });
  resetFixtureProgress(fixtureRoot);
  return fixtureRoot;
}

function mutate(root, relativePath, transform) {
  const filePath = path.join(root, "docs/migrations/wails-v3", relativePath);
  const source = fs.readFileSync(filePath, "utf8").replace(/\r\n?/g, "\n");
  fs.writeFileSync(filePath, transform(source), "utf8");
}

function withFixture(callback) {
  const root = createFixtureRoot();
  try {
    fixtureLedgerContext.run(maxLedgerNumber(root), () => callback(root));
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
}

// Mutation tests start from the current document schema but construct their own
// status history. This keeps them stable as real migration rows advance.
function resetFixtureProgress(root) {
  mutate(root, "capability-matrix.md", (source) => source.split("\n").map((line) => {
    if (!/^\| [A-Z]+-\d+(?:\.\d+)? \|/.test(line)) return line;
    const cells = line.slice(1, -1).split("|").map((cell) => cell.trim());
    cells[2] = cells[0] === "REL-03" ? "aggregate" : "required";
    cells[7] = "not-started";
    return `| ${cells.join(" | ")} |`;
  }).join("\n"));
  mutate(root, "migration-ledger.md", (source) => source
    .replace(/^- Status change: .*$/gm, "- Status change: `not-started -> not-started`")
    .replace(/^- Scope change: .*$/gm, "- Scope change: `none`")
    .replace(/^- Gate: .*$/gm, "- Gate: `none`")
    .replace(/^- Closure evidence: .*$/gm, "- Closure evidence: `none`")
    .replace(/^- Electron retirement:.*$/gm, "- Electron retirement: none: fixture baseline only"));
  fs.rmSync(path.join(root, "docs/migrations/wails-v3/gates"), { recursive: true, force: true });
}

function fixtureGit(root, ...args) {
  const result = childProcess.spawnSync("git", ["-C", root, ...args], { encoding: "utf8" });
  assert.equal(result.status, 0, `git ${args.join(" ")} failed: ${result.stderr}`);
  return result.stdout.trim();
}

function writeFixtureAiSource(root) {
  const filePath = path.join(root, "internal/capability/fixture.go");
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, "package capability\n", "utf8");
}

function commitFixture(root, message) {
  fixtureGit(root, "add", "-A");
  fixtureGit(root, "commit", "-m", message);
}

function initializeFixtureGit(root, { preGovernanceAi = false } = {}) {
  fixtureGit(root, "init");
  fixtureGit(root, "config", "user.email", "migration-checker@example.invalid");
  fixtureGit(root, "config", "user.name", "Migration Checker Test");
  fixtureGit(root, "config", "core.autocrlf", "false");
  fixtureGit(root, "config", "commit.gpgSign", "false");
  const hooksPath = path.join(root, ".git/disabled-hooks");
  fs.mkdirSync(hooksPath, { recursive: true });
  fixtureGit(root, "config", "core.hooksPath", hooksPath);
  if (preGovernanceAi) {
    writeFixtureAiSource(root);
    fixtureGit(root, "add", "internal/capability/fixture.go");
    fixtureGit(root, "commit", "-m", "pre-governance AI source");
  }
  commitFixture(root, "introduce migration governance");
}

function ledgerEntry({
  id = ledgerId(),
  capability = "FND-01",
  task = "P1-01",
  transition = "not-started -> implemented",
  scopeChange = "none",
  grade = "C",
  decisions = "none",
  gate = "none",
  closureEvidence = "none",
  retirement = "cutover-trigger: fixture cutover",
  verification = "fixture verification",
}) {
  return `
## ${id} - 2026-08-23 - ${task} fixture

- Capability rows: \`${capability}\`
- Plan task: \`${task}\`
- Status change: \`${transition}\`
- Scope change: \`${scopeChange}\`
- Goal: fixture goal
- Go canonical owner: fixture owner
- Frontend adapter: none
- Electron owner affected: fixture owner
- Preserved invariants: fixture invariant
- Data/schema impact: none
- Security impact: none
- Verification: ${verification}
- Platforms covered: fixture platform
- Evidence grade: \`${grade}\`
- Decision references: ${decisions}
- Gate: \`${gate}\`
- Closure evidence: \`${closureEvidence}\`
- Electron retirement: ${retirement}
- Documentation updated: matrix, plan, ledger
- Residual risks: fixture risk
- Next safe slice: fixture next
- Drift decision: \`needs-verification\`
`;
}

function setMatrixStatus(source, capability, status) {
  const pattern = new RegExp(`\\| ${capability.replace(".", "\\.")} \\|([^\\n]+)\\| [a-z-]+ \\|`);
  return source.replace(pattern, `| ${capability} |$1| ${status} |`);
}

function setMatrixScopeAndStatus(source, capability, scope, status) {
  const pattern = new RegExp(`(\\| ${capability.replace(".", "\\.")} \\|[^|]+\\|) [a-z-]+ (\\|[^\\n]+\\|) [a-z-]+ \\|`);
  return source.replace(pattern, `$1 ${scope} $2 ${status} |`);
}

function resolveReleaseMatrix(source) {
  return source.replaceAll("decision-required, release blocker", "required")
    .replaceAll("decision-required", "required");
}

function addAcceptedDecision(root, id, title, categories) {
  mutate(root, "decisions.md", (source) => source.replace(
    "## Required Future Decisions",
    `### ${id} - ${title}\n\n- Categories: ${categories.join(", ")}\n- Decision: fixture decision.\n\n## Required Future Decisions`,
  ));
}

function nonAiRequiredCapabilityIds(root) {
  const source = fs.readFileSync(
    path.join(root, "docs/migrations/wails-v3/capability-matrix.md"),
    "utf8",
  );
  const rows = source.split("\n").flatMap((line) => {
    if (!line.startsWith("|") || line.startsWith("| ID ") || line.startsWith("| ---")) return [];
    const cells = line.slice(1, -1).split("|").map((cell) => cell.trim());
    return [{ id: cells[0], scope: cells[2] }];
  });
  const prefixes = new Set(["FND", "TERM", "SSH", "SFTP", "NET", "SYS", "SYNC", "PLUG"]);
  return rows.filter((row) => {
    if (row.scope !== "required") return false;
    if (row.id === "REL-01" || row.id === "REL-02") return true;
    if (!prefixes.has(row.id.split("-", 1)[0])) return false;
    return !rows.some((candidate) => candidate.id.startsWith(`${row.id}.`));
  }).map((row) => row.id);
}

function addNonAiFixtureDecisions(root) {
  addAcceptedDecision(root, "WV3-901", "Fixture release targets", [
    "release-target:windows",
    "release-target:macos",
    "release-target:linux",
  ]);
  addAcceptedDecision(root, "WV3-902", "Fixture Agent dispositions", [
    "agent-runtime:cursor-bun",
    "agent-runtime:opencode-bun",
    "agent-disposition:copilot",
    "agent-disposition:codebuddy",
    "agent-disposition:cursor-cli",
  ]);
}

function gateEntry(id = ledgerId()) {
  return ledgerEntry({
    id,
    capability: "all non-AI required rows",
    task: "P6-05",
    transition: "not-started -> not-started",
    grade: "A",
    decisions: "`WV3-901`, `WV3-902`",
    gate: "NONAI-COMPLETE",
    retirement: "none: Wails disconnected; Electron frozen release carrier until approved cutover/rollback triggers",
    verification: "nonAiRows=verified; releaseTargets=WV3-901; agentDecisions=WV3-902; qualification=P6-02,P6-03,P6-04; authority=fixture gate authority",
  });
}

function gateMarker(id = ledgerId(), decisionIds = ["WV3-901", "WV3-902"]) {
  return {
    formatVersion: 1,
    gate: "NONAI-COMPLETE",
    ledgerId: id,
    decisionIds,
    recordedAt: "2026-08-25T00:00:00.000Z",
    evidenceCommit: "PENDING",
  };
}

function gateAuthority(id = ledgerId(), decisionIds = ["WV3-901", "WV3-902"]) {
  return { ledgerId: id, decisionIds };
}

function injectedAiBoundary(priorMarker, priorGate) {
  return {
    injectedBoundary: {
      changedPaths: ["internal/capability"],
      priorMarker,
      priorGate,
    },
  };
}

function writeGateMarker(root, id = ledgerId(), decisionIds = ["WV3-901", "WV3-902"]) {
  const markerPath = path.join(root, "docs/migrations/wails-v3/gates/nonai-complete.json");
  const marker = gateMarker(id, decisionIds);
  fs.mkdirSync(path.dirname(markerPath), { recursive: true });
  fs.writeFileSync(markerPath, `${JSON.stringify(marker, null, 2)}\n`, "utf8");
  return marker;
}

function setupNonAiFixture(root) {
  addNonAiFixtureDecisions(root);
  mutate(root, "capability-matrix.md", (source) => {
    const childRows = new Map([
      ["PLUG-02", "| PLUG-02.1 | Fixture WASM child | required | fixture | fixture | fixture | fixture | not-started |"],
      ["PLUG-03", "| PLUG-03.1 | Fixture native child | required | fixture | fixture | fixture | fixture | not-started |"],
    ]);
    for (const [parent, childRow] of childRows) {
      const pattern = new RegExp(`(^\\| ${parent} \\|.*$)`, "m");
      source = source.replace(pattern, `$1\n${childRow}`);
    }
    return source;
  });
  const capabilityIds = nonAiRequiredCapabilityIds(root);
  mutate(root, "release-target-matrix.md", resolveReleaseMatrix);
  return capabilityIds;
}

function appendCompleteNonAiGate(root, startOffset = 1) {
  const capabilityIds = setupNonAiFixture(root);
  const gateId = ledgerId(startOffset + 3);
  mutate(root, "capability-matrix.md", (source) => capabilityIds.reduce(
    (current, capability) => setMatrixStatus(current, capability, "verified"),
    source,
  ));
  const capabilities = capabilityIds.map((id) => `\`${id}\``).join(", ");
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(startOffset),
    capability: capabilities,
    task: "P6-05",
    transition: "not-started -> probe",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 1),
    capability: capabilities,
    task: "P6-05",
    transition: "probe -> implemented",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 2),
    capability: capabilities,
    task: "P6-05",
    transition: "implemented -> verified",
    grade: "A",
  })}${gateEntry(gateId)}`);
  const priorMarker = writeGateMarker(root, gateId);
  return {
    capabilityIds,
    nextOffset: startOffset + 4,
    priorGate: gateAuthority(gateId),
    priorMarker,
  };
}

function addReleaseLifecycleFixture(root, startOffset) {
  addAcceptedDecision(root, "WV3-903", "Fixture rollback window closure", ["rollback-window-closure"]);
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /(^\| AI-04 \|.*$)/m,
    "$1\n| AI-04.1 | Fixture external Agent child | required | fixture | fixture | fixture | fixture | not-started |",
  ));
  const aiIds = ["AI-01", "AI-02", "AI-03", "AI-04", "AI-04.1"];
  mutate(root, "capability-matrix.md", (source) => {
    for (const capability of [...aiIds, "REL-03.1", "REL-03.2"]) {
      source = setMatrixStatus(source, capability, "verified");
    }
    return source;
  });
  const aiCapabilities = aiIds.map((id) => `\`${id}\``).join(", ");
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(startOffset),
    capability: aiCapabilities,
    task: "P7-01",
    transition: "not-started -> probe",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 1),
    capability: aiCapabilities,
    task: "P7-04",
    transition: "probe -> implemented",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 2),
    capability: aiCapabilities,
    task: "P7-06",
    transition: "implemented -> verified",
    grade: "A",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 3),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "not-started -> probe",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 4),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "probe -> implemented",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 5),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "implemented -> verified",
    grade: "A",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 6),
    capability: "REL-03.1",
    task: "P8-02",
    transition: "verified -> verified",
    grade: "A",
    gate: "WAILS-CUTOVER",
    retirement: "none: cutover gate evidence only",
    verification: "rc=fixture signed RC; platforms=fixture targets; migration=fixture migration; rollback=fixture rollback; authority=fixture cutover authority",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 7),
    capability: "REL-03.2",
    task: "P8-03",
    transition: "not-started -> not-started",
    grade: "A",
    decisions: "`WV3-903`",
    gate: "ROLLBACK-CLOSED",
    closureEvidence: "thresholds=fixture thresholds; sample=fixture sample; blockers=none; authority=fixture closure authority",
    retirement: "none: rollback closure gate evidence only",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 8),
    capability: "REL-03.2",
    task: "P9-01",
    transition: "not-started -> probe",
    decisions: "`WV3-903`",
    retirement: "rollback-trigger: fixture rollback window closed",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 9),
    capability: "REL-03.2",
    task: "P9-01",
    transition: "probe -> implemented",
    decisions: "`WV3-903`",
    retirement: "deleted: fixture Electron runtime",
  })}${ledgerEntry({
    id: ledgerId(startOffset + 10),
    capability: "REL-03.2",
    task: "P9-02",
    transition: "implemented -> verified",
    grade: "A",
    decisions: "`WV3-903`",
    retirement: "deleted: fixture Electron runtime and repository paths",
  })}`);
}

test("current Wails migration documentation is consistent", () => {
  assert.deepEqual(checkMigrationDocs(rootDir), []);
});

test("checker rejects capability progress without ledger evidence", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /\| FND-01 \|([^\n]+)\| not-started \|/,
    "| FND-01 |$1| implemented |",
  ));
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("FND-01 matrix status implemented does not match latest ledger state not-started")
  )));
}));

test("checker requires the canonical Scope column and aggregate owner", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source
    .replace("| ID | Capability | Scope |", "| ID | Capability | Classification |")
    .replace("| FND-01 | Wails shell and typed frontend ports | required |", "| FND-01 | Wails shell and typed frontend ports | aggregate |"));
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes("canonical eight-column header including Scope")));
  assert.ok(errors.some((error) => error.includes("FND-01 cannot use aggregate scope")));
}));

test("checker rejects incomplete ledger records", () => withFixture((root) => {
  mutate(root, "migration-ledger.md", (source) => source.replace(
    /^- Electron retirement:.*\n/m,
    "",
  ));
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("WV3-L001 is missing ledger field: Electron retirement")
  )));
}));

test("checker rejects unknown decision references", () => withFixture((root) => {
  mutate(root, "migration-ledger.md", (source) => source.replace(
    "`WV3-001`, `WV3-003`, `WV3-006`",
    "`WV3-001`, `WV3-999`",
  ));
  assert.ok(checkMigrationDocs(root).some((error) => error.includes("unknown decision: WV3-999")));
}));

test("checker rejects broken local Markdown links", () => withFixture((root) => {
  mutate(root, "README.md", (source) => `${source}\n[broken](./missing-document.md)\n`);
  assert.ok(checkMigrationDocs(root).some((error) => error.includes("broken link")));
}));

test("checker requires composite capabilities to split before implementation", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /\| AI-04 \|([^\n]+)\| not-started \|/,
    "| AI-04 |$1| implemented |",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
    capability: "AI-04",
    task: "P7-05",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("AI-04 must be split into stable child rows")
  )));
}));

test("checker rejects the obsolete date-first ledger template shape", () => withFixture((root) => {
  mutate(root, "migration-ledger.md", (source) => `${source}\n## 2026-08-23 - P1-01 fixture\n\n- Goal: hidden\n`);
  assert.ok(checkMigrationDocs(root).some((error) => error.includes("Non-canonical ledger heading")));
}));

test("checker applies append-only transitions in order", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /\| FND-01 \|([^\n]+)\| not-started \|/,
    "| FND-01 |$1| implemented |",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
  })}${ledgerEntry({
    id: ledgerId(2),
    transition: "implemented -> blocked",
  })}`);
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes(
    "FND-01 matrix status implemented does not match latest ledger state blocked",
  )));
}));

test("checker rejects misleading evidence grades", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /\| FND-01 \|([^\n]+)\| not-started \|/,
    "| FND-01 |$1| verified |",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
  })}${ledgerEntry({
    id: ledgerId(2),
    transition: "implemented -> verified",
    grade: "not A",
  })}`);
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes(`${ledgerId(2)} has an invalid evidence grade`)));
  assert.ok(errors.some((error) => error.includes("FND-01 is verified without evidence grade A")));
}));

test("checker rejects verified advancement with pending retirement evidence", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /\| FND-01 \|([^\n]+)\| not-started \|/,
    "| FND-01 |$1| verified |",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
  })}${ledgerEntry({
    id: ledgerId(2),
    transition: "implemented -> verified",
    grade: "A",
    retirement: "pending",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes(`${ledgerId(2)} advanced to verified without a canonical Electron retirement trigger`)
  )));
}));

test("checker requires an approved decision for capability retirement", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /\| FND-01 \|([^\n]+)\| not-started \|/,
    "| FND-01 |$1| retired |",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
    transition: "not-started -> retired",
    grade: "A",
    decisions: "none",
    retirement: "removed: fixture capability",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes(`${ledgerId()} retired a capability without an approved decision`)
  )));
}));

test("checker rejects malformed and renamed root capability IDs", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace("| FND-02 |", "| FND_02 |"));
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes("Malformed capability ID: FND_02")));
  assert.ok(errors.some((error) => error.includes("Missing root capability ID: FND-02")));
}));

test("checker rejects unknown plan references outside the ledger", () => withFixture((root) => {
  mutate(root, "README.md", (source) => `${source}\nNext task: \`P9-99\`.\n`);
  assert.ok(checkMigrationDocs(root).some((error) => error.includes("unknown plan task: P9-99")));
}));

test("blocked composite capabilities do not require implementation child rows", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /\| AI-04 \|([^\n]+)\| not-started \|/,
    "| AI-04 |$1| blocked |",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
    capability: "AI-04",
    task: "P0-05",
    transition: "not-started -> blocked",
  })}`);
  const errors = checkMigrationDocs(root);
  assert.equal(errors.some((error) => error.includes("AI-04 must be split")), false);
}));

test("checker rejects orphan child capability rows", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(
    /\| REL-03 \|.*\| not-started \|/,
    (line) => `${line}\n| GHOST-99.1 | Orphan child | required | fixture | fixture | fixture | fixture | not-started |`,
  ));
  assert.ok(checkMigrationDocs(root).some((error) => error.includes("Orphan child capability ID: GHOST-99.1")));
}));

test("checker rejects absolute, UNC, and repository-escaping links without probing them", () => withFixture((root) => {
  mutate(root, "README.md", (source) => `${source}\n[drive](C:/Windows/win.ini)\n[unc](//server/share/file.md)\n[escape](../../../../outside.md)\n`);
  const errors = checkMigrationDocs(root);
  assert.equal(errors.filter((error) => error.includes("forbidden absolute or network link")).length, 2);
  assert.ok(errors.some((error) => error.includes("link outside the repository")));
}));

test("checker validates reference links and accepts inline link titles", () => withFixture((root) => {
  mutate(root, "README.md", (source) => `${source}\n[valid](./architecture.md "Architecture")\n[missing][no-such-definition]\n`);
  const errors = checkMigrationDocs(root);
  assert.equal(errors.some((error) => error.includes("architecture.md \"Architecture\"")), false);
  assert.ok(errors.some((error) => error.includes("undefined reference link: no-such-definition")));
}));

test("checker is read-only and its CLI exits non-zero on invalid documents", () => withFixture((root) => {
  mutate(root, "README.md", (source) => `${source}\n[broken](./missing.md)\n`);
  const before = fs.readFileSync(path.join(root, "docs/migrations/wails-v3/README.md"), "utf8");
  const result = childProcess.spawnSync(
    process.execPath,
    [path.join(scriptDir, "check-wails-migration-docs.mjs"), "--root", root],
    { encoding: "utf8" },
  );
  const after = fs.readFileSync(path.join(root, "docs/migrations/wails-v3/README.md"), "utf8");
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /broken link/);
  assert.equal(after, before);
}));

test("checker requires canonical REL-03 child rows", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => source.replace(/^\| REL-03\.2 \|.*\n/m, ""));
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes("Missing required child capability ID: REL-03.2")));
}));

test("checker rejects AI advancement before the non-AI gate", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "AI-01", "probe"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
    capability: "AI-01",
    task: "P7-01",
    transition: "not-started -> probe",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes(`${ledgerId()} advances AI-01 without a currently valid Gate NONAI-COMPLETE epoch`)
  )));
}));

test("checker rejects a premature non-AI completion gate", () => withFixture((root) => {
  mutate(root, "release-target-matrix.md", (source) => `${source}\n| RT-FIXTURE | Fixture unresolved target | fixture | fixture | fixture | fixture | fixture | decision-required |`);
  mutate(root, "migration-ledger.md", (source) => `${source}${gateEntry()}`);
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes(
    `${ledgerId()} Gate NONAI-COMPLETE is premature: FND-01 is not-started, expected verified`,
  )));
  assert.ok(errors.some((error) => error.includes("release target decisions remain unresolved")));
}));

test("checker rejects REL-03.2 advancement before the release gates", () => withFixture((root) => {
  addAcceptedDecision(root, "WV3-901", "Rollback window closure fixture", ["rollback-window-closure"]);
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "REL-03.2", "probe"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
    capability: "REL-03.2",
    task: "P9-01",
    transition: "not-started -> probe",
    decisions: "`WV3-901`",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes(`${ledgerId()} advances REL-03.2 before Gate WAILS-CUTOVER`)
  )));
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes(`${ledgerId()} advances REL-03.2 before Gate ROLLBACK-CLOSED`)
  )));
}));

test("checker requires a rollback-window closure decision for REL-03.2 progression", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => {
    source = setMatrixStatus(source, "REL-03.1", "verified");
    return setMatrixStatus(source, "REL-03.2", "probe");
  });
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "not-started -> probe",
  })}${ledgerEntry({
    id: ledgerId(2),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "probe -> implemented",
  })}${ledgerEntry({
    id: ledgerId(3),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "implemented -> verified",
    grade: "A",
  })}${ledgerEntry({
    id: ledgerId(4),
    capability: "REL-03.2",
    task: "P9-01",
    transition: "not-started -> probe",
    decisions: "`WV3-001`",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes(`${ledgerId(4)} advances REL-03.2 without decision category: rollback-window-closure`)
  )));
}));

test("checker rejects authoritative capability and task ID range shorthand", () => withFixture((root) => {
  mutate(root, "README.md", (source) => `${source}\nForbidden: \`AI-01..04\`, \`P7-01..P7-06\`, \`PLUG-01 - PLUG-03\`, \`P5-01-07\`.\n`);
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes("forbidden capability ID range shorthand: AI-01..04")));
  assert.ok(errors.some((error) => error.includes("forbidden plan task ID range shorthand: P7-01..P7-06")));
  assert.ok(errors.some((error) => error.includes("forbidden capability ID range shorthand: PLUG-01 - PLUG-03")));
  assert.ok(errors.some((error) => error.includes("forbidden plan task ID range shorthand: P5-01-07")));
}));

test("checker reports malformed encoded Markdown anchors without throwing", () => withFixture((root) => {
  mutate(root, "README.md", (source) => `${source}\n[bad anchor](./README.md#%ZZ)\n`);
  let errors;
  assert.doesNotThrow(() => {
    errors = checkMigrationDocs(root);
  });
  assert.ok(errors.some((error) => error.includes("invalid encoded Markdown anchor")));
}));

test("checker accepts a complete non-AI gate without mutating mixed capability states", () => withFixture((root) => {
  addNonAiFixtureDecisions(root);
  mutate(root, "capability-matrix.md", (source) => {
    const childRows = new Map([
      ["PLUG-02", "| PLUG-02.1 | Fixture WASM child | required | fixture | fixture | fixture | fixture | not-started |"],
      ["PLUG-03", "| PLUG-03.1 | Fixture native child | required | fixture | fixture | fixture | fixture | not-started |"],
    ]);
    for (const [parent, childRow] of childRows) {
      const pattern = new RegExp(`(^\\| ${parent} \\|.*$)`, "m");
      source = source.replace(pattern, `$1\n${childRow}`);
    }
    return source;
  });
  const capabilityIds = nonAiRequiredCapabilityIds(root);
  mutate(root, "capability-matrix.md", (source) => capabilityIds.reduce(
    (current, capability) => setMatrixStatus(current, capability, "verified"),
    source,
  ));
  mutate(root, "release-target-matrix.md", resolveReleaseMatrix);
  const capabilities = capabilityIds.map((id) => `\`${id}\``).join(", ");
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
    capability: capabilities,
    task: "P6-05",
    transition: "not-started -> probe",
  })}${ledgerEntry({
    id: ledgerId(2),
    capability: capabilities,
    task: "P6-05",
    transition: "probe -> implemented",
  })}${ledgerEntry({
    id: ledgerId(3),
    capability: capabilities,
    task: "P6-05",
    transition: "implemented -> verified",
    grade: "A",
  })}${gateEntry(ledgerId(4))}`);
  writeGateMarker(root, ledgerId(4));
  assert.deepEqual(checkMigrationDocs(root), []);
}));

test("checker requires removed scope to use retired status and latest approved removal evidence", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => setMatrixScopeAndStatus(
    source,
    "FND-01",
    "removed",
    "retired",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(),
    transition: "not-started -> retired",
    grade: "A",
    decisions: "none",
    retirement: "cutover-trigger: pending removal",
  })}`);
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes("retired a capability without an approved decision")));
  assert.ok(errors.some((error) => error.includes("retired a capability without removed: or deleted: evidence")));
  assert.ok(errors.some((error) => error.includes("FND-01 is removed without retained removed/deleted evidence")));
}));

test("checker rejects every production AI path before the non-AI gate", () => withFixture((root) => {
  const forbiddenPaths = [
    "internal/capability",
    "internal/agent",
    "cmd/netcatty-mcp",
    "cmd/netcatty-tool",
  ];
  for (const relativePath of forbiddenPaths) {
    fs.mkdirSync(path.join(root, relativePath), { recursive: true });
  }
  const errors = checkMigrationDocs(root);
  for (const relativePath of forbiddenPaths) {
    assert.ok(errors.some((error) => error.includes(
      `Production AI path exists before a valid Gate NONAI-COMPLETE: ${relativePath}`,
    )));
  }
}));

test("checker accepts a production AI path after a complete non-AI gate", () => withFixture((root) => {
  const { priorGate, priorMarker } = appendCompleteNonAiGate(root);
  fs.mkdirSync(path.join(root, "internal/capability"), { recursive: true });
  assert.deepEqual(checkMigrationDocs(root, injectedAiBoundary(priorMarker, priorGate)), []);
}));

test("checker rejects an incomplete injected parent gate marker", () => withFixture((root) => {
  const { priorGate, priorMarker } = appendCompleteNonAiGate(root);
  const incompletePriorMarker = { ...priorMarker };
  delete incompletePriorMarker.recordedAt;
  fs.mkdirSync(path.join(root, "internal/capability"), { recursive: true });
  assert.ok(checkMigrationDocs(root, injectedAiBoundary(incompletePriorMarker, priorGate)).some((error) => (
    error.includes("changes AI source without the latest valid NONAI gate marker in its parent commit")
  )));
}));

test("Git history rejects a parent marker that predeclares a future gate", () => withFixture((root) => {
  initializeFixtureGit(root);
  writeGateMarker(root, ledgerId(4));
  commitFixture(root, "predeclare future non-AI gate marker");
  appendCompleteNonAiGate(root);
  writeFixtureAiSource(root);
  commitFixture(root, "add gate authority and AI source");
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("changes AI source without the latest valid NONAI gate marker in its parent commit")
  )));
}));

test("Git history rejects a pre-gate AI commit hidden by a later gate commit", () => withFixture((root) => {
  initializeFixtureGit(root);
  writeFixtureAiSource(root);
  commitFixture(root, "add AI source before gate");
  appendCompleteNonAiGate(root);
  commitFixture(root, "record non-AI gate");
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("changes AI source without the latest valid NONAI gate marker in its parent commit")
  )));
}));

test("Git history accepts AI source committed after a valid parent gate", () => withFixture((root) => {
  initializeFixtureGit(root);
  appendCompleteNonAiGate(root);
  commitFixture(root, "record non-AI gate");
  writeFixtureAiSource(root);
  commitFixture(root, "add AI source after gate");
  assert.deepEqual(checkMigrationDocs(root), []);
}));

test("Git history rejects a gate that grandfathers pre-governance AI source", () => withFixture((root) => {
  initializeFixtureGit(root, { preGovernanceAi: true });
  appendCompleteNonAiGate(root);
  commitFixture(root, "record gate over existing AI source");
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("Production AI path exists before a valid Gate NONAI-COMPLETE: internal/capability")
  )));
}));

test("Git history allows pre-governance AI cleanup with the gate", () => withFixture((root) => {
  initializeFixtureGit(root, { preGovernanceAi: true });
  fs.rmSync(path.join(root, "internal/capability"), { recursive: true, force: true });
  appendCompleteNonAiGate(root);
  commitFixture(root, "remove pre-gate AI source and record gate");
  writeFixtureAiSource(root);
  commitFixture(root, "add AI source after clean gate");
  assert.deepEqual(checkMigrationDocs(root), []);
}));

test("Git root comparison accepts Windows path casing differences", {
  skip: process.platform !== "win32",
}, () => withFixture((root) => {
  initializeFixtureGit(root);
  const alternateCaseRoot = root.replace(/[A-Za-z]/g, (character) => (
    character === character.toLowerCase() ? character.toUpperCase() : character.toLowerCase()
  ));
  assert.deepEqual(checkMigrationDocs(alternateCaseRoot), []);
}));

test("checker rejects loose prose and self-asserted NONAI decision evidence", () => {
  withFixture((root) => {
    appendCompleteNonAiGate(root);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      "nonAiRows=verified; releaseTargets=WV3-901; agentDecisions=WV3-902; qualification=P6-02,P6-03,P6-04; authority=fixture gate authority",
      "required rows verified; release decisions and Agent decisions resolved",
    ));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("Gate NONAI-COMPLETE requires canonical structured Verification evidence")
    )));
  });

  withFixture((root) => {
    appendCompleteNonAiGate(root);
    mutate(root, "migration-ledger.md", (source) => source
      .replace("releaseTargets=WV3-901", "releaseTargets=WV3-009")
      .replace("`WV3-901`, `WV3-902`", "`WV3-009`, `WV3-901`, `WV3-902`"));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("Verification releaseTargets contains an irrelevant decision: WV3-009")
    )));
  });
});

test("checker requires every exact NONAI decision category", () => withFixture((root) => {
  appendCompleteNonAiGate(root);
  mutate(root, "decisions.md", (source) => source.replace(
    "agent-disposition:cursor-cli\n- Decision: fixture decision.",
    "agent-disposition:cursor-cli-renamed\n- Decision: fixture decision.",
  ));
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes(
    "Gate NONAI-COMPLETE is missing decision category: agent-disposition:cursor-cli",
  )));
  assert.ok(errors.some((error) => error.includes(
    "Verification agentDecisions is missing decision category: agent-disposition:cursor-cli",
  )));
}));

test("checker rejects accepted decisions without canonical Categories metadata", () => withFixture((root) => {
  addAcceptedDecision(root, "WV3-901", "Fixture malformed decision", ["fixture:category"]);
  mutate(root, "decisions.md", (source) => source.replace(
    "- Categories: fixture:category\n",
    "",
  ));
  assert.ok(checkMigrationDocs(root).some((error) => error.includes(
    "WV3-901 must contain exactly one Categories field",
  )));
}));

test("checker rejects gate-only metadata on the wrong records", () => {
  withFixture((root) => {
    mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
      closureEvidence: "thresholds=fixture; sample=fixture; blockers=none; authority=fixture",
    })}`);
    assert.ok(checkMigrationDocs(root).some((error) => error.includes(
      `${ledgerId()} must use Closure evidence: none unless Gate is ROLLBACK-CLOSED`,
    )));
  });

  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    mutate(root, "migration-ledger.md", (source) => source
      .replace(
        `## ${ledgerId(nextOffset - 1)} - 2026-08-23 - P6-05 fixture\n\n- Capability rows: \`all non-AI required rows\`\n- Plan task: \`P6-05\`\n- Status change: \`not-started -> not-started\`\n- Scope change: \`none\``,
        `## ${ledgerId(nextOffset - 1)} - 2026-08-23 - P6-05 fixture\n\n- Capability rows: \`all non-AI required rows\`\n- Plan task: \`P6-05\`\n- Status change: \`not-started -> not-started\`\n- Scope change: \`FND-01: required -> removed\``,
      )
      .replace(
        "- Electron retirement: none: Wails disconnected; Electron frozen release carrier until approved cutover/rollback triggers",
        "- Electron retirement: cutover-trigger: invalid gate retirement",
      ));
    const errors = checkMigrationDocs(root);
    assert.ok(errors.some((error) => error.includes("gate records must use Scope change: none")));
    assert.ok(errors.some((error) => error.includes("Gate NONAI-COMPLETE must use a none: retirement record")));
  });
});

test("checker rejects an irrelevant decision for a same-entry scope removal", () => withFixture((root) => {
  addAcceptedDecision(root, "WV3-901", "Irrelevant scope fixture", ["fixture:irrelevant"]);
  mutate(root, "capability-matrix.md", (source) => setMatrixScopeAndStatus(
    source,
    "FND-01",
    "removed",
    "retired",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: "FND-01",
    transition: "not-started -> retired",
    scopeChange: "FND-01: required -> removed",
    grade: "A",
    decisions: "`WV3-901`",
    retirement: "removed: fixture owner",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("Scope change requires decision category: scope-removal:FND-01")
  )));
}));

test("checker does not let a post-gate scope removal retroactively satisfy NONAI", () => withFixture((root) => {
  const capabilityIds = setupNonAiFixture(root);
  const verifiedIds = capabilityIds.filter((id) => id !== "FND-01");
  addAcceptedDecision(root, "WV3-903", "Fixture FND removal", ["scope-removal:FND-01"]);
  mutate(root, "capability-matrix.md", (source) => {
    source = verifiedIds.reduce(
      (current, capability) => setMatrixStatus(current, capability, "verified"),
      source,
    );
    return setMatrixScopeAndStatus(source, "FND-01", "removed", "retired");
  });
  const capabilities = verifiedIds.map((id) => `\`${id}\``).join(", ");
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: capabilities,
    task: "P6-05",
    transition: "not-started -> probe",
  })}${ledgerEntry({
    id: ledgerId(2),
    capability: capabilities,
    task: "P6-05",
    transition: "probe -> implemented",
  })}${ledgerEntry({
    id: ledgerId(3),
    capability: capabilities,
    task: "P6-05",
    transition: "implemented -> verified",
    grade: "A",
  })}${gateEntry(ledgerId(4))}${ledgerEntry({
    id: ledgerId(5),
    capability: "FND-01",
    transition: "not-started -> retired",
    scopeChange: "FND-01: required -> removed",
    grade: "A",
    decisions: "`WV3-903`",
    retirement: "removed: fixture owner",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes(`${ledgerId(4)} Gate NONAI-COMPLETE is premature: FND-01 is not-started, expected verified`)
  )));
}));

test("checker accepts an approved pre-gate scope removal", () => withFixture((root) => {
  const capabilityIds = setupNonAiFixture(root);
  const verifiedIds = capabilityIds.filter((id) => id !== "FND-01");
  addAcceptedDecision(root, "WV3-903", "Fixture FND removal", ["scope-removal:FND-01"]);
  mutate(root, "capability-matrix.md", (source) => {
    source = setMatrixScopeAndStatus(source, "FND-01", "removed", "retired");
    return verifiedIds.reduce(
      (current, capability) => setMatrixStatus(current, capability, "verified"),
      source,
    );
  });
  const capabilities = verifiedIds.map((id) => `\`${id}\``).join(", ");
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: "FND-01",
    transition: "not-started -> retired",
    scopeChange: "FND-01: required -> removed",
    grade: "A",
    decisions: "`WV3-903`",
    retirement: "removed: fixture owner",
  })}${ledgerEntry({
    id: ledgerId(2),
    capability: capabilities,
    task: "P6-05",
    transition: "not-started -> probe",
  })}${ledgerEntry({
    id: ledgerId(3),
    capability: capabilities,
    task: "P6-05",
    transition: "probe -> implemented",
  })}${ledgerEntry({
    id: ledgerId(4),
    capability: capabilities,
    task: "P6-05",
    transition: "implemented -> verified",
    grade: "A",
  })}${gateEntry(ledgerId(5))}`);
  writeGateMarker(root, ledgerId(5));
  assert.deepEqual(checkMigrationDocs(root), []);
}));

test("checker rejects a second chronological removal of the same scope", () => withFixture((root) => {
  addAcceptedDecision(root, "WV3-901", "Fixture FND removal", ["scope-removal:FND-01"]);
  mutate(root, "capability-matrix.md", (source) => setMatrixScopeAndStatus(
    source,
    "FND-01",
    "removed",
    "retired",
  ));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: "FND-01",
    transition: "not-started -> retired",
    scopeChange: "FND-01: required -> removed",
    grade: "A",
    decisions: "`WV3-901`",
    retirement: "removed: fixture owner",
  })}${ledgerEntry({
    id: ledgerId(2),
    capability: "FND-01",
    transition: "retired -> retired",
    scopeChange: "FND-01: required -> removed",
    grade: "A",
    decisions: "`WV3-901`",
    retirement: "removed: duplicate fixture removal",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => error.includes(
    `${ledgerId(2)} Scope change for FND-01 starts at required but current ledger scope is removed`,
  )));
}));

test("checker invalidates NONAI after regression and forbids AI state and production paths again", () => withFixture((root) => {
  const { nextOffset } = appendCompleteNonAiGate(root);
  mutate(root, "capability-matrix.md", (source) => {
    source = setMatrixStatus(source, "FND-01", "blocked");
    return setMatrixStatus(source, "AI-01", "probe");
  });
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(nextOffset),
    capability: "FND-01",
    transition: "verified -> blocked",
    retirement: "none: blocked evidence only",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 1),
    capability: "AI-01",
    task: "P7-01",
    transition: "not-started -> probe",
  })}`);
  fs.mkdirSync(path.join(root, "internal/capability"), { recursive: true });
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes(
    `${ledgerId(nextOffset + 1)} advances AI-01 without a currently valid Gate NONAI-COMPLETE epoch`,
  )));
  assert.ok(errors.some((error) => error.includes(
    "Production AI path exists before a valid Gate NONAI-COMPLETE: internal/capability",
  )));
}));

test("checker accepts recovery, a second NONAI gate, and later AI advancement", () => withFixture((root) => {
  const { nextOffset } = appendCompleteNonAiGate(root);
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "AI-01", "probe"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(nextOffset),
    capability: "FND-01",
    transition: "verified -> blocked",
    retirement: "none: blocked evidence only",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 1),
    capability: "FND-01",
    transition: "blocked -> implemented",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 2),
    capability: "FND-01",
    transition: "implemented -> verified",
    grade: "A",
  })}${gateEntry(ledgerId(nextOffset + 3))}${ledgerEntry({
    id: ledgerId(nextOffset + 4),
    capability: "AI-01",
    task: "P7-01",
    transition: "not-started -> probe",
  })}`);
  const gateId = ledgerId(nextOffset + 3);
  const priorMarker = writeGateMarker(root, gateId);
  fs.mkdirSync(path.join(root, "internal/capability"), { recursive: true });
  assert.deepEqual(checkMigrationDocs(root, injectedAiBoundary(priorMarker, gateAuthority(gateId))), []);
}));

test("checker keeps NONAI valid when a required row advances to migrated", () => withFixture((root) => {
  const { nextOffset, priorGate, priorMarker } = appendCompleteNonAiGate(root);
  mutate(root, "capability-matrix.md", (source) => {
    source = setMatrixStatus(source, "FND-01", "migrated");
    return setMatrixStatus(source, "AI-01", "probe");
  });
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(nextOffset),
    capability: "FND-01",
    transition: "verified -> migrated",
    grade: "A",
    retirement: "deleted: fixture Electron owner",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 1),
    capability: "AI-01",
    task: "P7-01",
    transition: "not-started -> probe",
  })}`);
  fs.mkdirSync(path.join(root, "internal/capability"), { recursive: true });
  assert.deepEqual(checkMigrationDocs(root, injectedAiBoundary(priorMarker, priorGate)), []);
}));

test("checker invalidates NONAI after an approved post-gate scope removal", () => withFixture((root) => {
  const { nextOffset } = appendCompleteNonAiGate(root);
  addAcceptedDecision(root, "WV3-903", "Fixture FND removal", ["scope-removal:FND-01"]);
  mutate(root, "capability-matrix.md", (source) => {
    source = setMatrixScopeAndStatus(source, "FND-01", "removed", "retired");
    return setMatrixStatus(source, "AI-01", "probe");
  });
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(nextOffset),
    capability: "FND-01",
    transition: "verified -> retired",
    scopeChange: "FND-01: required -> removed",
    grade: "A",
    decisions: "`WV3-903`",
    retirement: "removed: fixture owner",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 1),
    capability: "AI-01",
    task: "P7-01",
    transition: "not-started -> probe",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => error.includes(
    `${ledgerId(nextOffset + 1)} advances AI-01 without a currently valid Gate NONAI-COMPLETE epoch`,
  )));
}));

test("checker accepts a new NONAI gate after an approved post-gate scope removal", () => withFixture((root) => {
  const { nextOffset } = appendCompleteNonAiGate(root);
  addAcceptedDecision(root, "WV3-903", "Fixture FND removal", ["scope-removal:FND-01"]);
  mutate(root, "capability-matrix.md", (source) => {
    source = setMatrixScopeAndStatus(source, "FND-01", "removed", "retired");
    return setMatrixStatus(source, "AI-01", "probe");
  });
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(nextOffset),
    capability: "FND-01",
    transition: "verified -> retired",
    scopeChange: "FND-01: required -> removed",
    grade: "A",
    decisions: "`WV3-903`",
    retirement: "removed: fixture owner",
  })}${gateEntry(ledgerId(nextOffset + 1))}${ledgerEntry({
    id: ledgerId(nextOffset + 2),
    capability: "AI-01",
    task: "P7-01",
    transition: "not-started -> probe",
  })}`);
  const gateId = ledgerId(nextOffset + 1);
  const priorMarker = writeGateMarker(root, gateId);
  fs.mkdirSync(path.join(root, "internal/capability"), { recursive: true });
  assert.deepEqual(checkMigrationDocs(root, injectedAiBoundary(priorMarker, gateAuthority(gateId))), []);
}));

test("checker treats an invalid repeated NONAI record as the latest gate", () => withFixture((root) => {
  const { nextOffset } = appendCompleteNonAiGate(root);
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "AI-01", "probe"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(nextOffset),
    capability: "all non-AI required rows",
    task: "P6-05",
    transition: "not-started -> not-started",
    grade: "A",
    decisions: "`WV3-901`, `WV3-902`",
    gate: "NONAI-COMPLETE",
    retirement: "none: invalid fixture gate",
    verification: "nonAiRows=verified; releaseTargets=WV3-901; agentDecisions=WV3-902; qualification=P6-02,P6-03; authority=fixture gate authority",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 1),
    capability: "AI-01",
    task: "P7-01",
    transition: "not-started -> probe",
  })}`);
  const errors = checkMigrationDocs(root);
  assert.ok(errors.some((error) => error.includes(
    `${ledgerId(nextOffset + 1)} advances AI-01 without a currently valid Gate NONAI-COMPLETE epoch`,
  )));
}));

test("checker requires only P8-01 for the first REL-03.1 advancement", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "REL-03.1", "probe"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: "REL-03.1",
    task: "P8-02",
    transition: "not-started -> probe",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("first advances REL-03.1 without only plan task P8-01")
  )));
}));

test("checker cannot bypass the first REL-03.1 task through blocked state", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "REL-03.1", "probe"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: "REL-03.1",
    task: "P8-01",
    transition: "not-started -> blocked",
    retirement: "none: blocked evidence only",
  })}${ledgerEntry({
    id: ledgerId(2),
    capability: "REL-03.1",
    task: "P8-02",
    transition: "blocked -> probe",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("first advances REL-03.1 without only plan task P8-01")
  )));
}));

test("checker rejects wrong and unstructured WAILS-CUTOVER gate records", () => {
  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      `## ${ledgerId(nextOffset + 6)} - 2026-08-23 - P8-02 fixture\n\n- Capability rows: \`REL-03.1\`\n- Plan task: \`P8-02\``,
      `## ${ledgerId(nextOffset + 6)} - 2026-08-23 - P8-02 fixture\n\n- Capability rows: \`REL-03.1\`\n- Plan task: \`P8-01\``,
    ));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("Gate WAILS-CUTOVER must use only plan task P8-02")
    )));
  });

  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      "rc=fixture signed RC; platforms=fixture targets; migration=fixture migration; rollback=fixture rollback; authority=fixture cutover authority",
      "fixture signed RC passed all target checks",
    ));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("Gate WAILS-CUTOVER requires structured cutover Verification evidence")
    )));
  });
});

test("checker rejects wrong-task and unstructured ROLLBACK-CLOSED gate records", () => {
  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      `## ${ledgerId(nextOffset + 7)} - 2026-08-23 - P8-03 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P8-03\``,
      `## ${ledgerId(nextOffset + 7)} - 2026-08-23 - P8-03 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P9-01\``,
    ));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("Gate ROLLBACK-CLOSED must use only plan task P8-03")
    )));
  });

  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      "thresholds=fixture thresholds; sample=fixture sample; blockers=none; authority=fixture closure authority",
      "none",
    ));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("Gate ROLLBACK-CLOSED requires canonical Closure evidence")
    )));
  });
});

test("checker rejects ROLLBACK-CLOSED before WAILS-CUTOVER", () => withFixture((root) => {
  addAcceptedDecision(root, "WV3-901", "Fixture rollback closure", ["rollback-window-closure"]);
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: "REL-03.2",
    task: "P8-03",
    transition: "not-started -> not-started",
    grade: "A",
    decisions: "`WV3-901`",
    gate: "ROLLBACK-CLOSED",
    closureEvidence: "thresholds=fixture thresholds; sample=fixture sample; blockers=none; authority=fixture closure authority",
    retirement: "none: rollback closure gate evidence only",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => error.includes(
    "Gate ROLLBACK-CLOSED requires an earlier valid WAILS-CUTOVER gate",
  )));
}));

test("checker rejects WAILS-CUTOVER before required AI leaves are verified", () => withFixture((root) => {
  const { nextOffset } = appendCompleteNonAiGate(root);
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "REL-03.1", "verified"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    id: ledgerId(nextOffset),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "not-started -> probe",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 1),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "probe -> implemented",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 2),
    capability: "REL-03.1",
    task: "P8-01",
    transition: "implemented -> verified",
    grade: "A",
  })}${ledgerEntry({
    id: ledgerId(nextOffset + 3),
    capability: "REL-03.1",
    task: "P8-02",
    transition: "verified -> verified",
    grade: "A",
    gate: "WAILS-CUTOVER",
    retirement: "none: cutover gate evidence only",
    verification: "rc=fixture signed RC; platforms=fixture targets; migration=fixture migration; rollback=fixture rollback; authority=fixture cutover authority",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => error.includes(
    "Gate WAILS-CUTOVER requires AI-01 to be verified or removed by an approved decision",
  )));
}));

test("checker rejects release gate records that advance capability status", () => {
  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      `## ${ledgerId(nextOffset + 6)} - 2026-08-23 - P8-02 fixture\n\n- Capability rows: \`REL-03.1\`\n- Plan task: \`P8-02\`\n- Status change: \`verified -> verified\``,
      `## ${ledgerId(nextOffset + 6)} - 2026-08-23 - P8-02 fixture\n\n- Capability rows: \`REL-03.1\`\n- Plan task: \`P8-02\`\n- Status change: \`verified -> migrated\``,
    ));
    assert.ok(checkMigrationDocs(root).some((error) => error.includes(
      "Gate WAILS-CUTOVER must not advance capability status",
    )));
  });

  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      `## ${ledgerId(nextOffset + 7)} - 2026-08-23 - P8-03 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P8-03\`\n- Status change: \`not-started -> not-started\``,
      `## ${ledgerId(nextOffset + 7)} - 2026-08-23 - P8-03 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P8-03\`\n- Status change: \`not-started -> probe\``,
    ));
    assert.ok(checkMigrationDocs(root).some((error) => error.includes(
      "Gate ROLLBACK-CLOSED must not advance capability status",
    )));
  });
});

test("checker enforces P9-01 start and P9-02 verification tasks for REL-03.2", () => {
  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      `## ${ledgerId(nextOffset + 9)} - 2026-08-23 - P9-01 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P9-01\``,
      `## ${ledgerId(nextOffset + 9)} - 2026-08-23 - P9-01 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P9-02\``,
    ));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("advances REL-03.2 to implemented without only plan task P9-01")
    )));
  });

  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      `## ${ledgerId(nextOffset + 8)} - 2026-08-23 - P9-01 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P9-01\``,
      `## ${ledgerId(nextOffset + 8)} - 2026-08-23 - P9-01 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P9-02\``,
    ));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("first advances REL-03.2 without only plan task P9-01")
    )));
  });

  withFixture((root) => {
    const { nextOffset } = appendCompleteNonAiGate(root);
    addReleaseLifecycleFixture(root, nextOffset);
    mutate(root, "migration-ledger.md", (source) => source.replace(
      `## ${ledgerId(nextOffset + 10)} - 2026-08-23 - P9-02 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P9-02\``,
      `## ${ledgerId(nextOffset + 10)} - 2026-08-23 - P9-02 fixture\n\n- Capability rows: \`REL-03.2\`\n- Plan task: \`P9-01\``,
    ));
    assert.ok(checkMigrationDocs(root).some((error) => (
      error.includes("advances REL-03.2 to verified without only plan task P9-02")
    )));
  });
});

test("checker accepts the complete cutover and rollback lifecycle", () => withFixture((root) => {
  const { nextOffset } = appendCompleteNonAiGate(root);
  addReleaseLifecycleFixture(root, nextOffset);
  assert.deepEqual(checkMigrationDocs(root), []);
}));

test("checker requires the exact P9-01 rollback closure prerequisite", () => withFixture((root) => {
  mutate(root, "implementation-plan.md", (source) => source.replace(
    "前置：P8-03 已追加 grade A `Gate: ROLLBACK-CLOSED` record",
    "前置：P8-03 已追加 grade A rollback closure record",
  ));
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("P9-01 prerequisite is missing Gate: ROLLBACK-CLOSED")
  )));
}));

test("checker retains a canonical retirement trigger across a verified no-op", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "FND-01", "verified"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: "FND-01",
    transition: "not-started -> probe",
    retirement: "cutover-trigger: fixture owner cut over",
  })}${ledgerEntry({
    id: ledgerId(2),
    capability: "FND-01",
    transition: "probe -> implemented",
    retirement: "cutover-trigger: fixture owner cut over",
  })}${ledgerEntry({
    id: ledgerId(3),
    capability: "FND-01",
    transition: "implemented -> verified",
    grade: "A",
    retirement: "cutover-trigger: fixture owner cut over",
  })}${ledgerEntry({
    id: ledgerId(4),
    capability: "FND-01",
    transition: "verified -> verified",
    grade: "A",
    retirement: "none: docs-only verification refresh",
  })}`);
  assert.deepEqual(checkMigrationDocs(root), []);
}));

test("checker rejects a verified leaf with no retained canonical retirement trigger", () => withFixture((root) => {
  mutate(root, "capability-matrix.md", (source) => setMatrixStatus(source, "FND-01", "verified"));
  mutate(root, "migration-ledger.md", (source) => `${source}${ledgerEntry({
    capability: "FND-01",
    transition: "not-started -> probe",
    retirement: "pending",
  })}${ledgerEntry({
    id: ledgerId(2),
    capability: "FND-01",
    transition: "probe -> implemented",
    retirement: "none: docs-only assertion",
  })}${ledgerEntry({
    id: ledgerId(3),
    capability: "FND-01",
    transition: "implemented -> verified",
    grade: "A",
    retirement: "none: docs-only assertion",
  })}`);
  assert.ok(checkMigrationDocs(root).some((error) => (
    error.includes("FND-01 is verified without a retained canonical Electron retirement trigger")
  )));
}));
