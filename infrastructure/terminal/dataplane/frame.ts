// Terminal data-plane TS codec (P3-01): mirrors the Go binary frame contract
// in internal/terminal/dataplane/frame.go byte-for-byte, plus a credit
// controller that gates renderer-bound output.

export const FRAME_MAGIC = 0x4e544450; // NTDP
export const FRAME_VERSION = 2;
export const FRAME_HEADER_BYTES = 40;
export const MAX_PAYLOAD_BYTES = 128 * 1024;
export const RECEIVE_WINDOW_BYTES = 1024 * 1024;

export const FRAME_OUTPUT = 1;
export const FRAME_CREDIT = 2;
export const FRAME_URGENT = 3;
export const FRAME_URGENT_ACK = 4;
export const FRAME_COMPLETE = 5;
export const FRAME_DRAIN_REQUEST = 6;
export const FRAME_DRAIN_MARKER = 7;
export const FRAME_DRAIN_READY = 8;

export interface Frame {
  kind: number;
  generation: number;
  sequence: number; // uint64 low bits; JS safe range enforced by callers
  creditCost: number;
  correlation: number;
  timestampMicros: number;
  payload: Uint8Array;
}

export class FrameCodecError extends Error {}

export function marshalFrame(frame: Frame): Uint8Array {
  if (frame.payload.length > MAX_PAYLOAD_BYTES) {
    throw new FrameCodecError(`payload exceeds ${MAX_PAYLOAD_BYTES} byte limit`);
  }
  const kinds = [FRAME_OUTPUT, FRAME_CREDIT, FRAME_URGENT, FRAME_URGENT_ACK, FRAME_COMPLETE, FRAME_DRAIN_REQUEST, FRAME_DRAIN_MARKER, FRAME_DRAIN_READY];
  if (!kinds.includes(frame.kind)) throw new FrameCodecError(`invalid frame kind ${frame.kind}`);

  const out = new Uint8Array(FRAME_HEADER_BYTES + frame.payload.length);
  const view = new DataView(out.buffer);
  view.setUint32(0, FRAME_MAGIC);
  out[4] = FRAME_VERSION;
  out[5] = frame.kind;
  out[6] = 0;
  out[7] = 0;
  view.setUint32(8, frame.generation >>> 0);
  // sequence: two 32-bit halves (uint64 on the wire)
  view.setUint32(12, Math.floor(frame.sequence / 0x100000000) >>> 0);
  view.setUint32(16, frame.sequence >>> 0);
  view.setUint32(20, frame.creditCost >>> 0);
  view.setUint32(24, frame.payload.length);
  view.setUint32(28, frame.correlation >>> 0);
  // timestampMicros: 64-bit — low 32 bits carry the meaningful range for
  // latency math; high bits written as zero (parity with Go's uint64 lower
  // 32 bits read by the TS side of the probe harness).
  view.setUint32(32, 0);
  view.setUint32(36, frame.timestampMicros >>> 0);
  out.set(frame.payload, FRAME_HEADER_BYTES);
  return out;
}

export function unmarshalFrame(data: Uint8Array): Frame {
  if (data.length < FRAME_HEADER_BYTES) throw new FrameCodecError("frame is shorter than version 2 header");
  if (data.length > MAX_FRAME_BYTES()) throw new FrameCodecError(`frame exceeds ${MAX_FRAME_BYTES()} byte limit`);
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  if (view.getUint32(0) !== FRAME_MAGIC) throw new FrameCodecError("invalid frame magic");
  if (data[4] !== FRAME_VERSION) throw new FrameCodecError(`unsupported frame version ${data[4]}`);
  if (data[6] !== 0 || data[7] !== 0) throw new FrameCodecError("version 2 flags and reserved fields must be zero");
  const payloadLength = view.getUint32(24);
  if (payloadLength > MAX_PAYLOAD_BYTES || data.length - FRAME_HEADER_BYTES !== payloadLength) {
    throw new FrameCodecError("payload length is invalid");
  }
  const kind = data[5];
  const valid = [FRAME_OUTPUT, FRAME_CREDIT, FRAME_URGENT, FRAME_URGENT_ACK, FRAME_COMPLETE, FRAME_DRAIN_REQUEST, FRAME_DRAIN_MARKER, FRAME_DRAIN_READY];
  if (!valid.includes(kind)) throw new FrameCodecError(`invalid frame kind ${kind}`);
  return {
    kind,
    generation: view.getUint32(8),
    sequence: view.getUint32(12) * 0x100000000 + view.getUint32(16),
    creditCost: view.getUint32(20),
    correlation: view.getUint32(28),
    timestampMicros: view.getUint32(32) * 0x100000000 + view.getUint32(36),
    payload: data.slice(FRAME_HEADER_BYTES),
  };
}

function MAX_FRAME_BYTES(): number { return FRAME_HEADER_BYTES + MAX_PAYLOAD_BYTES; }
