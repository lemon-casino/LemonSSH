import { createSHA256, type IHasher } from "hash-wasm";
import type { TerminalFrame } from "./protocol.js";
import type { CreditStats } from "./creditController.js";

const MAX_LATENCY_SAMPLES = 24;
const MAX_MEMORY_SAMPLES = 4;
const MAX_INTERRUPTION_RECORDS = 8;
const MAX_HASH_STATE_BYTES = 2048;

export interface BackendMemorySample {
  sampledAtUnixMillis: number;
  goHeapAllocBytes: number;
  goHeapInuseBytes: number;
  serviceLifetimeObservedMaxGoHeapAllocBytes: number;
  processRssBytes: number | null;
  processLifetimePeakRssBytes: number | null;
  processRssAvailable: boolean;
  processRssError: string;
}

export interface BackendSessionEvidence {
  generation: number;
  sequence: number;
  appliedSequence: number;
  payloadBytes: number;
  creditBytes: number;
  framesSent: number;
  framesAcked: number;
  framesReplayed: number;
  backendMaxOutstanding: number;
  pauseCount: number;
  resumeCount: number;
  pauseDurationMillis: number;
  urgentCount: number;
  urgentServiceMicros: number;
  drainCount: number;
  drainDurationMillis: number;
  routeInterruptionCount: number;
  frameWriteAttempts: number;
  preStopOutstandingBytes: number;
  startedAtUnixMillis: number;
  completedAtUnixMillis: number;
  error: string;
  closeStatus: string;
  expectedPayloadBytes: number;
  expectedCreditBytes: number;
  expectedFrames: number;
  expectedPayloadSha256: string;
}

interface LatencySample {
  sequence: number;
  micros: number;
}

export type MemoryPhase = "start" | "steady" | "complete" | "settled";

export interface MemorySample {
  sampledAtUnixMillis: number;
  phase: MemoryPhase;
  jsHeapBytes: number | null;
  jsHeapAvailable: boolean;
  backend: BackendMemorySample;
}

export interface InterruptionRecord {
  correlationId: string;
  kind: string;
  startedAtUnixMillis: number;
  endedAtUnixMillis: number;
  durationMillis: number;
  startAcknowledgedSequence: number;
  endAcknowledgedSequence: number;
}

export interface ClientMetricState {
  sessionId: string;
  workload: string;
  phase: string;
  roundIndex: number;
  startedAtUnixMillis: number;
  completedAtUnixMillis: number;
  workloadComplete: boolean;
  roundStopReason: string;
  payloadBytes: number;
  creditBytes: number;
  framesReceived: number;
  framesApplied: number;
  metadataFrames: number;
  appliedSequence: number;
  frontendPendingBytes: number;
  frontendPendingHighWaterBytes: number;
  backendToWebViewMicros: LatencySample[];
  backendToXtermMicros: LatencySample[];
  xtermCallbackMicros: LatencySample[];
  metadataProcessMicros: LatencySample[];
  urgentRendererRttMicros: number[];
  memorySamples: MemorySample[];
  memoryPhaseErrors: Partial<Record<MemoryPhase, string>>;
  interruptions: InterruptionRecord[];
}

export interface ClientSessionReport extends ClientMetricState {
  backend: BackendSessionEvidence | null;
  actualPayloadSha256: string;
  expectedPayloadSha256: string;
  expectedPayloadBytes: number;
  expectedCreditBytes: number;
  expectedFrames: number;
  sequenceIntegrity: boolean | null;
  byteIntegrity: boolean | null;
  creditIntegrity: boolean | null;
  digestIntegrity: boolean | null;
  durationMillis: number;
  throughputBytesPerSecond: number;
  roundObservedSampleMaxima: {
    goHeapAllocBytes: number;
    goHeapInuseBytes: number;
    processRssBytes: number | null;
    jsHeapBytes: number | null;
  };
  memoryPhaseStatus: {
    start: "sampled" | "missing" | "capture-failed";
    steady: "sampled" | "threshold-not-reached" | "capture-failed";
    complete: "sampled" | "session-not-complete" | "capture-failed";
    settled: "sampled" | "missing" | "capture-failed";
  };
  credit: Omit<CreditStats, "acknowledgedSequence"> & { acknowledgedSequence: number };
}

