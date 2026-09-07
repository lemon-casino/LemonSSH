import { decodeFrame, encodeFrame, FrameKind, FRAME_HEADER_BYTES, isExpectedUrgentAck, type TerminalFrame, UrgentAckLedger } from "../src/protocol.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

const source: TerminalFrame = {
  kind: FrameKind.Output,
  generation: 7,
  sequence: 42n,
  creditCost: 3,
  correlation: 9,
  timestampMicros: 123456,
  payload: new Uint8Array([1, 2, 3]),
};
const decoded = decodeFrame(encodeFrame(source));
assert(decoded.kind === source.kind, "kind roundtrip");
assert(decoded.generation === source.generation, "generation roundtrip");
assert(decoded.sequence === source.sequence, "sequence roundtrip");
assert(decoded.creditCost === source.creditCost, "credit roundtrip");
assert(decoded.correlation === source.correlation, "correlation roundtrip");
assert(decoded.timestampMicros === source.timestampMicros, "timestamp roundtrip");
assert(decoded.payload.join(",") === "1,2,3", "payload roundtrip");

const malformed = encodeFrame(source);
new DataView(malformed).setUint8(7, 1);
let rejected = false;
try {
  decodeFrame(malformed);
} catch {
  rejected = true;
}
assert(rejected, "reserved byte rejection");

rejected = false;
try {
  decodeFrame(new ArrayBuffer(FRAME_HEADER_BYTES - 1));
} catch {
  rejected = true;
}
assert(rejected, "short frame rejection");

const urgentAck: TerminalFrame = {
  kind: FrameKind.UrgentAck,
  generation: 3,
  sequence: 9n,
  creditCost: 0,
  correlation: 44,
  timestampMicros: 123,
  payload: new Uint8Array(),
};
assert(isExpectedUrgentAck(urgentAck, { generation: 3, sequence: 9n, correlation: 44 }), "strict urgent ACK acceptance");
assert(!isExpectedUrgentAck({ ...urgentAck, creditCost: 1 }, { generation: 3, sequence: 9n, correlation: 44 }), "urgent ACK credit rejection");
assert(!isExpectedUrgentAck({ ...urgentAck, timestampMicros: 0 }, { generation: 3, sequence: 9n, correlation: 44 }), "urgent ACK timestamp rejection");

const urgentLedger = new UrgentAckLedger(2);
const issuedOne = urgentLedger.issue(3, 9n);
urgentLedger.expire(issuedOne);
assert(urgentLedger.consume({ ...urgentAck, correlation: issuedOne }) === "late", "late timed-out urgent ACK is ignored");
const issuedTwo = urgentLedger.issue(3, 10n);
assert(urgentLedger.consume({ ...urgentAck, correlation: issuedTwo, sequence: 11n }) === "invalid", "malformed live urgent ACK is rejected");
assert(urgentLedger.consume({ ...urgentAck, correlation: 99 }) === "invalid", "impossible future urgent ACK is rejected");

const boundedLedger = new UrgentAckLedger(1);
boundedLedger.issue(3, 9n);
boundedLedger.expire(1);
boundedLedger.issue(3, 10n);
boundedLedger.expire(2);
assert(boundedLedger.consume({ ...urgentAck, correlation: 1 }) === "late", "valid evicted urgent tombstone remains harmless");
assert(boundedLedger.consume({ ...urgentAck, correlation: 1, generation: 2 }) === "invalid", "malformed evicted urgent ACK is rejected");

const exhaustedLedger = new UrgentAckLedger();
exhaustedLedger.setNextForTest(0xffff_ffff);
assert(exhaustedLedger.issue(1, 0n) === 0xffff_ffff, "final uint32 urgent correlation is issued");
let exhaustionRejected = false;
try { exhaustedLedger.issue(1, 0n); } catch { exhaustionRejected = true; }
assert(exhaustionRejected, "urgent uint32 correlation exhaustion is explicit");

console.log("protocol tests passed");
