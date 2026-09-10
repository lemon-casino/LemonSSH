import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { parseArgs, resolveSignTool, signingStatus } from "./sign-wails-probe.mjs";

test("signingStatus stays unsigned without artifact, signtool, or cert", async () => {
  assert.deepEqual(signingStatus({}), { signed: false, reason: "missing-artifact" });
  const dir = await mkdtemp(path.join(tmpdir(), "sign-probe-"));
  const artifact = path.join(dir, "LemonSSH.exe");
  await writeFile(artifact, "not-an-exe");
  assert.equal(signingStatus({ artifactPath: artifact }).reason, "signtool-unavailable");
  assert.equal(
    signingStatus({ artifactPath: artifact, signToolPath: artifact }).reason,
    "certificate-unavailable",
  );
  const pending = signingStatus({
    artifactPath: artifact,
    signToolPath: artifact,
    certThumbprint: "ABC",
  });
  assert.equal(pending.signed, false);
  assert.equal(pending.reason, "not-invoked");
});

test("resolveSignTool prefers SIGNTOOL_PATH when the file exists", async () => {
  const dir = await mkdtemp(path.join(tmpdir(), "signtool-"));
  const tool = path.join(dir, "signtool.exe");
  await writeFile(tool, "stub");
  assert.equal(resolveSignTool({ SIGNTOOL_PATH: tool }, () => null), tool);
  assert.equal(resolveSignTool({}, () => null), null);
});

test("parseArgs accepts artifact and cert", () => {
  assert.deepEqual(parseArgs(["--artifact", "a.exe", "--cert", "deadbeef"]), {
    artifact: "a.exe",
    cert: "deadbeef",
  });
});