interface HandoffEnvelope {
  version: 2;
  state: ClientMetricState;
  hashStateBase64: string;
}

export interface FrameReceipt {
  sequence: number;
  payloadBytes: number;
  creditCost: number;
  receivedAtUnixMicros: number;
  receivedAtPerformanceMillis: number;
  backendSentAtMicros: number;
}

function pushBounded<T>(target: T[], value: T, limit: number): void {
  if (target.length >= limit) target.shift();
  target.push(value);
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  return Uint8Array.from(binary, (character) => character.charCodeAt(0));
}

function newCorrelationID(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return [...bytes].map((value) => value.toString(16).padStart(2, "0")).join("");
}

function observedMaximum(values: Array<number | null>): number | null {
  const available = values.filter((value): value is number => value !== null);
  return available.length > 0 ? Math.max(...available) : null;
}

export class ClientMetrics {
  private constructor(private readonly hasher: IHasher, private readonly state: ClientMetricState) {}

  static async create(sessionId: string, workload: string, phase: string, roundIndex: number, handoffState?: string): Promise<ClientMetrics> {
    const hasher = await createSHA256();
    if (handoffState) {
      const envelope = JSON.parse(handoffState) as HandoffEnvelope;
      if (envelope.version !== 2 || envelope.state.sessionId !== sessionId || envelope.hashStateBase64.length > 4096) {
        throw new Error("invalid client metrics handoff state");
      }
      envelope.state.backendToWebViewMicros = envelope.state.backendToWebViewMicros.slice(-MAX_LATENCY_SAMPLES);
      envelope.state.backendToXtermMicros = envelope.state.backendToXtermMicros.slice(-MAX_LATENCY_SAMPLES);
      envelope.state.xtermCallbackMicros = envelope.state.xtermCallbackMicros.slice(-MAX_LATENCY_SAMPLES);
      envelope.state.metadataProcessMicros = (envelope.state.metadataProcessMicros ?? []).slice(-MAX_LATENCY_SAMPLES);
      envelope.state.urgentRendererRttMicros = envelope.state.urgentRendererRttMicros.slice(-MAX_LATENCY_SAMPLES);
      if (!Array.isArray(envelope.state.memorySamples) || envelope.state.memorySamples.length > MAX_MEMORY_SAMPLES) {
        throw new Error("memory handoff samples exceed their bound");
      }
      const memoryPhaseOrder: Record<MemoryPhase, number> = { start: 0, steady: 1, complete: 2, settled: 3 };
      envelope.state.memoryPhaseErrors = envelope.state.memoryPhaseErrors ?? {};
      for (const [phase, message] of Object.entries(envelope.state.memoryPhaseErrors)) {
        if (!(phase in memoryPhaseOrder) || typeof message !== "string" || message.length === 0 || message.length > 256) {
          throw new Error("invalid memory phase error handoff state");
        }
      }
      let previousMemoryPhase = -1;
      for (const sample of envelope.state.memorySamples) {
        const order = memoryPhaseOrder[sample.phase];
        if (order === undefined || order <= previousMemoryPhase || !Number.isFinite(sample.sampledAtUnixMillis) ||
          (sample.jsHeapBytes !== null && !Number.isFinite(sample.jsHeapBytes)) || typeof sample.jsHeapAvailable !== "boolean") {
          throw new Error("invalid memory phase handoff state");
        }
        previousMemoryPhase = order;
      }
      if (!Array.isArray(envelope.state.interruptions) || envelope.state.interruptions.length > MAX_INTERRUPTION_RECORDS) {
        throw new Error("interruption handoff history exceeds its bound");
      }
      const correlationIDs = new Set<string>();
      let openInterruptions = 0;
      for (const record of envelope.state.interruptions) {
        const open = record.endedAtUnixMillis === 0;
        const valid = /^[a-f0-9]{32}$/.test(record.correlationId) && !correlationIDs.has(record.correlationId) &&
          typeof record.kind === "string" && record.kind.length > 0 && record.kind.length <= 64 &&
          Number.isFinite(record.startedAtUnixMillis) && record.startedAtUnixMillis > 0 &&
          Number.isFinite(record.endedAtUnixMillis) && record.endedAtUnixMillis >= 0 &&
          Number.isFinite(record.durationMillis) && record.durationMillis >= 0 &&
          Number.isSafeInteger(record.startAcknowledgedSequence) && record.startAcknowledgedSequence >= 0 &&
          Number.isSafeInteger(record.endAcknowledgedSequence) && record.endAcknowledgedSequence >= 0 &&
          (open
            ? record.durationMillis === 0 && record.endAcknowledgedSequence === 0
            : record.endedAtUnixMillis >= record.startedAtUnixMillis &&
              record.durationMillis === record.endedAtUnixMillis - record.startedAtUnixMillis);
        if (!valid) throw new Error("invalid interruption handoff record");
        correlationIDs.add(record.correlationId);
        if (open) openInterruptions += 1;
      }
      if (openInterruptions > 1) throw new Error("multiple open interruption handoff records");
      envelope.state.roundStopReason = envelope.state.roundStopReason ?? "";
      envelope.state.workloadComplete = envelope.state.workloadComplete === true;
      if (envelope.state.workloadComplete &&
        !envelope.state.memorySamples.some((sample) => sample.phase === "complete") &&
        !envelope.state.memoryPhaseErrors.complete) {
        throw new Error("completed handoff state is missing a complete memory outcome");
      }
      const hashState = base64ToBytes(envelope.hashStateBase64);
      if (hashState.byteLength > MAX_HASH_STATE_BYTES) throw new Error("hash handoff state exceeds limit");
      hasher.load(hashState);
      return new ClientMetrics(hasher, envelope.state);
    }
    hasher.init();
    return new ClientMetrics(hasher, {
      sessionId, workload, phase, roundIndex,
      startedAtUnixMillis: Date.now(), completedAtUnixMillis: 0, workloadComplete: false, roundStopReason: "",
      payloadBytes: 0, creditBytes: 0, framesReceived: 0, framesApplied: 0,
      metadataFrames: 0, appliedSequence: 0,
      frontendPendingBytes: 0, frontendPendingHighWaterBytes: 0,
      backendToWebViewMicros: [], backendToXtermMicros: [], xtermCallbackMicros: [], metadataProcessMicros: [],
      urgentRendererRttMicros: [], memorySamples: [], memoryPhaseErrors: {}, interruptions: [],
    });
  }

