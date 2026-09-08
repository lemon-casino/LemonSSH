import assert from "node:assert/strict";
import { test } from "node:test";

import {
  unmarshalFrame,
  marshalFrame,
  FRAME_HEADER_BYTES,
  MAX_PAYLOAD_BYTES,
  FrameCodecError,
  FRAME_OUTPUT,
  FRAME_CREDIT,
} from "./frame";

test("TS codec round-trips a Go-shaped output frame", () => {
  const frame = {
    kind: FRAME_OUTPUT,
    generation: 2,
    sequence: 17,
    creditCost: 4,
    correlation: 9,
    timestampMicros: 123,
    payload: new TextEncoder().encode("output"),
  };
  const encoded = marshalFrame(frame);
  assert.equal(encoded.length, FRAME_HEADER_BYTES + frame.payload.length);
  const decoded = unmarshalFrame(encoded);
  assert.equal(decoded.kind, frame.kind);
  assert.equal(decoded.generation, 2);
  assert.equal(decoded.sequence, 17);
  assert.equal(decoded.creditCost, 4);
  assert.equal(decoded.correlation, 9);
  assert.equal(decoded.timestampMicros, 123);
  assert.deepEqual(decoded.payload, frame.payload);
});

test("TS codec rejects malformed frames like the Go side", () => {
  assert.throws(() => unmarshalFrame(new Uint8Array(FRAME_HEADER_BYTES - 1)));
  assert.throws(() => unmarshalFrame(new Uint8Array(10)));
  const frame = marshalFrame({ kind: FRAME_CREDIT, generation: 1, sequence: 1, creditCost: 0, correlation: 0, timestampMicros: 0, payload: new Uint8Array(0) });
  const bad = new Uint8Array(frame);
  bad[0] ^= 1;
  assert.throws(() => unmarshalFrame(bad));
  const badFlags = new Uint8Array(frame);
  badFlags[6] = 1;
  assert.throws(() => unmarshalFrame(badFlags));
});

test("oversized payloads are rejected by the TS codec", () => {
  assert.throws(() => marshalFrame({
    kind: FRAME_OUTPUT, generation: 1, sequence: 1, creditCost: 0,
    correlation: 0, timestampMicros: 0,
    payload: new Uint8Array(MAX_PAYLOAD_BYTES + 1),
  }), FrameCodecError);
});

test("decoder accepts the maximum payload size", () => {
  const frame = { kind: FRAME_OUTPUT, generation: 1, sequence: 1, creditCost: 0, correlation: 0, timestampMicros: 0, payload: new Uint8Array(MAX_PAYLOAD_BYTES) };
  const encoded = marshalFrame(frame);
  const decoded = unmarshalFrame(encoded);
  assert.equal(decoded.payload.length, MAX_PAYLOAD_BYTES);
});
