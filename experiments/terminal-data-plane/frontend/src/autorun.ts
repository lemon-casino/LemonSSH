import type { ClientSessionReport } from "./clientMetrics.js";
import type { RoundReport } from "./reportStore.js";

export interface AutorunConfigShape {
  enabled: boolean;
  workload: string;
  sessionCount: number;
  chunkCount: number;
  hardTimeoutMillis: number;
  conditionTimeoutMillis: number;
  pollIntervalMillis: number;
}

export interface AutorunResultShape {
  formatVersion: number;
  sessionId: string;
  workload: string;
  sessionCount: number;
  chunkCount: number;
  payloadBytes: number;
  creditBytes: number;
  frameCount: number;
  sequenceIntegrity: boolean;
  byteIntegrity: boolean;
  creditIntegrity: boolean;
  digestIntegrity: boolean;
  urgentRendererRttMicros: number;
  generation: number;
  rebindCount: number;
  frontendQueueHighWaterBytes: number;
  backendOutstandingHighWaterBytes: number;
  memoryStart: boolean;
  memorySteady: boolean;
  memoryComplete: boolean;
  memorySettled: boolean;
  totalDurationMillis: number;
}

interface ConditionWaitOptions<T> {
  name: string;
  timeoutMillis: number;
  pollIntervalMillis: number;
  sample: () => Promise<T>;
  accept: (value: T) => boolean;
  now?: () => number;
  delay?: (milliseconds: number) => Promise<void>;
}

export async function waitForCondition<T>(options: ConditionWaitOptions<T>): Promise<T> {
  if (!Number.isFinite(options.timeoutMillis) || options.timeoutMillis <= 0 ||
    !Number.isFinite(options.pollIntervalMillis) || options.pollIntervalMillis <= 0) {
    throw new Error("condition wait bounds are invalid");
  }
  const now = options.now ?? (() => performance.now());
  const delay = options.delay ?? ((milliseconds: number) => new Promise<void>((resolve) => {
    window.setTimeout(resolve, milliseconds);
  }));
  const startedAt = now();
	let expired = false;
	let timeout: ReturnType<typeof globalThis.setTimeout> | undefined;
	const deadline = new Promise<never>((_, reject) => {
		timeout = globalThis.setTimeout(() => {
			expired = true;
			reject(new Error(`${options.name} condition timed out`));
		}, options.timeoutMillis);
	});
	const poll = async (): Promise<T> => {
		for (;;) {
			const value = await options.sample();
			const elapsed = now() - startedAt;
			if (expired || elapsed >= options.timeoutMillis) throw new Error(`${options.name} condition timed out`);
			if (options.accept(value)) return value;
			const remaining = options.timeoutMillis - elapsed;
			if (remaining <= 0) throw new Error(`${options.name} condition timed out`);
			await delay(Math.min(options.pollIntervalMillis, remaining));
		}
	};
	try {
		return await Promise.race([poll(), deadline]);
	} finally {
		if (timeout !== undefined) globalThis.clearTimeout(timeout);
	}
}

export function buildAutorunResult(
  config: AutorunConfigShape,
  round: RoundReport,
  totalDurationMillis: number,
): AutorunResultShape {
  if (!config.enabled || config.workload !== "sustained" || config.sessionCount !== 1 || config.chunkCount !== 1600) {
    throw new Error("autorun config does not describe the canonical smoke workload");
  }
  if (!Number.isFinite(totalDurationMillis) || totalDurationMillis <= 0 || totalDurationMillis > config.hardTimeoutMillis) {
    throw new Error("autorun duration is outside its hard bound");
  }
  if (round.sessions.length !== 1) throw new Error("autorun round must contain exactly one session");
  const report = round.sessions[0]!;
  validateSessionReport(config, report);
  const backend = report.backend!;
  const urgentRendererRttMicros = Math.max(1, Math.round(report.urgentRendererRttMicros[0]!));
  return {
    formatVersion: 1,
    sessionId: report.sessionId,
    workload: report.workload,
    sessionCount: 1,
    chunkCount: config.chunkCount,
    payloadBytes: report.payloadBytes,
    creditBytes: report.creditBytes,
    frameCount: report.framesApplied,
    sequenceIntegrity: report.sequenceIntegrity === true,
    byteIntegrity: report.byteIntegrity === true,
    creditIntegrity: report.creditIntegrity === true,
    digestIntegrity: report.digestIntegrity === true,
    urgentRendererRttMicros,
    generation: backend.generation,
    rebindCount: backend.routeInterruptionCount,
    frontendQueueHighWaterBytes: report.credit.queueHighWaterBytes,
    backendOutstandingHighWaterBytes: backend.backendMaxOutstanding,
    memoryStart: report.memoryPhaseStatus.start === "sampled",
    memorySteady: report.memoryPhaseStatus.steady === "sampled",
    memoryComplete: report.memoryPhaseStatus.complete === "sampled",
    memorySettled: report.memoryPhaseStatus.settled === "sampled",
    totalDurationMillis: Math.round(totalDurationMillis),
  };
}

function validateSessionReport(config: AutorunConfigShape, report: ClientSessionReport): void {
  const backend = report.backend;
  if (!backend || report.workload !== config.workload || !report.workloadComplete || report.roundStopReason !== "completed") {
    throw new Error("autorun report identity or completion state is invalid");
  }
  if (report.sequenceIntegrity !== true || report.byteIntegrity !== true ||
    report.creditIntegrity !== true || report.digestIntegrity !== true) {
    throw new Error("autorun report integrity is incomplete");
  }
  if (report.memoryPhaseStatus.start !== "sampled" || report.memoryPhaseStatus.steady !== "sampled" ||
    report.memoryPhaseStatus.complete !== "sampled" || report.memoryPhaseStatus.settled !== "sampled") {
    throw new Error("autorun report is missing a required memory phase");
  }
  if (report.framesApplied !== config.chunkCount || report.framesReceived !== config.chunkCount ||
    report.payloadBytes !== backend.expectedPayloadBytes || report.creditBytes !== backend.expectedCreditBytes ||
    report.appliedSequence !== backend.expectedFrames || backend.sequence !== backend.expectedFrames ||
    backend.appliedSequence !== backend.expectedFrames || backend.framesAcked !== backend.expectedFrames ||
    backend.framesSent !== backend.expectedFrames || backend.error !== "" || backend.closeStatus !== "stopped") {
    throw new Error("autorun report does not match exact backend counts");
  }
  if (backend.generation !== 2 || backend.routeInterruptionCount !== 1 || backend.drainCount !== 1 ||
    backend.pauseCount < 1 || backend.resumeCount < 1 || backend.urgentCount !== 1 ||
    backend.backendMaxOutstanding <= 0 || backend.preStopOutstandingBytes !== 0) {
    throw new Error("autorun report is missing stall, urgent, resume, or rebind evidence");
  }
  if (report.credit.stallCount < 1 || report.credit.queueHighWaterBytes <= 0 || report.credit.pendingBytes !== 0 ||
    report.urgentRendererRttMicros.length !== 1 || report.urgentRendererRttMicros[0]! <= 0) {
    throw new Error("autorun frontend queue or urgent evidence is invalid");
  }
  if (report.interruptions.length !== 1 || report.interruptions[0]!.kind !== "same-window-rebind" ||
    report.interruptions[0]!.endedAtUnixMillis <= 0) {
    throw new Error("autorun ordered rebind evidence is invalid");
  }
}