  recordReceive(frame: TerminalFrame): FrameReceipt {
    const nowUnixMicros = Date.now() * 1000;
    const nowPerformance = performance.now();
    const sequence = Number(frame.sequence);
    this.state.framesReceived += 1;
    this.state.frontendPendingBytes += frame.payload.byteLength;
    this.state.frontendPendingHighWaterBytes = Math.max(this.state.frontendPendingHighWaterBytes, this.state.frontendPendingBytes);
    pushBounded(this.state.backendToWebViewMicros, {
      sequence,
      micros: Math.max(0, nowUnixMicros - frame.timestampMicros),
    }, MAX_LATENCY_SAMPLES);
    return {
      sequence, payloadBytes: frame.payload.byteLength, creditCost: frame.creditCost,
      receivedAtUnixMicros: nowUnixMicros, receivedAtPerformanceMillis: nowPerformance,
      backendSentAtMicros: frame.timestampMicros,
    };
  }

  get phase(): string { return this.state.phase; }
  get roundIndex(): number { return this.state.roundIndex; }
  get startedAtUnixMillis(): number { return this.state.startedAtUnixMillis; }
  get appliedCreditBytes(): number { return this.state.creditBytes; }
  get complete(): boolean { return this.state.workloadComplete; }

  recordApplied(receipt: FrameReceipt, payload: Uint8Array): void {
    this.hasher.update(payload);
    this.state.framesApplied += 1;
    this.state.payloadBytes += payload.byteLength;
    this.state.frontendPendingBytes = Math.max(0, this.state.frontendPendingBytes - receipt.payloadBytes);
    if (payload.byteLength === 0) this.state.metadataFrames += 1;
    const callbackMicros = Math.max(0, (performance.now() - receipt.receivedAtPerformanceMillis) * 1000);
    if (payload.byteLength === 0) {
      pushBounded(this.state.metadataProcessMicros, { sequence: receipt.sequence, micros: callbackMicros }, MAX_LATENCY_SAMPLES);
    } else {
      pushBounded(this.state.backendToXtermMicros, {
        sequence: receipt.sequence,
        micros: Math.max(0, Date.now() * 1000 - receipt.backendSentAtMicros),
      }, MAX_LATENCY_SAMPLES);
      pushBounded(this.state.xtermCallbackMicros, { sequence: receipt.sequence, micros: callbackMicros }, MAX_LATENCY_SAMPLES);
    }
  }

