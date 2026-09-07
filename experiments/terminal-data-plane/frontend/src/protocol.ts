export const FRAME_HEADER_BYTES = 40;
export const MAX_PAYLOAD_BYTES = 128 * 1024;
export const FRAME_MAGIC = 0x4e544450;
export const FRAME_VERSION = 2;

export const FrameKind = {
  Output: 1,
  Credit: 2,
  Urgent: 3,
  UrgentAck: 4,
  Complete: 5,
  DrainRequest: 6,
  DrainMarker: 7,
  DrainReady: 8,
} as const;

export type FrameKindValue = (typeof FrameKind)[keyof typeof FrameKind];

export interface TerminalFrame {
  kind: FrameKindValue;
  generation: number;
  sequence: bigint;
  creditCost: number;
  correlation: number;
  timestampMicros: number;
  payload: Uint8Array;
}

const validKinds = new Set<number>(Object.values(FrameKind));

export function encodeFrame(frame: TerminalFrame): ArrayBuffer {
  if (!validKinds.has(frame.kind)) throw new Error(`invalid frame kind ${frame.kind}`);
  if (frame.payload.byteLength > MAX_PAYLOAD_BYTES) throw new Error("payload exceeds frame limit");
  const buffer = new ArrayBuffer(FRAME_HEADER_BYTES + frame.payload.byteLength);
  const view = new DataView(buffer);
  view.setUint32(0, FRAME_MAGIC);
  view.setUint8(4, FRAME_VERSION);
  view.setUint8(5, frame.kind);
  view.setUint32(8, frame.generation);
  view.setBigUint64(12, frame.sequence);
  view.setUint32(20, frame.creditCost);
  view.setUint32(24, frame.payload.byteLength);
  view.setUint32(28, frame.correlation);
  view.setBigUint64(32, BigInt(frame.timestampMicros));
  new Uint8Array(buffer, FRAME_HEADER_BYTES).set(frame.payload);
  return buffer;
}

export function decodeFrame(buffer: ArrayBuffer): TerminalFrame {
  if (buffer.byteLength < FRAME_HEADER_BYTES) throw new Error("frame is shorter than header");
  if (buffer.byteLength > FRAME_HEADER_BYTES + MAX_PAYLOAD_BYTES) throw new Error("frame exceeds limit");
  const view = new DataView(buffer);
  if (view.getUint32(0) !== FRAME_MAGIC) throw new Error("invalid frame magic");
  if (view.getUint8(4) !== FRAME_VERSION) throw new Error("unsupported frame version");
  if (view.getUint8(6) !== 0 || view.getUint8(7) !== 0) throw new Error("reserved fields must be zero");
  const kind = view.getUint8(5);
  if (!validKinds.has(kind)) throw new Error(`invalid frame kind ${kind}`);
  const payloadLength = view.getUint32(24);
  if (payloadLength > MAX_PAYLOAD_BYTES || payloadLength !== buffer.byteLength - FRAME_HEADER_BYTES) {
    throw new Error("payload length mismatch");
  }
  return {
    kind: kind as FrameKindValue,
    generation: view.getUint32(8),
    sequence: view.getBigUint64(12),
    creditCost: view.getUint32(20),
    correlation: view.getUint32(28),
    timestampMicros: Number(view.getBigUint64(32)),
    payload: new Uint8Array(buffer.slice(FRAME_HEADER_BYTES)),
  };
}

export function isExpectedUrgentAck(
  frame: TerminalFrame,
  expected: { generation: number; sequence: bigint; correlation: number },
): boolean {
  return frame.kind === FrameKind.UrgentAck && frame.generation === expected.generation &&
    frame.sequence === expected.sequence && frame.correlation === expected.correlation &&
    frame.creditCost === 0 && frame.payload.byteLength === 0 && frame.timestampMicros > 0;
}

type UrgentExpectation = { generation: number; sequence: bigint; correlation: number };

export class UrgentAckLedger {
  private readonly live = new Map<number, UrgentExpectation>();
  private readonly expired = new Map<number, UrgentExpectation>();
  private highestIssued = 0;
  private latestGeneration = 0;
  private highestSequence = 0n;

  constructor(private readonly maxExpired = 32) {}

  issue(generation: number, sequence: bigint): number {
    if (this.highestIssued >= 0xffff_ffff) throw new Error("urgent correlation space exhausted");
    const correlation = this.highestIssued + 1;
    this.register({ generation, sequence, correlation });
    return correlation;
  }

  private register(expected: UrgentExpectation): void {
    if (expected.correlation !== this.highestIssued + 1) throw new Error("urgent correlations must be contiguous");
    this.live.set(expected.correlation, expected);
    this.highestIssued = expected.correlation;
    this.latestGeneration = expected.generation;
    if (expected.sequence > this.highestSequence) this.highestSequence = expected.sequence;
  }

  setNextForTest(nextCorrelation: number): void {
    if (!Number.isInteger(nextCorrelation) || nextCorrelation < 1 || nextCorrelation > 0xffff_ffff) {
      throw new Error("invalid urgent correlation test state");
    }
    this.highestIssued = nextCorrelation - 1;
  }

  expire(correlation: number): void {
    const expected = this.live.get(correlation);
    if (!expected) return;
    this.live.delete(correlation);
    this.expired.set(correlation, expected);
    while (this.expired.size > this.maxExpired) {
      const oldest = this.expired.keys().next().value as number | undefined;
      if (oldest === undefined) break;
      this.expired.delete(oldest);
    }
  }

  consume(frame: TerminalFrame): "live" | "late" | "invalid" {
    const live = this.live.get(frame.correlation);
    if (live) {
      if (!isExpectedUrgentAck(frame, live)) return "invalid";
      this.live.delete(frame.correlation);
      return "live";
    }
    const expired = this.expired.get(frame.correlation);
    if (expired) {
      if (!isExpectedUrgentAck(frame, expired)) return "invalid";
      this.expired.delete(frame.correlation);
      return "late";
    }
    const validPastEnvelope = frame.kind === FrameKind.UrgentAck && frame.correlation > 0 &&
      frame.correlation <= this.highestIssued && frame.creditCost === 0 &&
      frame.generation === this.latestGeneration && frame.sequence <= this.highestSequence &&
      frame.payload.byteLength === 0 && frame.timestampMicros > 0;
    return validPastEnvelope ? "late" : "invalid";
  }
}
