"use strict";

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const production = readFileSync(path.join(root, "internal", "terminal", "dataplane", "frame.go"), "utf8");
const probe = readFileSync(path.join(root, "experiments", "terminal-data-plane", "protocol.go"), "utf8");

test("production and probe terminal frame contracts stay aligned", () => {
  assert.match(probe, /frameMagic\s+uint32\s*=\s*0x4e544450/);
  assert.match(probe, /frameVersion\s+uint8\s*=\s*2/);
  assert.match(probe, /frameHeaderSize\s*=\s*40/);
  assert.match(probe, /maxPayloadBytes\s*=\s*128\s*\*\s*1024/);
  assert.match(production, /FrameMagic\s+uint32\s*=\s*0x4e544450/);
  assert.match(production, /FrameVersion\s+uint8\s*=\s*2/);
  assert.match(production, /FrameHeaderBytes\s*=\s*40/);
  assert.match(production, /MaxPayloadBytes\s*=\s*128\s*\*\s*1024/);
});