  recordCreditSent(sequence: bigint, cost: number): void {
    this.state.creditBytes += cost;
    this.state.appliedSequence = Number(sequence);
  }

  recordUrgentRTT(milliseconds: number): void {
    pushBounded(this.state.urgentRendererRttMicros, Math.max(0, milliseconds * 1000), MAX_LATENCY_SAMPLES);
  }

  hasMemoryPhase(phase: MemoryPhase): boolean {
    return this.state.memorySamples.some((sample) => sample.phase === phase);
  }

  hasMemoryPhaseOutcome(phase: MemoryPhase): boolean {
    return this.hasMemoryPhase(phase) || Boolean(this.state.memoryPhaseErrors[phase]);
  }

  hasMemoryPhaseError(phase: MemoryPhase): boolean {
    return Boolean(this.state.memoryPhaseErrors[phase]);
  }

  recordMemoryPhase(phase: MemoryPhase, backend: BackendMemorySample): void {
    if (this.hasMemoryPhase(phase)) throw new Error(`memory phase ${phase} was sampled twice`);
    if (phase !== "start" && !this.hasMemoryPhase("start")) throw new Error(`memory phase ${phase} requires start`);
    if (phase === "complete" && !this.state.workloadComplete) throw new Error("complete memory phase requires completed digest state");
    if (phase === "complete" && !this.hasMemoryPhaseOutcome("steady")) throw new Error("complete memory phase requires steady outcome");
    if (phase === "settled" && this.state.workloadComplete && !this.hasMemoryPhaseOutcome("complete")) {
      throw new Error("completed round cannot settle before complete memory phase");
    }
    const browserMemory = (performance as Performance & { memory?: { usedJSHeapSize?: number } }).memory;
    const jsHeap = Number.isFinite(browserMemory?.usedJSHeapSize) ? browserMemory!.usedJSHeapSize! : null;
    pushBounded(this.state.memorySamples, {
      sampledAtUnixMillis: Date.now(), phase,
      jsHeapBytes: jsHeap, jsHeapAvailable: jsHeap !== null, backend,
    }, MAX_MEMORY_SAMPLES);
    delete this.state.memoryPhaseErrors[phase];
  }

  recordMemoryPhaseError(phase: MemoryPhase, error: unknown): void {
    const message = String(error).slice(0, 256);
    this.state.memoryPhaseErrors[phase] = message || "unknown memory sample error";
  }

  startInterruption(kind: string): string {
    if (kind.length === 0 || kind.length > 64) throw new Error("invalid interruption kind");
    const existingOpen = this.state.interruptions.find((record) => record.endedAtUnixMillis === 0);
    if (existingOpen) throw new Error(`interruption ${existingOpen.correlationId} is already open`);
    if (this.state.interruptions.length >= MAX_INTERRUPTION_RECORDS) {
      const completedIndex = this.state.interruptions.findIndex((record) => record.endedAtUnixMillis > 0);
      if (completedIndex < 0) throw new Error("bounded interruption history is full");
      this.state.interruptions.splice(completedIndex, 1);
    }
    const correlationId = newCorrelationID();
    this.state.interruptions.push({
      correlationId,
      kind,
      startedAtUnixMillis: Date.now(),
      endedAtUnixMillis: 0,
      durationMillis: 0,
      startAcknowledgedSequence: this.state.appliedSequence,
      endAcknowledgedSequence: 0,
    });
    return correlationId;
  }

