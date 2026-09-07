#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultRoot = path.resolve(scriptDir, "../..");

const VALID_STATUSES = new Set([
  "not-started",
  "probe",
  "implemented",
  "verified",
  "migrated",
  "blocked",
  "retired",
]);
const VALID_EVIDENCE_GRADES = new Set(["A", "B", "C"]);
const VALID_SCOPES = new Set(["required", "removed", "aggregate"]);
const VALID_GATES = new Set(["none", "NONAI-COMPLETE", "WAILS-CUTOVER", "ROLLBACK-CLOSED"]);
const COMPOSITE_CAPABILITIES = new Set(["TERM-03", "AI-04", "PLUG-02", "PLUG-03"]);
const NON_AI_GATE_PREFIXES = new Set(["FND", "TERM", "SSH", "SFTP", "NET", "SYS", "SYNC", "PLUG"]);
const AI_CAPABILITY_PREFIX = "AI-";
const REQUIRED_CHILD_CAPABILITY_IDS = Object.freeze(["REL-03.1", "REL-03.2"]);
const REQUIRED_NONAI_DECISION_CATEGORIES = Object.freeze([
  "release-target:windows",
  "release-target:macos",
  "release-target:linux",
  "agent-runtime:cursor-bun",
  "agent-runtime:opencode-bun",
  "agent-disposition:copilot",
  "agent-disposition:codebuddy",
  "agent-disposition:cursor-cli",
]);
const REQUIRED_RELEASE_TARGET_CATEGORIES = Object.freeze(
  REQUIRED_NONAI_DECISION_CATEGORIES.filter((category) => category.startsWith("release-target:")),
);
const REQUIRED_AGENT_DECISION_CATEGORIES = Object.freeze(
  REQUIRED_NONAI_DECISION_CATEGORIES.filter((category) => !category.startsWith("release-target:")),
);
const NONAI_VERIFICATION_FIELDS = Object.freeze([
  "nonAiRows",
  "releaseTargets",
  "agentDecisions",
  "qualification",
  "authority",
]);
const CUTOVER_VERIFICATION_FIELDS = Object.freeze(["rc", "platforms", "migration", "rollback", "authority"]);
const ROLLBACK_CLOSURE_FIELDS = Object.freeze(["thresholds", "sample", "blockers", "authority"]);
const AI_SOURCE_PREFIXES = Object.freeze([
  "internal/capability",
  "internal/agent",
  "cmd/netcatty-mcp",
  "cmd/netcatty-tool",
]);
const MIGRATION_GOVERNANCE_PATH = "docs/migrations/wails-v3/README.md";
const NONAI_GATE_MARKER_PATH = "docs/migrations/wails-v3/gates/nonai-complete.json";
const NONAI_GATE_MARKER_FIELDS = Object.freeze([
  "decisionIds",
  "evidenceCommit",
  "formatVersion",
  "gate",
  "ledgerId",
  "recordedAt",
]);
const ADVANCEMENT_STATUSES = new Set(["probe", "implemented", "verified", "migrated"]);
const CAPABILITY_RANGE_PATTERN = /\b(?:FND|TERM|SSH|SFTP|NET|SYS|SYNC|AI|PLUG|REL)-\d+(?:\.\d+)?(?:\s*(?:\.{2,}|…|–|—)\s*|\s+to\s+|\s*-\s*)(?:(?:FND|TERM|SSH|SFTP|NET|SYS|SYNC|AI|PLUG|REL)-)?\d+(?:\.\d+)?\b/g;
const TASK_RANGE_PATTERN = /\bP\d+-\d+[A-Z]?(?:\s*(?:\.{2,}|…|–|—)\s*|\s+to\s+|\s*-\s*)(?:P\d+-)?\d+[A-Z]?\b/g;
const ALLOWED_STATUS_TRANSITIONS = Object.freeze({
  "not-started": new Set(["not-started", "probe", "blocked", "retired"]),
  probe: new Set(["probe", "implemented", "blocked", "not-started", "retired"]),
  implemented: new Set(["implemented", "verified", "blocked", "probe", "retired"]),
  verified: new Set(["verified", "migrated", "blocked", "implemented", "retired"]),
  migrated: new Set(["migrated", "blocked"]),
  blocked: new Set(["blocked", "not-started", "probe", "implemented", "retired"]),
  retired: new Set(["retired"]),
});
const EXPECTED_ROOT_CAPABILITY_IDS = Object.freeze([
  "AI-01", "AI-02", "AI-03", "AI-04",
  "FND-01", "FND-02", "FND-03", "FND-04",
  "NET-01",
  "PLUG-01", "PLUG-02", "PLUG-03",
  "REL-01", "REL-02", "REL-03",
  "SFTP-01", "SFTP-02",
  "SSH-01", "SSH-02",
  "SYNC-01", "SYNC-02",
  "SYS-01", "SYS-02", "SYS-03", "SYS-04",
  "TERM-01", "TERM-02", "TERM-03",
]);
const REQUIRED_LEDGER_FIELDS = Object.freeze([
  "Capability rows",
  "Plan task",
  "Status change",
  "Scope change",
  "Goal",
  "Go canonical owner",
  "Frontend adapter",
  "Electron owner affected",
  "Preserved invariants",
  "Data/schema impact",
  "Security impact",
  "Verification",
  "Platforms covered",
  "Evidence grade",
  "Decision references",
  "Gate",
  "Closure evidence",
  "Electron retirement",
  "Documentation updated",
  "Residual risks",
  "Next safe slice",
  "Drift decision",
]);

function compareCodePoints(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function readText(filePath) {
  return fs.readFileSync(filePath, "utf8").replaceAll("\r\n", "\n");
}

function listMarkdownFiles(directory) {
  const files = [];
  const visit = (current) => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const child = path.join(current, entry.name);
      if (entry.isDirectory()) visit(child);
      else if (entry.isFile() && entry.name.endsWith(".md")) files.push(child);
    }
  };
  visit(directory);
  return files.sort(compareCodePoints);
}

function collectUnique(values, label, errors) {
  const seen = new Set();
  for (const value of values) {
    if (seen.has(value)) errors.push(`Duplicate ${label}: ${value}`);
    seen.add(value);
  }
  return seen;
}

function parseMatrix(source, errors) {
  const rows = [];
  const lines = source.split("\n");
  const headerIndex = lines.findIndex((line) => /^\| ID \| Capability \|/.test(line));
  if (headerIndex < 0) {
    errors.push("Capability matrix table header is missing");
    return rows;
  }
  if (lines[headerIndex].trim() !== "| ID | Capability | Scope | Current owner | Target owner | Initial blocker | Required completion evidence | Status |") {
    errors.push("Capability matrix table must use the canonical eight-column header including Scope");
  }
  for (const line of lines.slice(headerIndex + 1)) {
    if (!line.startsWith("|")) break;
    const cells = line.slice(1, -1).split("|").map((cell) => cell.trim());
    if (cells.every((cell) => /^-+$/.test(cell))) continue;
    if (cells.length !== 8) {
      errors.push(`Malformed capability matrix row: ${line}`);
      continue;
    }
    const [id, capability, scope, currentOwner, targetOwner, blocker, evidence, status] = cells;
    if (!/^[A-Z]+-\d+(?:\.\d+)?$/.test(id)) {
      errors.push(`Malformed capability ID: ${id}`);
      continue;
    }
    if ([capability, currentOwner, targetOwner, blocker, evidence].some((value) => !value)) {
      errors.push(`Capability matrix row ${id} contains an empty required cell`);
    }
    rows.push({ id, scope, status });
  }
  collectUnique(rows.map((row) => row.id), "capability ID", errors);
  for (const row of rows) {
    if (!VALID_STATUSES.has(row.status)) errors.push(`Unknown status for ${row.id}: ${row.status}`);
    if (!VALID_SCOPES.has(row.scope)) errors.push(`Unknown scope for ${row.id}: ${row.scope}`);
    if (row.scope === "required" && row.status === "retired") {
      errors.push(`${row.id} is required and cannot be retired`);
    }
    if (row.scope === "removed" && row.status !== "retired") {
      errors.push(`${row.id} is removed but does not have retired status`);
    }
    if (row.scope === "aggregate" && row.status !== "not-started") {
      errors.push(`${row.id} is aggregate and must remain not-started`);
    }
    if (row.scope === "aggregate" && row.id !== "REL-03") {
      errors.push(`${row.id} cannot use aggregate scope; REL-03 is the only canonical aggregate row`);
    }
  }
  const ids = new Set(rows.map((row) => row.id));
  const expectedRootIds = new Set(EXPECTED_ROOT_CAPABILITY_IDS);
  for (const expectedId of EXPECTED_ROOT_CAPABILITY_IDS) {
    if (!ids.has(expectedId)) errors.push(`Missing root capability ID: ${expectedId}`);
  }
  for (const expectedId of REQUIRED_CHILD_CAPABILITY_IDS) {
    if (!ids.has(expectedId)) errors.push(`Missing required child capability ID: ${expectedId}`);
  }
  for (const row of rows) {
    if (!row.id.includes(".") && !expectedRootIds.has(row.id)) {
      errors.push(`Unexpected root capability ID: ${row.id}`);
      continue;
    }
    if (row.id.includes(".")) {
      const parentId = row.id.split(".", 1)[0];
      if (!expectedRootIds.has(parentId) || !ids.has(parentId)) {
        errors.push(`Orphan child capability ID: ${row.id}`);
      }
    }
  }
  const rel03 = rows.find((row) => row.id === "REL-03");
  if (rel03 && rel03.scope !== "aggregate") errors.push("REL-03 must have aggregate scope");
  if (rows.length === 0) errors.push("Capability matrix has no parseable rows");
  return rows;
}

