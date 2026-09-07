import type { ClientSessionReport } from "./clientMetrics.js";

const MAX_ROUNDS = 40;
const DEFAULT_MAX_SERIALIZED_BYTES = 8 * 1024 * 1024;
const DEFAULT_MAX_ROUND_BYTES = 240 * 1024;

export interface RoundReport {
  roundIndex: number;
  phase: string;
  startedAtUnixMillis: number;
  completedAtUnixMillis: number;
  settledAtUnixMillis: number;
  settleCondition: string;
  memorySampleProtocol: {
    start: string;
    steady: string;
    complete: string;
    settled: string;
  };
  roundObservedSampleMaxima: {
    goHeapAllocBytes: number;
    goHeapInuseBytes: number;
    processRssBytes: number | null;
    jsHeapBytes: number | null;
  };
  retainedReportAtStart: { rounds: number; serializedBytes: number };
  retainedReportAtEnd: { rounds: number; serializedBytes: number };
  sessions: ClientSessionReport[];
}

export interface ProbeReport {
  formatVersion: 2;
  exportedAtUnixMillis: number;
  bounds: {
    maxRounds: number;
    maxSessionsPerRound: number;
    maxLatencySamplesPerSession: number;
    maxMemorySamplesPerSession: number;
    maxSerializedBytes: number;
    maxSerializedBytesPerRound: number;
    retainedSerializedBytes: number;
    retainedReportMemoryIncludedInProcessRSS: boolean;
    minimumRoundsAtPerRoundCap: number;
  };
  rounds: RoundReport[];
}

export class ReportStore {
  private readonly rounds: Array<{ round: RoundReport; bytes: number }> = [];
  private retainedBytes = 0;

  constructor(
    private readonly maxSerializedBytes = DEFAULT_MAX_SERIALIZED_BYTES,
    private readonly maxRoundBytes = DEFAULT_MAX_ROUND_BYTES,
    private readonly maxRounds = MAX_ROUNDS,
  ) {}

  add(round: RoundReport): void {
    if (round.settledAtUnixMillis <= 0 || round.settledAtUnixMillis < round.completedAtUnixMillis ||
      round.sessions.some((session) => !session.memorySamples.some((sample) => sample.phase === "settled"))) {
      throw new Error("only resource-settled rounds may be archived");
    }
    const bytes = new TextEncoder().encode(JSON.stringify(round)).byteLength;
    if (bytes > this.maxRoundBytes) throw new Error(`round report exceeds ${this.maxRoundBytes} byte cap`);
    const retained = { round: structuredClone(round), bytes };
    this.rounds.push(retained);
    this.retainedBytes += bytes;
    while (this.rounds.length > 0 &&
      (this.rounds.length > this.maxRounds || this.serializedReportBytes() > this.maxSerializedBytes)) {
      const removed = this.rounds.shift();
      if (removed) this.retainedBytes -= removed.bytes;
    }
    if (this.serializedReportBytes() > this.maxSerializedBytes) {
      throw new Error(`report envelope exceeds ${this.maxSerializedBytes} byte cap`);
    }
  }

  build(): ProbeReport {
    const rounds = this.rounds.map((entry) => entry.round).slice(-this.maxRounds);
    const report = this.createReport(rounds);
    if (encodedBytes(report) > this.maxSerializedBytes) {
      throw new Error(`complete report exceeds ${this.maxSerializedBytes} byte cap`);
    }
    return report;
  }

  private createReport(rounds: RoundReport[]): ProbeReport {
    const emptyReportBytes = encodedBytes(this.createReportEnvelope([]));
    return this.createReportEnvelope(rounds, Math.max(
      0,
      Math.min(this.maxRounds, Math.floor((this.maxSerializedBytes - emptyReportBytes) / this.maxRoundBytes)),
    ));
  }

  private createReportEnvelope(rounds: RoundReport[], minimumRoundsAtPerRoundCap = 0): ProbeReport {
    return {
      formatVersion: 2,
      exportedAtUnixMillis: Date.now(),
      bounds: {
        maxRounds: this.maxRounds,
        maxSessionsPerRound: 8,
        maxLatencySamplesPerSession: 24,
        maxMemorySamplesPerSession: 4,
        maxSerializedBytes: this.maxSerializedBytes,
        maxSerializedBytesPerRound: this.maxRoundBytes,
        retainedSerializedBytes: this.retainedBytes,
        retainedReportMemoryIncludedInProcessRSS: true,
        minimumRoundsAtPerRoundCap,
      },
      rounds: structuredClone(rounds),
    };
  }

  private serializedReportBytes(): number {
    return encodedBytes(this.createReport(this.rounds.map((entry) => entry.round)));
  }

  get size(): number {
    return this.rounds.length;
  }

  get retainedSerializedBytes(): number {
    return this.retainedBytes;
  }

  retentionSnapshot(): { rounds: number; serializedBytes: number } {
    return { rounds: this.rounds.length, serializedBytes: this.retainedBytes };
  }
}

function encodedBytes(value: unknown): number {
  return new TextEncoder().encode(JSON.stringify(value)).byteLength;
}