  findOpenInterruption(kind: string): string | undefined {
    return this.state.interruptions.find((record) => record.kind === kind && record.endedAtUnixMillis === 0)?.correlationId;
  }

  finishInterruption(correlationId: string): void {
    const record = this.state.interruptions.find((item) => item.correlationId === correlationId);
    if (!record || record.endedAtUnixMillis !== 0) throw new Error("interruption correlation is missing or already finished");
    record.endedAtUnixMillis = Date.now();
    record.durationMillis = Math.max(0, record.endedAtUnixMillis - record.startedAtUnixMillis);
    record.endAcknowledgedSequence = this.state.appliedSequence;
  }

  markComplete(): void {
    this.state.workloadComplete = true;
    if (this.state.completedAtUnixMillis === 0) this.state.completedAtUnixMillis = Date.now();
  }

  markStopped(reason: string): void {
    if (reason.length === 0 || reason.length > 64) throw new Error("invalid round stop reason");
    this.state.roundStopReason = reason;
    if (this.state.completedAtUnixMillis === 0) this.state.completedAtUnixMillis = Date.now();
  }

  handoffState(): string {
    const envelope: HandoffEnvelope = {
      version: 2,
      state: structuredClone(this.state),
      hashStateBase64: bytesToBase64(this.hasher.save()),
    };
    return JSON.stringify(envelope);
  }

  async report(expectation: BackendSessionEvidence | undefined, credit: CreditStats): Promise<ClientSessionReport> {
    const clone = await createSHA256();
    clone.load(this.hasher.save());
    const digest = clone.digest("hex") as string;
    const completedAt = this.state.completedAtUnixMillis || Date.now();
    const duration = Math.max(0, completedAt - this.state.startedAtUnixMillis);
    const expectedReady = Boolean(expectation?.expectedPayloadSha256);
    const memorySamples = this.state.memorySamples;
    return {
      ...structuredClone(this.state),
      backend: expectation ? structuredClone(expectation) : null,
      actualPayloadSha256: digest,
      expectedPayloadSha256: expectation?.expectedPayloadSha256 ?? "",
      expectedPayloadBytes: expectation?.expectedPayloadBytes ?? 0,
      expectedCreditBytes: expectation?.expectedCreditBytes ?? 0,
      expectedFrames: expectation?.expectedFrames ?? 0,
      sequenceIntegrity: expectedReady ? this.state.appliedSequence === expectation!.expectedFrames : null,
      byteIntegrity: expectedReady ? this.state.payloadBytes === expectation!.expectedPayloadBytes : null,
      creditIntegrity: expectedReady ? this.state.creditBytes === expectation!.expectedCreditBytes : null,
      digestIntegrity: expectedReady ? digest === expectation!.expectedPayloadSha256 : null,
      durationMillis: duration,
      throughputBytesPerSecond: duration > 0 ? this.state.payloadBytes * 1000 / duration : 0,
      roundObservedSampleMaxima: {
        goHeapAllocBytes: observedMaximum(memorySamples.map((sample) => sample.backend.goHeapAllocBytes)) ?? 0,
        goHeapInuseBytes: observedMaximum(memorySamples.map((sample) => sample.backend.goHeapInuseBytes)) ?? 0,
        processRssBytes: observedMaximum(memorySamples.map((sample) => sample.backend.processRssBytes)),
        jsHeapBytes: observedMaximum(memorySamples.map((sample) => sample.jsHeapBytes)),
      },
      memoryPhaseStatus: {
        start: this.hasMemoryPhase("start") ? "sampled" : this.hasMemoryPhaseError("start") ? "capture-failed" : "missing",
        steady: this.hasMemoryPhase("steady") ? "sampled" : this.hasMemoryPhaseError("steady") ? "capture-failed" : "threshold-not-reached",
        complete: this.hasMemoryPhase("complete") ? "sampled" : this.hasMemoryPhaseError("complete") ? "capture-failed" : "session-not-complete",
        settled: this.hasMemoryPhase("settled") ? "sampled" : this.hasMemoryPhaseError("settled") ? "capture-failed" : "missing",
      },
      credit: { ...credit, acknowledgedSequence: Number(credit.acknowledgedSequence) },
    };
  }
}