function parseHeadingIds(source, pattern, label, errors) {
  const values = [...source.matchAll(pattern)].map((match) => match[1]);
  return collectUnique(values, label, errors);
}

function parseLedgerFields(section, entryId, errors) {
  const fields = new Map();
  let activeField = null;
  for (const line of section.split("\n")) {
    const field = line.match(/^- ([^:]+):\s*(.*)$/);
    if (field) {
      activeField = field[1];
      if (fields.has(activeField)) errors.push(`${entryId} has duplicate ledger field: ${activeField}`);
      fields.set(activeField, field[2].trim());
      continue;
    }
    if (activeField && /^\s{2,}\S/.test(line)) {
      fields.set(activeField, `${fields.get(activeField)} ${line.trim()}`.trim());
    } else if (line.trim() !== "") {
      activeField = null;
    }
  }
  return fields;
}

function parseLedger(source, errors) {
  const entriesIndex = source.indexOf("## Entries");
  if (entriesIndex < 0) {
    errors.push("Migration ledger is missing the Entries section");
    return [];
  }
  const entriesSource = source.slice(entriesIndex + "## Entries".length);
  const headings = [...entriesSource.matchAll(/^## (.+)$/gm)];
  const matches = [];
  for (const heading of headings) {
    const canonical = heading[1].match(/^(WV3-L\d{3}) - \d{4}-\d{2}-\d{2} - \S.*$/);
    if (!canonical) {
      errors.push(`Non-canonical ledger heading: ${heading[1]}`);
      continue;
    }
    matches.push({ ...heading, ledgerId: canonical[1] });
  }
  const entries = matches.map((match, index) => {
    const start = match.index + match[0].length;
    const end = matches[index + 1]?.index ?? source.length;
    return {
      id: match.ledgerId,
      fields: parseLedgerFields(entriesSource.slice(start, end), match.ledgerId, errors),
    };
  });
  collectUnique(entries.map((entry) => entry.id), "ledger ID", errors);
  entries.forEach((entry, index) => {
    const expected = `WV3-L${String(index + 1).padStart(3, "0")}`;
    if (entry.id !== expected) errors.push(`Ledger entry ${entry.id} is out of sequence; expected ${expected}`);
  });
  return entries;
}

function extractIds(value, pattern) {
  return [...value.matchAll(pattern)].map((match) => match[0]);
}

function parseStructuredFields(value, names) {
  const parts = value.split("; ");
  if (parts.length !== names.length) return null;
  const fields = new Map();
  const valid = parts.every((part, index) => {
    const prefix = `${names[index]}=`;
    if (!part.startsWith(prefix)) return false;
    const fieldValue = part.slice(prefix.length).trim();
    if (!fieldValue) return false;
    fields.set(names[index], fieldValue);
    return true;
  });
  return valid ? fields : null;
}

function parseCanonicalDecisionIds(value) {
  if (!/^WV3-\d{3}(?:,WV3-\d{3})*$/.test(value)) return null;
  const ids = value.split(",");
  return new Set(ids).size === ids.length ? ids : null;
}

function parseDecisions(source, errors) {
  const acceptedStart = source.indexOf("## Accepted Decisions");
  const futureStart = source.indexOf("## Required Future Decisions", acceptedStart + 1);
  if (acceptedStart < 0 || futureStart < 0) {
    errors.push("Decisions document is missing canonical accepted/future sections");
    return { accepted: new Map(), futureCategories: new Set() };
  }
  const acceptedSource = source.slice(acceptedStart, futureStart);
  const decisions = new Map();
  const headings = [...acceptedSource.matchAll(/^### (WV3-\d{3}) - (\S.*)$/gm)];
  for (const [index, match] of headings.entries()) {
    const id = match[1];
    if (decisions.has(id)) errors.push(`Duplicate decision ID: ${id}`);
    const sectionStart = match.index + match[0].length;
    const sectionEnd = headings[index + 1]?.index ?? acceptedSource.length;
    const section = acceptedSource.slice(sectionStart, sectionEnd);
    const categoryMatches = [...section.matchAll(/^- Categories:\s*(\S.*)$/gm)];
    if (categoryMatches.length !== 1) {
      errors.push(`${id} must contain exactly one Categories field`);
    }
    const categoryValue = categoryMatches[0]?.[1].trim() ?? "";
    const categories = categoryValue ? categoryValue.split(",").map((category) => category.trim()) : [];
    if (categories.length === 0 || categories.some((category) => !/^[A-Za-z0-9][A-Za-z0-9.-]*(?::[A-Za-z0-9][A-Za-z0-9.-]*)*$/.test(category))) {
      errors.push(`${id} has invalid canonical decision categories: ${categoryValue || "missing"}`);
    }
    if (categoryValue !== categories.join(", ")) errors.push(`${id} Categories must use canonical comma-space separators`);
    if (new Set(categories).size !== categories.length) errors.push(`${id} has duplicate decision categories`);
    decisions.set(id, {
      title: match[2].trim(),
      categories: new Set(categories),
    });
  }
  const futureSource = source.slice(futureStart);
  const futureCategories = new Set(
    [...futureSource.matchAll(/^- `([A-Za-z0-9][A-Za-z0-9.-]*(?::[A-Za-z0-9][A-Za-z0-9.-]*)*)`:/gm)]
      .map((match) => match[1]),
  );
  for (const category of REQUIRED_NONAI_DECISION_CATEGORIES) {
    if (!futureCategories.has(category)) errors.push(`Required future decision category is not declared: ${category}`);
  }
  return { accepted: decisions, futureCategories };
}

function decisionCategories(decisionIds, acceptedDecisions) {
  const categories = new Set();
  for (const decisionId of decisionIds) {
    for (const category of acceptedDecisions.get(decisionId)?.categories ?? []) categories.add(category);
  }
  return categories;
}

function validateNonAiVerification(
  entryId,
  verification,
  referencedDecisions,
  acceptedDecisions,
  errors,
) {
  const fields = parseStructuredFields(verification, NONAI_VERIFICATION_FIELDS);
  if (!fields) {
    errors.push(`${entryId} Gate NONAI-COMPLETE requires canonical structured Verification evidence`);
    return false;
  }

  let valid = true;
  if (fields.get("nonAiRows") !== "verified") {
    errors.push(`${entryId} Gate NONAI-COMPLETE Verification must use nonAiRows=verified`);
    valid = false;
  }
  if (fields.get("qualification") !== "P6-02,P6-03,P6-04") {
    errors.push(`${entryId} Gate NONAI-COMPLETE Verification must use qualification=P6-02,P6-03,P6-04`);
    valid = false;
  }

  const referencedDecisionSet = new Set(referencedDecisions);
  const validateDecisionField = (fieldName, requiredCategories) => {
    const decisionIds = parseCanonicalDecisionIds(fields.get(fieldName));
    if (!decisionIds) {
      errors.push(`${entryId} Gate NONAI-COMPLETE Verification has invalid ${fieldName} decision IDs`);
      valid = false;
      return;
    }
    for (const decisionId of decisionIds) {
      if (!acceptedDecisions.has(decisionId)) {
        errors.push(`${entryId} Gate NONAI-COMPLETE Verification ${fieldName} references unknown or unaccepted decision: ${decisionId}`);
        valid = false;
      }
      if (!referencedDecisionSet.has(decisionId)) {
        errors.push(`${entryId} Gate NONAI-COMPLETE Verification ${fieldName} decision is missing from Decision references: ${decisionId}`);
        valid = false;
      }
      const categories = acceptedDecisions.get(decisionId)?.categories ?? new Set();
      if (!requiredCategories.some((category) => categories.has(category))) {
        errors.push(`${entryId} Gate NONAI-COMPLETE Verification ${fieldName} contains an irrelevant decision: ${decisionId}`);
        valid = false;
      }
    }
    const categories = decisionCategories(decisionIds, acceptedDecisions);
    for (const category of requiredCategories) {
      if (!categories.has(category)) {
        errors.push(`${entryId} Gate NONAI-COMPLETE Verification ${fieldName} is missing decision category: ${category}`);
        valid = false;
      }
    }
  };

  validateDecisionField("releaseTargets", REQUIRED_RELEASE_TARGET_CATEGORIES);
  validateDecisionField("agentDecisions", REQUIRED_AGENT_DECISION_CATEGORIES);
  return valid;
}

function parseRetirement(value) {
  const match = value.match(/^(deleted|removed|cutover-trigger|rollback-trigger|none):\s*(\S.*)$/);
  return match ? { kind: match[1], detail: match[2].trim() } : null;
}

function validateLedgerEntry(entry, matrixIds, planTasks, acceptedDecisions, errors) {
  for (const field of REQUIRED_LEDGER_FIELDS) {
    if (!entry.fields.get(field)) errors.push(`${entry.id} is missing ledger field: ${field}`);
  }
  for (const [field, value] of entry.fields) {
    if (/\b(?:TBD|TODO|FIXME)\b|<[^>]+>/i.test(value)) {
      errors.push(`${entry.id} contains a placeholder in ${field}`);
    }
  }

  const capabilityValue = entry.fields.get("Capability rows") ?? "";
  const capabilityIds = extractIds(capabilityValue, /[A-Z]+-\d+(?:\.\d+)?/g);
  const normalizedCapabilityValue = capabilityValue.replaceAll("`", "").trim();
  const allRows = /^all rows(?:;.*)?$/i.test(normalizedCapabilityValue);
  const allNonAiRequiredRows = normalizedCapabilityValue === "all non-AI required rows";
  if (!allRows && !allNonAiRequiredRows && capabilityIds.length === 0) {
    errors.push(`${entry.id} does not reference a capability row`);
  }
  for (const capabilityId of capabilityIds) {
    if (!matrixIds.has(capabilityId)) errors.push(`${entry.id} references unknown capability: ${capabilityId}`);
  }

  const planTaskValue = entry.fields.get("Plan task") ?? "";
  const referencedTasks = extractIds(planTaskValue, /P\d+-\d+[A-Z]?/g);
  if (referencedTasks.length === 0) errors.push(`${entry.id} does not reference a plan task`);
  for (const task of referencedTasks) {
    if (!planTasks.has(task)) errors.push(`${entry.id} references unknown plan task: ${task}`);
  }

  const transitionValue = (entry.fields.get("Status change") ?? "").replaceAll("`", "").trim();
  const transition = transitionValue.match(
    /^(not-started|probe|implemented|verified|migrated|blocked|retired)\s*->\s*(not-started|probe|implemented|verified|migrated|blocked|retired)$/,
  );
  if (!transition) errors.push(`${entry.id} has an invalid status transition`);
  if (transition && !ALLOWED_STATUS_TRANSITIONS[transition[1]].has(transition[2])) {
    errors.push(`${entry.id} has a disallowed status transition: ${transition[1]} -> ${transition[2]}`);
  }

  const scopeChangeValue = (entry.fields.get("Scope change") ?? "").replaceAll("`", "").trim();
  const scopeRemoval = scopeChangeValue.match(/^([A-Z]+-\d+(?:\.\d+)?): required -> removed$/);
  if (scopeChangeValue !== "none" && !scopeRemoval) errors.push(`${entry.id} has an invalid Scope change value`);
  if (scopeRemoval && !matrixIds.has(scopeRemoval[1])) {
    errors.push(`${entry.id} Scope change references unknown capability: ${scopeRemoval[1]}`);
  }

  const evidenceGradeValue = (entry.fields.get("Evidence grade") ?? "").replaceAll("`", "").trim();
  const evidenceGrade = VALID_EVIDENCE_GRADES.has(evidenceGradeValue) ? evidenceGradeValue : undefined;
  if (!evidenceGrade || !VALID_EVIDENCE_GRADES.has(evidenceGrade)) {
    errors.push(`${entry.id} has an invalid evidence grade`);
  }

  const decisionValue = (entry.fields.get("Decision references") ?? "").replaceAll("`", "").trim();
  const referencedDecisions = extractIds(decisionValue, /WV3-\d{3}/g);
  const decisionIsNone = /^none$/i.test(decisionValue);
  const decisionRemainder = decisionValue
    .replace(/WV3-\d{3}/g, "")
    .replace(/[\s,]/g, "");
  const decisionSyntaxValid = decisionIsNone
    || (referencedDecisions.length > 0 && decisionRemainder === "");
  if (!decisionSyntaxValid) {
    errors.push(`${entry.id} must reference a decision or say none`);
  }
  let decisionReferencesValid = decisionSyntaxValid;
  for (const decisionId of referencedDecisions) {
    if (!acceptedDecisions.has(decisionId)) {
      errors.push(`${entry.id} references unknown or unaccepted decision: ${decisionId}`);
      decisionReferencesValid = false;
    }
  }

  const categories = decisionCategories(referencedDecisions, acceptedDecisions);
  const gateValue = (entry.fields.get("Gate") ?? "").replaceAll("`", "").trim();
  const gate = VALID_GATES.has(gateValue) ? gateValue : null;
  let nonAiVerificationValid = false;
  if (!gate) errors.push(`${entry.id} has an invalid Gate value`);
  if (allNonAiRequiredRows && gate !== "NONAI-COMPLETE") {
    errors.push(`${entry.id} uses the non-AI gate capability scope without Gate NONAI-COMPLETE`);
  }
  if (gate === "NONAI-COMPLETE") {
    if (!allNonAiRequiredRows) errors.push(`${entry.id} Gate NONAI-COMPLETE must use Capability rows: all non-AI required rows`);
    if (referencedTasks.length !== 1 || referencedTasks[0] !== "P6-05") errors.push(`${entry.id} Gate NONAI-COMPLETE must use only plan task P6-05`);
    if (transition?.[1] !== "not-started" || transition?.[2] !== "not-started") {
      errors.push(`${entry.id} Gate NONAI-COMPLETE must record not-started -> not-started without capability advancement`);
    }
    if (evidenceGrade !== "A") errors.push(`${entry.id} Gate NONAI-COMPLETE requires evidence grade A`);
    for (const category of REQUIRED_NONAI_DECISION_CATEGORIES) {
      if (!categories.has(category)) errors.push(`${entry.id} Gate NONAI-COMPLETE is missing decision category: ${category}`);
    }
    const verification = entry.fields.get("Verification") ?? "";
    nonAiVerificationValid = validateNonAiVerification(
      entry.id,
      verification,
      referencedDecisions,
      acceptedDecisions,
      errors,
    );
  }

  let cutoverVerificationValid = false;
  if (gate === "WAILS-CUTOVER") {
    if (referencedTasks.length !== 1 || referencedTasks[0] !== "P8-02") {
      errors.push(`${entry.id} Gate WAILS-CUTOVER must use only plan task P8-02`);
    }
    if (capabilityIds.length !== 1 || capabilityIds[0] !== "REL-03.1" || allRows) {
      errors.push(`${entry.id} Gate WAILS-CUTOVER must use Capability rows: REL-03.1`);
    }
    if (evidenceGrade !== "A") errors.push(`${entry.id} Gate WAILS-CUTOVER requires evidence grade A`);
    const verification = entry.fields.get("Verification") ?? "";
    cutoverVerificationValid = Boolean(parseStructuredFields(verification, CUTOVER_VERIFICATION_FIELDS));
    if (!cutoverVerificationValid) {
      errors.push(`${entry.id} Gate WAILS-CUTOVER requires structured cutover Verification evidence`);
    }
  }

  if (gate === "ROLLBACK-CLOSED") {
    if (capabilityIds.length !== 1 || capabilityIds[0] !== "REL-03.2" || allRows) {
      errors.push(`${entry.id} Gate ROLLBACK-CLOSED must use Capability rows: REL-03.2`);
    }
    if (referencedTasks.length !== 1 || referencedTasks[0] !== "P8-03") {
      errors.push(`${entry.id} Gate ROLLBACK-CLOSED must use only plan task P8-03`);
    }
    if (evidenceGrade !== "A") errors.push(`${entry.id} Gate ROLLBACK-CLOSED requires evidence grade A`);
    if (!categories.has("rollback-window-closure")) {
      errors.push(`${entry.id} Gate ROLLBACK-CLOSED requires decision category: rollback-window-closure`);
    }
  }

  const closureEvidence = (entry.fields.get("Closure evidence") ?? "").replaceAll("`", "").trim();
  let rollbackClosureValid = false;
  if (gate === "ROLLBACK-CLOSED") {
    rollbackClosureValid = Boolean(parseStructuredFields(closureEvidence, ROLLBACK_CLOSURE_FIELDS));
    if (!rollbackClosureValid) {
      errors.push(`${entry.id} Gate ROLLBACK-CLOSED requires canonical Closure evidence`);
    }
  } else if (closureEvidence !== "none") {
    errors.push(`${entry.id} must use Closure evidence: none unless Gate is ROLLBACK-CLOSED`);
  }
  if (gate && gate !== "none" && scopeChangeValue !== "none") {
    errors.push(`${entry.id} gate records must use Scope change: none`);
  }

  const retirementValue = (entry.fields.get("Electron retirement") ?? "").replaceAll("`", "").trim();
  const retirement = parseRetirement(retirementValue);
  if (!retirement) errors.push(`${entry.id} has a non-canonical Electron retirement value`);
  const isStatusChange = Boolean(transition && transition[1] !== transition[2]);
  if (transition && isStatusChange && ADVANCEMENT_STATUSES.has(transition[2]) && (!retirement || retirement.kind === "none")) {
    errors.push(`${entry.id} advanced to ${transition[2]} without a canonical Electron retirement trigger`);
  }
  if (transition?.[2] === "migrated" && evidenceGrade !== "A") {
    errors.push(`${entry.id} cannot migrate a capability below evidence grade A`);
  }
  if (transition?.[2] === "blocked" && retirement?.kind === "none" && retirementValue !== "none: blocked evidence only") {
    errors.push(`${entry.id} blocked without a trigger or the exact none: blocked evidence only marker`);
  }
  if (transition?.[2] === "retired" && referencedDecisions.length === 0) {
    errors.push(`${entry.id} retired a capability without an approved decision`);
  }
  if (transition?.[2] === "retired" && retirement && !["deleted", "removed"].includes(retirement.kind)) {
    errors.push(`${entry.id} retired a capability without removed: or deleted: evidence`);
  }
  if (scopeRemoval) {
    const capabilityId = scopeRemoval[1];
    if (capabilityIds.length !== 1 || capabilityIds[0] !== capabilityId || allRows || allNonAiRequiredRows) {
      errors.push(`${entry.id} Scope change must target only ${capabilityId}`);
    }
    if (transition?.[2] !== "retired") errors.push(`${entry.id} Scope change must transition ${capabilityId} to retired`);
    if (!categories.has(`scope-removal:${capabilityId}`)) {
      errors.push(`${entry.id} Scope change requires decision category: scope-removal:${capabilityId}`);
    }
    if (!retirement || !["deleted", "removed"].includes(retirement.kind)) {
      errors.push(`${entry.id} Scope change requires removed: or deleted: retirement evidence`);
    }
  } else if (transition?.[2] === "retired") {
    errors.push(`${entry.id} retired a capability without the same-entry Scope change`);
  }
  if (gate === "NONAI-COMPLETE" && retirement?.kind !== "none") {
    errors.push(`${entry.id} Gate NONAI-COMPLETE must use a none: retirement record`);
  }
  return {
    allRows,
    allNonAiRequiredRows,
    capabilityIds,
    transition,
    evidenceGrade,
    referencedTasks,
    referencedDecisions,
    decisionReferencesValid,
    categories,
    gate,
    nonAiVerificationValid,
    cutoverVerificationValid,
    rollbackClosureValid,
    scopeChangeValue,
    closureEvidence,
    scopeRemoval: scopeRemoval?.[1] ?? null,
    retirement,
  };
}

function isLeafCapability(row, rows) {
  return !rows.some((candidate) => candidate.id.startsWith(`${row.id}.`));
}

function isRequiredNonAiGateRow(row, rows) {
  if (row.id === "REL-01" || row.id === "REL-02") return true;
  if (!isLeafCapability(row, rows)) return false;
  const prefix = row.id.split("-", 1)[0];
  return NON_AI_GATE_PREFIXES.has(prefix);
}

function normalizeRepoPath(value) {
  return value.replaceAll("\\", "/").replace(/^\.\//, "").replace(/\/$/, "");
}

function isAiSourcePath(value) {
  const normalized = normalizeRepoPath(value);
  return AI_SOURCE_PREFIXES.some((prefix) => normalized === prefix || normalized.startsWith(`${prefix}/`));
}

function parseNullDelimitedPaths(value) {
  return value.split("\0").filter(Boolean).map(normalizeRepoPath);
}

function parseNameStatus(value) {
  const fields = value.split("\0").filter(Boolean);
  const changes = [];
  for (let index = 0; index < fields.length; index += 2) {
    changes.push({ status: fields[index], path: normalizeRepoPath(fields[index + 1]) });
  }
  return changes;
}

function runGit(rootDir, args) {
  const result = spawnSync("git", ["-C", rootDir, ...args], {
    encoding: "utf8",
    maxBuffer: 10 * 1024 * 1024,
  });
  return result.status === 0 ? result.stdout : null;
}

function canonicalRoot(value) {
  try {
    const realPath = fs.realpathSync.native(value);
    return process.platform === "win32" ? realPath.toLowerCase() : realPath;
  } catch {
    return null;
  }
}

function readGitMarker(rootDir, ref) {
  if (!ref) return null;
  const source = runGit(rootDir, ["show", `${ref}:${NONAI_GATE_MARKER_PATH}`]);
  if (source === null) return null;
  try {
    return JSON.parse(source);
  } catch {
    return null;
  }
}

function readGitText(rootDir, ref, repoPath) {
  return runGit(rootDir, ["show", `${ref}:${repoPath}`]);
}

function firstParent(rootDir, ref) {
  const line = runGit(rootDir, ["rev-list", "--parents", "-n", "1", ref]);
  return line?.trim().split(/\s+/)[1] ?? null;
}

function readGitDocumentGateAuthority(rootDir, ref) {
  if (!ref) return null;
  const matrixSource = readGitText(rootDir, ref, "docs/migrations/wails-v3/capability-matrix.md");
  const planSource = readGitText(rootDir, ref, "docs/migrations/wails-v3/implementation-plan.md");
  const decisionsSource = readGitText(rootDir, ref, "docs/migrations/wails-v3/decisions.md");
  const ledgerSource = readGitText(rootDir, ref, "docs/migrations/wails-v3/migration-ledger.md");
  const releaseMatrixSource = readGitText(rootDir, ref, "docs/migrations/wails-v3/release-target-matrix.md");
  if ([matrixSource, planSource, decisionsSource, ledgerSource, releaseMatrixSource].some((source) => source === null)) {
    return null;
  }

  const errors = [];
  const matrix = parseMatrix(matrixSource, errors);
  const matrixIds = new Set(matrix.map((row) => row.id));
  const planTasks = parseHeadingIds(planSource, /^### (P\d+-\d+[A-Z]?)\b/gm, "plan task", errors);
  const { accepted: acceptedDecisions } = parseDecisions(decisionsSource, errors);
  const ledgerEntries = parseLedger(ledgerSource, errors);
  const entriesWithRefs = ledgerEntries.map((entry) => ({
    entry,
    ...validateLedgerEntry(entry, matrixIds, planTasks, acceptedDecisions, errors),
  }));
  const progress = validateMatrixProgress(matrix, entriesWithRefs, releaseMatrixSource, errors);
  return errors.length === 0 && progress.nonAiGateValid ? progress.latestValidNonAiGate : null;
}

function gitTreeHasAiSource(rootDir, ref) {
  const source = runGit(rootDir, ["ls-tree", "-r", "--name-only", "-z", ref, "--", ...AI_SOURCE_PREFIXES]);
  return source === null || parseNullDelimitedPaths(source).some(isAiSourcePath);
}

function sameGateAuthority(left, right) {
  return left?.ledgerId === right?.ledgerId
    && JSON.stringify(left?.decisionIds) === JSON.stringify(right?.decisionIds);
}

function readGitGateAuthority(rootDir, ref, cache) {
  if (!ref) return null;
  if (cache.has(ref)) return cache.get(ref);
  const authority = readGitDocumentGateAuthority(rootDir, ref);
  if (!authority) {
    cache.set(ref, null);
    return null;
  }

  const ledgerCommits = runGit(rootDir, [
    "log", "--format=%H", "--reverse", ref, "--", "docs/migrations/wails-v3/migration-ledger.md",
  ])?.trim().split(/\r?\n/).filter(Boolean);
  if (!ledgerCommits) {
    cache.set(ref, null);
    return null;
  }
  const establishedAt = ledgerCommits.find((commit) => (
    sameGateAuthority(readGitDocumentGateAuthority(rootDir, commit), authority)
  ));
  const result = establishedAt && !gitTreeHasAiSource(rootDir, establishedAt) ? authority : null;
  cache.set(ref, result);
  return result;
}

function readCommitChanges(rootDir, commit, parent) {
  const args = parent
    ? ["diff", "--name-status", "-z", "--no-renames", parent, commit]
    : ["diff-tree", "--root", "--no-commit-id", "--name-status", "-z", "-r", "--no-renames", commit];
  const source = runGit(rootDir, args);
  return source === null ? null : parseNameStatus(source);
}

function collectGitChangeBoundaries(rootDir) {
  const topLevel = runGit(rootDir, ["rev-parse", "--show-toplevel"]);
  if (topLevel === null || canonicalRoot(topLevel.trim()) !== canonicalRoot(rootDir)) return null;

  const head = runGit(rootDir, ["rev-parse", "--verify", "HEAD"]);
  const tracked = head === null ? "" : runGit(rootDir, ["diff", "--name-status", "-z", "--no-renames", "HEAD"]);
  const untracked = runGit(rootDir, ["ls-files", "--others", "--exclude-standard", "-z"]);
  if (tracked === null || untracked === null) return null;

  const workingChanges = [
    ...parseNameStatus(tracked),
    ...parseNullDelimitedPaths(untracked).map((repoPath) => ({ status: "A", path: repoPath })),
  ];
  if (head === null) {
    return [{ label: "working tree", changes: workingChanges, priorMarker: null, priorGate: null }];
  }

  // The migration README introduces this policy. Scan every reachable commit
  // from that commit onward, without relying on a remote branch or CI-only ref.
  const introductions = runGit(rootDir, [
    "log", "--diff-filter=A", "--format=%H", "--reverse", "HEAD", "--", MIGRATION_GOVERNANCE_PATH,
  ]);
  const introduction = introductions?.trim().split(/\r?\n/).filter(Boolean)[0] ?? null;
  const boundaries = [];
  const gateAuthorityCache = new Map();
  if (introduction) {
    const base = firstParent(rootDir, introduction);
    const rangeArgs = ["rev-list", "--reverse", "--topo-order", "HEAD"];
    if (base) rangeArgs.push(`^${base}`);
    const commits = runGit(rootDir, rangeArgs)?.trim().split(/\r?\n/).filter(Boolean);
    if (!commits) return null;
    for (const commit of commits) {
      const parent = firstParent(rootDir, commit);
      const changes = readCommitChanges(rootDir, commit, parent);
      if (!changes) return null;
      boundaries.push({
        label: `commit ${commit}`,
        changes,
        priorMarker: readGitMarker(rootDir, parent),
        priorGate: readGitGateAuthority(rootDir, parent, gateAuthorityCache),
      });
    }
  }

  boundaries.push({
    label: "working tree",
    changes: workingChanges,
    priorMarker: readGitMarker(rootDir, "HEAD"),
    priorGate: readGitGateAuthority(rootDir, "HEAD", gateAuthorityCache),
  });
  return boundaries;
}

function isIsoTimestamp(value) {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/.test(value)) return false;
  const parsed = new Date(value);
  return !Number.isNaN(parsed.valueOf()) && parsed.toISOString() === value;
}

function validateNonAiGateMarker(marker, expectedGate, label, errors) {
  if (!marker || typeof marker !== "object" || Array.isArray(marker)) {
    errors.push(`${label} NONAI gate marker is missing or invalid JSON`);
    return false;
  }
  let valid = true;
  const keys = Object.keys(marker).sort(compareCodePoints);
  if (keys.length !== NONAI_GATE_MARKER_FIELDS.length
    || keys.some((key, index) => key !== NONAI_GATE_MARKER_FIELDS[index])) {
    errors.push(`${label} NONAI gate marker must contain only the canonical fields`);
    valid = false;
  }
  if (marker.formatVersion !== 1) {
    errors.push(`${label} NONAI gate marker must use formatVersion 1`);
    valid = false;
  }
  if (marker.gate !== "NONAI-COMPLETE") {
    errors.push(`${label} NONAI gate marker must use gate NONAI-COMPLETE`);
    valid = false;
  }
  if (!/^WV3-L\d{3}$/.test(marker.ledgerId ?? "")) {
    errors.push(`${label} NONAI gate marker has an invalid ledgerId`);
    valid = false;
  }
  if (!Array.isArray(marker.decisionIds)
    || marker.decisionIds.length === 0
    || marker.decisionIds.some((id) => !/^WV3-\d{3}$/.test(id))
    || new Set(marker.decisionIds).size !== marker.decisionIds.length) {
    errors.push(`${label} NONAI gate marker has invalid decisionIds`);
    valid = false;
  }
  if (!isIsoTimestamp(marker.recordedAt)) {
    errors.push(`${label} NONAI gate marker recordedAt must be an ISO 8601 UTC timestamp`);
    valid = false;
  }
  if (marker.evidenceCommit !== "PENDING" && !/^[0-9a-f]{40}$/.test(marker.evidenceCommit ?? "")) {
    errors.push(`${label} NONAI gate marker evidenceCommit must be PENDING or a full lowercase Git commit ID`);
    valid = false;
  }
  if (!expectedGate) {
    errors.push(`${label} NONAI gate marker does not reference a valid NONAI-COMPLETE ledger record`);
    return false;
  }
  if (marker.ledgerId !== expectedGate.ledgerId) {
    errors.push(`${label} NONAI gate marker ledgerId ${marker.ledgerId} does not match latest valid gate ${expectedGate.ledgerId}`);
    valid = false;
  }
  if (JSON.stringify(marker.decisionIds) !== JSON.stringify(expectedGate.decisionIds)) {
    errors.push(`${label} NONAI gate marker decisionIds do not match ${expectedGate.ledgerId}`);
    valid = false;
  }
  return valid;
}

function readCurrentNonAiGateMarker(rootDir, latestValidGate, errors) {
  const markerPath = path.join(rootDir, NONAI_GATE_MARKER_PATH);
  if (!fs.existsSync(markerPath)) {
    if (latestValidGate) errors.push(`Missing NONAI gate marker for ${latestValidGate.ledgerId}: ${NONAI_GATE_MARKER_PATH}`);
    return null;
  }
  let marker;
  try {
    marker = JSON.parse(readText(markerPath));
  } catch {
    errors.push(`Current NONAI gate marker is invalid JSON: ${NONAI_GATE_MARKER_PATH}`);
    return null;
  }
  validateNonAiGateMarker(marker, latestValidGate, "Current", errors);
  return marker;
}

function existingAiSourcePrefixes(rootDir) {
  return AI_SOURCE_PREFIXES.filter((prefix) => fs.existsSync(path.join(rootDir, prefix)));
}

function validateAiSourceCommitBoundary(rootDir, progress, options, errors) {
  readCurrentNonAiGateMarker(rootDir, progress.latestValidNonAiGate, errors);
  const existingAiPaths = existingAiSourcePrefixes(rootDir);

  let boundaries;
  if (Object.hasOwn(options, "injectedBoundary")) {
    // Tests inject one complete boundary: parent ledger authority and marker are
    // independent evidence, matching the two Git reads used in production.
    const injected = options.injectedBoundary ?? {};
    boundaries = [{
      label: "Injected snapshot",
      changes: (injected.changedPaths ?? []).map((repoPath) => ({ status: "A", path: repoPath })),
      priorMarker: injected.priorMarker ?? null,
      priorGate: injected.priorGate ?? null,
    }];
  } else {
    boundaries = collectGitChangeBoundaries(rootDir);
    if (!boundaries) {
      boundaries = [{
        label: "Git-unavailable snapshot",
        changes: existingAiPaths.map((repoPath) => ({ status: "A", path: repoPath })),
        priorMarker: null,
        priorGate: null,
      }];
    }
  }

  const currentParent = boundaries.at(-1);
  const currentParentGateValid = progress.nonAiGateValid && validateNonAiGateMarker(
    currentParent?.priorMarker,
    currentParent?.priorGate,
    `${currentParent?.label ?? "Current"} parent`,
    [],
  );
  if (!currentParentGateValid) {
    for (const aiPath of existingAiPaths) {
      errors.push(`Production AI path exists before a valid Gate NONAI-COMPLETE: ${aiPath}`);
    }
  }

  for (const boundary of boundaries) {
    const changedAiPaths = [...new Set(boundary.changes
      .filter((change) => change.status !== "D")
      .map((change) => normalizeRepoPath(change.path))
      .filter(isAiSourcePath))];
    if (changedAiPaths.length === 0) continue;
    if (!progress.nonAiGateValid) {
      errors.push(`${boundary.label} changes AI source while the current NONAI-COMPLETE gate is invalid: ${changedAiPaths.join(", ")}`);
    }
    const priorErrors = [];
    const priorValid = validateNonAiGateMarker(
      boundary.priorMarker,
      boundary.priorGate,
      `${boundary.label} parent`,
      priorErrors,
    );
    if (!priorValid) {
      errors.push(`${boundary.label} changes AI source without the latest valid NONAI gate marker in its parent commit: ${changedAiPaths.join(", ")}`);
    }
  }
}

function validateMatrixProgress(rows, entriesWithRefs, releaseMatrixSource, errors) {
  const stateByCapability = new Map(rows.map((row) => [row.id, {
    status: "not-started",
    scope: row.scope === "aggregate" ? "aggregate" : "required",
    evidenceGrade: null,
    decisions: [],
    retirement: null,
    hasAdvanced: false,
  }]));
  let nonAiGateValid = false;
  let nonAiGateEpoch = 0;
  let latestValidNonAiGate = null;
  let wailsCutoverValid = false;
  let rollbackClosedValid = false;
  let cutoverRequiredLeafIds = new Set();
  for (const entry of entriesWithRefs) {
    if (!entry.transition) {
      if (entry.gate === "NONAI-COMPLETE") nonAiGateValid = false;
      continue;
    }
    if (entry.gate === "NONAI-COMPLETE") {
      let valid = entry.allNonAiRequiredRows
        && entry.referencedTasks.length === 1
        && entry.referencedTasks[0] === "P6-05"
        && entry.transition[1] === "not-started"
        && entry.transition[2] === "not-started"
        && entry.evidenceGrade === "A"
        && entry.nonAiVerificationValid
        && entry.scopeChangeValue === "none"
        && entry.closureEvidence === "none"
        && entry.retirement?.kind === "none"
        && entry.decisionReferencesValid
        && REQUIRED_NONAI_DECISION_CATEGORIES.every((category) => entry.categories.has(category));
      const requiredAtGate = rows.filter((candidate) => {
        const state = stateByCapability.get(candidate.id);
        return state?.scope === "required" && isRequiredNonAiGateRow(candidate, rows);
      });
      for (const row of requiredAtGate) {
        const state = stateByCapability.get(row.id);
        if (state?.status !== "verified") {
          errors.push(`${entry.entry.id} Gate NONAI-COMPLETE is premature: ${row.id} is ${state?.status}, expected verified`);
          valid = false;
        }
      }
      for (const row of rows.filter((candidate) => candidate.id.startsWith(AI_CAPABILITY_PREFIX))) {
        const state = stateByCapability.get(row.id);
        if (state?.status !== "not-started") {
          errors.push(`${entry.entry.id} Gate NONAI-COMPLETE requires ${row.id} to remain not-started`);
          valid = false;
        }
      }
      if (/^\| RT-[^\n]*\|\s*decision-required(?:\s*,[^|]*)?\s*\|\s*$/m.test(releaseMatrixSource)) {
        errors.push(`${entry.entry.id} Gate NONAI-COMPLETE is premature: release target decisions remain unresolved`);
        valid = false;
      }
      nonAiGateValid = valid;
      if (valid) {
        nonAiGateEpoch += 1;
        latestValidNonAiGate = {
          ledgerId: entry.entry.id,
          decisionIds: [...entry.referencedDecisions],
        };
      }
      continue;
    }
    if (entry.gate === "WAILS-CUTOVER") {
      let valid = entry.referencedTasks.length === 1
        && entry.referencedTasks[0] === "P8-02"
        && entry.evidenceGrade === "A"
        && entry.cutoverVerificationValid
        && entry.scopeChangeValue === "none"
        && entry.closureEvidence === "none"
        && entry.capabilityIds.length === 1
        && entry.capabilityIds[0] === "REL-03.1"
        && entry.decisionReferencesValid
        && entry.retirement
        && !entry.allRows;
      const purityStatus = stateByCapability.get("REL-03.1")?.status;
      if (!["verified", "migrated"].includes(purityStatus)) {
        errors.push(`${entry.entry.id} Gate WAILS-CUTOVER requires REL-03.1 to be verified or migrated`);
        valid = false;
      }
      if (entry.transition[1] !== purityStatus || entry.transition[2] !== purityStatus) {
        errors.push(`${entry.entry.id} Gate WAILS-CUTOVER must record the current REL-03.1 status without advancement`);
        valid = false;
      }
      for (const row of rows.filter((candidate) => (
        candidate.id.startsWith(AI_CAPABILITY_PREFIX) && isLeafCapability(candidate, rows)
      ))) {
        const state = stateByCapability.get(row.id);
        if (state?.scope === "required" && state.status !== "verified") {
          errors.push(`${entry.entry.id} Gate WAILS-CUTOVER requires ${row.id} to be verified or removed by an approved decision`);
          valid = false;
        }
      }
      if (entry.transition[1] !== entry.transition[2]) {
        errors.push(`${entry.entry.id} Gate WAILS-CUTOVER must not advance capability status`);
        valid = false;
      }
      if (valid) {
        wailsCutoverValid = true;
        rollbackClosedValid = false;
        cutoverRequiredLeafIds = new Set(rows.filter((candidate) => {
          const state = stateByCapability.get(candidate.id);
          return state?.scope === "required"
            && isLeafCapability(candidate, rows)
            && ["verified", "migrated"].includes(state.status);
        }).map((candidate) => candidate.id));
      }
      continue;
    }
    if (entry.gate === "ROLLBACK-CLOSED") {
      let valid = entry.referencedTasks.length === 1
        && entry.referencedTasks[0] === "P8-03"
        && entry.evidenceGrade === "A"
        && entry.rollbackClosureValid
        && entry.scopeChangeValue === "none"
        && entry.capabilityIds.length === 1
        && entry.capabilityIds[0] === "REL-03.2"
        && entry.decisionReferencesValid
        && entry.retirement
        && !entry.allRows
        && entry.categories.has("rollback-window-closure");
      if (!wailsCutoverValid) {
        errors.push(`${entry.entry.id} Gate ROLLBACK-CLOSED requires an earlier valid WAILS-CUTOVER gate`);
        valid = false;
      }
      const retirementStatus = stateByCapability.get("REL-03.2")?.status;
      if (entry.transition[1] !== retirementStatus || entry.transition[2] !== retirementStatus) {
        errors.push(`${entry.entry.id} Gate ROLLBACK-CLOSED must record the current REL-03.2 status without advancement`);
        valid = false;
      }
      if (entry.transition[1] !== entry.transition[2]) {
        errors.push(`${entry.entry.id} Gate ROLLBACK-CLOSED must not advance capability status`);
        valid = false;
      }
      if (valid) rollbackClosedValid = true;
      continue;
    }
    if (entry.allNonAiRequiredRows) continue;
    if (entry.allRows && entry.transition[1] === entry.transition[2]) continue;
    const referencedIds = entry.allRows ? rows.map((row) => row.id) : entry.capabilityIds;
    for (const capabilityId of referencedIds) {
      const current = stateByCapability.get(capabilityId);
      if (!current) continue;
      const row = rows.find((candidate) => candidate.id === capabilityId);
      if (row?.scope === "aggregate" && entry.transition[2] !== "not-started") {
        errors.push(`${entry.entry.id} advances aggregate capability ${capabilityId}`);
      }
      if (capabilityId.startsWith(AI_CAPABILITY_PREFIX) && entry.transition[2] !== "not-started" && !nonAiGateValid) {
        errors.push(`${entry.entry.id} advances ${capabilityId} without a currently valid Gate NONAI-COMPLETE epoch`);
      }
      const isAdvancement = entry.transition[1] !== entry.transition[2]
        && ADVANCEMENT_STATUSES.has(entry.transition[2]);
      if (capabilityId === "REL-03.1" && !current.hasAdvanced && isAdvancement
        && (entry.referencedTasks.length !== 1 || entry.referencedTasks[0] !== "P8-01")) {
        errors.push(`${entry.entry.id} first advances REL-03.1 without only plan task P8-01`);
      }
      if (capabilityId === "REL-03.2" && isAdvancement) {
        if (!wailsCutoverValid) errors.push(`${entry.entry.id} advances REL-03.2 before Gate WAILS-CUTOVER`);
        if (!rollbackClosedValid) errors.push(`${entry.entry.id} advances REL-03.2 before Gate ROLLBACK-CLOSED`);
        if (entry.referencedTasks.length !== 1 || !["P9-01", "P9-02"].includes(entry.referencedTasks[0])) {
          errors.push(`${entry.entry.id} advances REL-03.2 without plan task P9-01 or P9-02`);
        }
        if (!current.hasAdvanced
          && (entry.referencedTasks.length !== 1 || entry.referencedTasks[0] !== "P9-01")) {
          errors.push(`${entry.entry.id} first advances REL-03.2 without only plan task P9-01`);
        }
        if (["probe", "implemented"].includes(entry.transition[2])
          && (entry.referencedTasks.length !== 1 || entry.referencedTasks[0] !== "P9-01")) {
          errors.push(`${entry.entry.id} advances REL-03.2 to ${entry.transition[2]} without only plan task P9-01`);
        }
        if (["verified", "migrated"].includes(entry.transition[2])
          && (entry.referencedTasks.length !== 1 || entry.referencedTasks[0] !== "P9-02")) {
          errors.push(`${entry.entry.id} advances REL-03.2 to ${entry.transition[2]} without only plan task P9-02`);
        }
        if (!entry.categories.has("rollback-window-closure")) {
          errors.push(`${entry.entry.id} advances REL-03.2 without decision category: rollback-window-closure`);
        }
      }
      if (entry.transition[1] !== current.status) {
        errors.push(
          `${entry.entry.id} transition for ${capabilityId} starts at ${entry.transition[1]} but current ledger state is ${current.status}`,
        );
      }
      if (entry.scopeRemoval && current.scope !== "required") {
        errors.push(`${entry.entry.id} Scope change for ${capabilityId} starts at required but current ledger scope is ${current.scope}`);
      }
      const nextScope = entry.scopeRemoval === capabilityId ? "removed" : current.scope;
      const wasRequiredNonAiComplete = nonAiGateValid
        && current.scope === "required"
        && isRequiredNonAiGateRow(row, rows)
        && ["verified", "migrated"].includes(current.status)
        && !["verified", "migrated"].includes(entry.transition[2]);
      const retirement = entry.retirement && entry.retirement.kind !== "none"
        ? entry.retirement
        : current.retirement;
      stateByCapability.set(capabilityId, {
        status: entry.transition[2],
        scope: nextScope,
        evidenceGrade: entry.evidenceGrade,
        decisions: entry.referencedDecisions.length > 0 ? entry.referencedDecisions : current.decisions,
        retirement,
        hasAdvanced: current.hasAdvanced || isAdvancement,
      });
      if (wasRequiredNonAiComplete) nonAiGateValid = false;
      if (wailsCutoverValid
        && cutoverRequiredLeafIds.has(capabilityId)
        && ["blocked", "implemented", "probe", "not-started"].includes(entry.transition[2])) {
        wailsCutoverValid = false;
        rollbackClosedValid = false;
      }
    }
  }
  for (const row of rows) {
    const state = stateByCapability.get(row.id);
    if (state?.status !== row.status) errors.push(`${row.id} matrix status ${row.status} does not match latest ledger state ${state?.status}`);
    if (state?.scope !== row.scope) errors.push(`${row.id} matrix scope ${row.scope} does not match latest ledger scope ${state?.scope}`);
    if (["verified", "migrated"].includes(row.status)) {
      if (state?.evidenceGrade !== "A") {
        errors.push(`${row.id} is ${row.status} without evidence grade A`);
      }
      if (isLeafCapability(row, rows) && !state?.retirement) {
        errors.push(`${row.id} is ${row.status} without a retained canonical Electron retirement trigger`);
      }
    }
    if (row.status === "retired" && state?.decisions.length === 0) {
      errors.push(`${row.id} is retired without an approved decision`);
    }
    if (row.scope === "removed" && !["removed", "deleted"].includes(state?.retirement?.kind)) {
      errors.push(`${row.id} is removed without retained removed/deleted evidence`);
    }
    if (COMPOSITE_CAPABILITIES.has(row.id) && ["implemented", "verified", "migrated"].includes(row.status)) {
      const childPrefix = `${row.id}.`;
      if (!rows.some((candidate) => candidate.id.startsWith(childPrefix))) {
        errors.push(`${row.id} must be split into stable child rows before implementation`);
      }
    }
  }
  return {
    nonAiGateValid,
    nonAiGateEpoch,
    latestValidNonAiGate,
    wailsCutoverValid,
    rollbackClosedValid,
  };
}

function validateNoIdRangeShorthand(markdownFiles, errors) {
  for (const filePath of markdownFiles) {
    const source = readText(filePath);
    for (const match of source.matchAll(CAPABILITY_RANGE_PATTERN)) {
      errors.push(`${path.basename(filePath)} uses forbidden capability ID range shorthand: ${match[0]}`);
    }
    for (const match of source.matchAll(TASK_RANGE_PATTERN)) {
      errors.push(`${path.basename(filePath)} uses forbidden plan task ID range shorthand: ${match[0]}`);
    }
  }
}

function validatePlanDependencies(source, errors) {
  const p704Start = source.indexOf("### P7-04 ");
  const p705Start = source.indexOf("### P7-05 ", p704Start + 1);
  if (p704Start >= 0 && p705Start >= 0) {
    const p704 = source.slice(p704Start, p705Start);
    for (const task of ["P7-01", "P7-02", "P7-03"]) {
      if (!p704.includes(task)) errors.push(`P7-04 prerequisite is missing ${task}`);
    }
    if (!/handler/i.test(p704)) errors.push("P7-04 prerequisite is missing verified handler dependencies");
  }

  const p901Start = source.indexOf("### P9-01 ");
  const p902Start = source.indexOf("### P9-02 ", p901Start + 1);
  if (p901Start < 0 || p902Start < 0) return;
  const p901 = source.slice(p901Start, p902Start).replace(/\s+/g, " ");
  for (const requirement of [
    "P8-03",
    "Gate: ROLLBACK-CLOSED",
    "accepted decision",
    "rollback-window-closure",
    "thresholds=<nonempty>; sample=<nonempty>; blockers=<nonempty>; authority=<nonempty>",
  ]) {
    if (!p901.includes(requirement)) errors.push(`P9-01 prerequisite is missing ${requirement}`);
  }
}

function validateCrossDocumentIds(markdownFiles, knownIds, pattern, label, errors) {
  for (const filePath of markdownFiles) {
    const source = readText(filePath);
    for (const value of extractIds(source, pattern)) {
      if (!knownIds.has(value)) errors.push(`${path.basename(filePath)} references unknown ${label}: ${value}`);
    }
  }
}

function validateDecisionReferences(markdownFiles, decisionIds, errors) {
  for (const filePath of markdownFiles) {
    const source = readText(filePath);
    for (const decisionId of extractIds(source, /WV3-\d{3}/g)) {
      if (!decisionIds.has(decisionId)) {
        errors.push(`${path.basename(filePath)} references unknown decision: ${decisionId}`);
      }
    }
  }
}

function headingAnchors(source) {
  const counts = new Map();
  const anchors = new Set();
  for (const match of source.matchAll(/^#{1,6}\s+(.+)$/gm)) {
    const base = match[1]
      .trim()
      .toLowerCase()
      .replace(/[`*_~]/g, "")
      .replace(/[^\p{L}\p{N}\s-]/gu, "")
      .replace(/\s+/g, "-")
      .replace(/-+/g, "-");
    const count = counts.get(base) ?? 0;
    counts.set(base, count + 1);
    anchors.add(count === 0 ? base : `${base}-${count}`);
  }
  return anchors;
}

function collectMarkdownTargets(source, fileName, errors) {
  const targets = [];
  const definitions = new Map();
  for (const match of source.matchAll(/^\s*\[([^\]]+)\]:\s*(<[^>]+>|\S+)/gm)) {
    const label = match[1].trim().toLowerCase();
    const target = match[2].replace(/^<|>$/g, "");
    definitions.set(label, target);
    targets.push(target);
  }
  for (const match of source.matchAll(/(?<!!)\[[^\]]+\]\(\s*(<[^>]+>|[^\s)]+)(?:\s+["'][^"']*["'])?\s*\)/g)) {
    targets.push(match[1].replace(/^<|>$/g, ""));
  }
  for (const match of source.matchAll(/(?<!!)\[([^\]]+)\]\[([^\]]*)\]/g)) {
    const label = (match[2] || match[1]).trim().toLowerCase();
    if (!definitions.has(label)) errors.push(`${fileName} has an undefined reference link: ${label}`);
  }
  return targets;
}

function validateMarkdownLinks(rootDir, markdownFiles, errors) {
  for (const filePath of markdownFiles) {
    const source = readText(filePath);
    for (const target of collectMarkdownTargets(source, path.basename(filePath), errors)) {
      if (/^(?:https?:|mailto:)/.test(target)) continue;
      if (/^(?:[A-Za-z]:[\\/]|[\\/]{1,2})/.test(target)) {
        errors.push(`${path.basename(filePath)} has a forbidden absolute or network link: ${target}`);
        continue;
      }
      const [rawTarget, rawAnchor = ""] = target.split("#", 2);
      let decodedTarget;
      try {
        decodedTarget = decodeURIComponent(rawTarget || "");
      } catch {
        errors.push(`${path.basename(filePath)} has an invalid encoded link: ${target}`);
        continue;
      }
      const targetPath = decodedTarget ? path.resolve(path.dirname(filePath), decodedTarget) : filePath;
      const relativeToRoot = path.relative(rootDir, targetPath);
      if (relativeToRoot.startsWith("..") || path.isAbsolute(relativeToRoot)) {
        errors.push(`${path.basename(filePath)} has a link outside the repository: ${target}`);
        continue;
      }
      if (!fs.existsSync(targetPath)) {
        errors.push(`${path.basename(filePath)} has a broken link: ${target}`);
        continue;
      }
      if (rawAnchor && targetPath.endsWith(".md")) {
        let anchor;
        try {
          anchor = decodeURIComponent(rawAnchor).toLowerCase();
        } catch {
          errors.push(`${path.basename(filePath)} has an invalid encoded Markdown anchor: ${target}`);
          continue;
        }
        if (!headingAnchors(readText(targetPath)).has(anchor)) {
          errors.push(`${path.basename(filePath)} has a broken Markdown anchor: ${target}`);
        }
      }
    }
  }
}

export function checkMigrationDocs(rootDir = defaultRoot, options = {}) {
  const errors = [];
  const docsDir = path.join(rootDir, "docs/migrations/wails-v3");
  const requiredFiles = [
    "README.md",
    "architecture.md",
    "capability-matrix.md",
    "decisions.md",
    "implementation-plan.md",
    "migration-ledger.md",
    "release-target-matrix.md",
    "verification-gates.md",
  ];
  for (const name of requiredFiles) {
    if (!fs.existsSync(path.join(docsDir, name))) errors.push(`Missing migration document: ${name}`);
  }
  if (errors.length > 0) return errors;

  const matrix = parseMatrix(readText(path.join(docsDir, "capability-matrix.md")), errors);
  const matrixIds = new Set(matrix.map((row) => row.id));
  const planSource = readText(path.join(docsDir, "implementation-plan.md"));
  const planTasks = parseHeadingIds(
    planSource,
    /^### (P\d+-\d+[A-Z]?)\b/gm,
    "plan task",
    errors,
  );
  const { accepted: acceptedDecisions } = parseDecisions(readText(path.join(docsDir, "decisions.md")), errors);
  const ledgerEntries = parseLedger(readText(path.join(docsDir, "migration-ledger.md")), errors);
  const entriesWithRefs = ledgerEntries.map((entry) => ({
    entry,
    ...validateLedgerEntry(entry, matrixIds, planTasks, acceptedDecisions, errors),
  }));

  const progress = validateMatrixProgress(
    matrix,
    entriesWithRefs,
    readText(path.join(docsDir, "release-target-matrix.md")),
    errors,
  );
  validateAiSourceCommitBoundary(rootDir, progress, options, errors);
  const markdownFiles = listMarkdownFiles(docsDir);
  validateDecisionReferences(markdownFiles, new Set(acceptedDecisions.keys()), errors);
  validateCrossDocumentIds(
    markdownFiles,
    matrixIds,
    /(?:FND|TERM|SSH|SFTP|NET|SYS|SYNC|AI|PLUG|REL)-\d+(?:\.\d+)?/g,
    "capability",
    errors,
  );
  validateCrossDocumentIds(markdownFiles, planTasks, /P\d+-\d+[A-Z]?/g, "plan task", errors);
  const ledgerIds = new Set(ledgerEntries.map((entry) => entry.id));
  validateCrossDocumentIds(markdownFiles, ledgerIds, /WV3-L\d{3}/g, "ledger entry", errors);
  validateNoIdRangeShorthand(markdownFiles, errors);
  validatePlanDependencies(planSource, errors);
  validateMarkdownLinks(rootDir, markdownFiles, errors);
  return [...new Set(errors)].sort(compareCodePoints);
}

function readRootArgument() {
  const rootIndex = process.argv.indexOf("--root");
  if (rootIndex < 0) return defaultRoot;
  const root = process.argv[rootIndex + 1];
  if (!root) throw new Error("--root requires a directory path");
  return path.resolve(root);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const errors = checkMigrationDocs(readRootArgument());
  if (errors.length > 0) {
    process.stderr.write(`${errors.map((error) => `- ${error}`).join("\n")}\n`);
    process.exitCode = 1;
  } else {
    process.stdout.write("Wails migration documentation is consistent.\n");
  }
}
