import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

import type {
  ServiceError,
  ServiceRequest,
  SubscriptionEvent,
  SubscriptionOpen,
  HealthStatus,
  VersionInfo,
  WindowRoleInfo,
} from "./generated/contracts";
import { isServiceErrorCode, toServiceError } from "./errorMapping";

const fixturesDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../../testdata/migration/contracts",
);

function readFixture(name: string): unknown {
  return JSON.parse(readFileSync(path.join(fixturesDir, name), "utf8"));
}

test("generated contracts match the Go golden fixtures", () => {
  const serviceError = readFixture("error.json") as ServiceError;
  assert.equal(serviceError.code, "netcatty.unavailable");
  assert.equal(serviceError.message, "shell offline");
  assert.equal(serviceError.details?.shell, "wails");
  assert.ok(isServiceErrorCode(serviceError.code));

  const request = readFixture("request.json") as ServiceRequest;
  assert.match(request.requestId, /^req_[0-9a-f]{24}$/);

  const event = readFixture("subscription.json") as SubscriptionEvent;
  assert.equal(event.subscriptionId, "sub_fixed");
  assert.equal(event.topic, "terminal.output");
  assert.equal(event.sequence, 42);
  // Go []byte arrives base64-encoded on the wire.
  assert.equal(Buffer.from(event.payload ?? "", "base64").toString("utf8"), "chunk");

  const instance = readFixture("instance.json") as {
    id: string;
    owner: string;
    request: ServiceRequest;
  };
  assert.match(instance.id, /^inst_[0-9a-f]{24}$/);
  assert.match(instance.owner, /^win_[0-9a-f]{24}$/);
});

test("code union rejects unknown codes", () => {
  assert.equal(isServiceErrorCode("netcatty.nope"), false);
  assert.equal(isServiceErrorCode("err"), false);
});

test("error mapping follows the Go AsError semantics", () => {
  const unavailable = toServiceError(
    Object.assign(new Error("bridge gone"), { name: "BridgeUnavailableError" }),
  );
  assert.equal(unavailable.code, "netcatty.unavailable");

  const cancelled = toServiceError(Object.assign(new Error("stop"), { name: "AbortError" }));
  assert.equal(cancelled.code, "netcatty.cancelled");

  const deadline = toServiceError(Object.assign(new Error("slow"), { name: "TimeoutError" }));
  assert.equal(deadline.code, "netcatty.deadline_exceeded");
  assert.equal(deadline.retryable, true);

  assert.equal(toServiceError("boom").code, "netcatty.unknown");
});

test("skeleton service shapes satisfy the contract interfaces", () => {
  // Structural assertions that the generated types remain usable as the
  // payloads produced by internal/app (checked end to end in Go tests).
  const health: HealthStatus = { status: "ok", pid: 1 };
  const version: VersionInfo = {
    name: "Netcatty",
    version: "0.0.0-wails-skeleton",
    goos: "windows",
    goarch: "amd64",
    goVersion: "go1.25.0",
  };
  const role: WindowRoleInfo = { role: "main", singleInstance: true };
  const open: SubscriptionOpen = { subscriptionId: "sub_fixed", topic: "terminal.output" };
  assert.ok(health.status && version.name && role.role && open.subscriptionId);
});
