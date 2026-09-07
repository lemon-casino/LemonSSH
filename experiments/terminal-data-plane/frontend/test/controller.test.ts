import { CreditController } from "../src/creditController.js";
import { ClientMetrics } from "../src/clientMetrics.js";
import { ReportStore } from "../src/reportStore.js";
import { cleanupFailedAutorunResources, cleanupPartialResources, prepareWithRollback, releasesCanSettle } from "../src/cleanupController.js";
import { RouteOperationController } from "../src/routeOperationController.js";
import { buildAutorunResult, waitForCondition } from "../src/autorun.js";

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

let clock = 0;
const sent: Array<{ sequence: bigint; cost: number }> = [];
const controller = new CreditController(0n, 1024, (ack) => sent.push(ack), () => clock);
controller.setStalled(true);
controller.complete(2n, 200);
controller.complete(1n, 100);
assert(sent.length === 0, "stall must retain completed credit");
assert(controller.snapshot().pendingBytes === 300, "stall pending bytes");
clock = 25;
controller.setStalled(false);
assert(sent.map((ack) => ack.sequence).join(",") === "1,2", "ACKs flush in sequence order");
assert(controller.snapshot().queueHighWaterBytes === 300, "credit queue high-water");
assert(controller.snapshot().stallDurationMillis === 25, "stall duration");
await controller.waitUntilAcknowledged(2n);

let rejected = false;
try {
  controller.complete(3n, 1025);
} catch {
  rejected = true;
}
assert(rejected, "credit queue rejects costs over the receive window");

const metrics = await ClientMetrics.create("session", "test", "measured", 1);
const memory = (heap: number) => ({
  sampledAtUnixMillis: heap,
  goHeapAllocBytes: heap,
  goHeapInuseBytes: heap + 1,
  serviceLifetimeObservedMaxGoHeapAllocBytes: heap + 2,
  processRssBytes: heap + 3,
  processLifetimePeakRssBytes: heap + 4,
  processRssAvailable: true,
  processRssError: "",
});
metrics.recordMemoryPhase("start", memory(10));
const receipt = metrics.recordReceive({
  kind: 1, generation: 1, sequence: 1n, creditCost: 3, correlation: 0,
  timestampMicros: Date.now() * 1000, payload: new Uint8Array([97, 98, 99]),
});
metrics.recordApplied(receipt, new Uint8Array([97, 98, 99]));
metrics.recordCreditSent(1n, 3);
metrics.recordMemoryPhase("steady", memory(20));
const correlationID = metrics.startInterruption("main-to-popup");
const handoffState = metrics.handoffState();
const restored = await ClientMetrics.create("session", "test", "measured", 1, handoffState);
assert(restored.findOpenInterruption("main-to-popup") === correlationID, "open interruption correlation survives handoff");
restored.finishInterruption(correlationID);
restored.markComplete();
restored.recordMemoryPhase("complete", memory(30));
restored.markStopped("completed");
restored.recordMemoryPhase("settled", memory(40));
const report = await restored.report({
  generation: 1, sequence: 1, appliedSequence: 1,
  payloadBytes: 3, creditBytes: 3, framesSent: 1, framesAcked: 1, framesReplayed: 0,
  backendMaxOutstanding: 3, pauseCount: 0, resumeCount: 0, pauseDurationMillis: 0,
  urgentCount: 0, urgentServiceMicros: 0, drainCount: 0, drainDurationMillis: 0,
  routeInterruptionCount: 0, startedAtUnixMillis: 1, completedAtUnixMillis: 2,
  frameWriteAttempts: 1, preStopOutstandingBytes: 0,
  error: "", closeStatus: "", expectedPayloadBytes: 3, expectedCreditBytes: 3,
  expectedFrames: 1, expectedPayloadSha256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
}, controller.snapshot());
assert(report.digestIntegrity === true && report.byteIntegrity === true, "incremental SHA-256 integrity");
assert(report.memorySamples.map((sample) => sample.phase).join(",") === "start,steady,complete,settled", "deterministic memory phases");
assert(report.roundObservedSampleMaxima.goHeapAllocBytes === 40, "round maximum derives from bounded samples");
assert(report.interruptions.length === 1 && report.interruptions[0]?.correlationId === correlationID &&
  report.interruptions[0]?.endedAtUnixMillis !== 0 && report.interruptions[0]?.startAcknowledgedSequence === 1 &&
  report.interruptions[0]?.endAcknowledgedSequence === 1, "popup move remains one correlated interruption");

const malformed = JSON.parse(handoffState) as { state: { interruptions: Array<{ correlationId: string }> } };
malformed.state.interruptions[0]!.correlationId = "invalid";
let malformedRejected = false;
try {
  await ClientMetrics.create("session", "test", "measured", 1, JSON.stringify(malformed));
} catch {
  malformedRejected = true;
}
assert(malformedRejected, "invalid interruption handoff state is rejected");

const missingComplete = JSON.parse(handoffState) as {
  state: { workloadComplete: boolean; completedAtUnixMillis: number; memorySamples: Array<{ phase: string }>; memoryPhaseErrors: Record<string, string> };
};
missingComplete.state.workloadComplete = true;
missingComplete.state.completedAtUnixMillis = Date.now();
missingComplete.state.memorySamples = missingComplete.state.memorySamples.filter((sample) => sample.phase !== "complete");
delete missingComplete.state.memoryPhaseErrors.complete;
let missingCompleteRejected = false;
try {
  await ClientMetrics.create("session", "test", "measured", 1, JSON.stringify(missingComplete));
} catch {
  missingCompleteRejected = true;
}
assert(missingCompleteRejected, "completed restore state without complete memory outcome is rejected");

const stoppedMetrics = await ClientMetrics.create("stopped-session", "test", "measured", 2);
stoppedMetrics.recordMemoryPhase("start", memory(11));
stoppedMetrics.markStopped("user-stop");
stoppedMetrics.recordMemoryPhase("settled", memory(12));
const stoppedReport = await stoppedMetrics.report(undefined, controller.snapshot());
assert(stoppedReport.memoryPhaseStatus.start === "sampled" &&
  stoppedReport.memoryPhaseStatus.steady === "threshold-not-reached" &&
  stoppedReport.memoryPhaseStatus.complete === "session-not-complete" &&
  stoppedReport.memoryPhaseStatus.settled === "sampled", "user-stop phase omissions are explicit without fabricated samples");

const store = new ReportStore();
let unsettledRejected = false;
try {
  store.add({
    roundIndex: 0, phase: "measured", startedAtUnixMillis: 0, completedAtUnixMillis: 1,
    settledAtUnixMillis: 0, settleCondition: "not settled",
    memorySampleProtocol: { start: "start", steady: "steady", complete: "complete", settled: "settled" },
    roundObservedSampleMaxima: { goHeapAllocBytes: 0, goHeapInuseBytes: 0, processRssBytes: null, jsHeapBytes: null },
    retainedReportAtStart: { rounds: 0, serializedBytes: 0 }, retainedReportAtEnd: { rounds: 0, serializedBytes: 0 },
    sessions: [],
  });
} catch {
  unsettledRejected = true;
}
assert(unsettledRejected, "unsettled rounds cannot be archived");
store.add({
  roundIndex: -1, phase: "measured", startedAtUnixMillis: 0, completedAtUnixMillis: 1,
  settledAtUnixMillis: 2, settleCondition: "test settled",
  memorySampleProtocol: { start: "start", steady: "steady", complete: "complete", settled: "settled" },
  roundObservedSampleMaxima: report.roundObservedSampleMaxima,
  retainedReportAtStart: { rounds: 0, serializedBytes: 0 }, retainedReportAtEnd: { rounds: 0, serializedBytes: 0 },
  sessions: [report],
});
for (let index = 0; index < 45; index += 1) {
  store.add({
    roundIndex: index,
    phase: "measured",
    startedAtUnixMillis: index,
    completedAtUnixMillis: index + 1,
    settledAtUnixMillis: index + 2,
    settleCondition: "test settled",
    memorySampleProtocol: { start: "start", steady: "steady", complete: "complete", settled: "settled" },
    roundObservedSampleMaxima: { goHeapAllocBytes: 0, goHeapInuseBytes: 0, processRssBytes: null, jsHeapBytes: null },
    retainedReportAtStart: { rounds: 0, serializedBytes: 0 }, retainedReportAtEnd: { rounds: 0, serializedBytes: 0 },
    sessions: [],
  });
}
assert(store.size === 40 && store.build().rounds[0]?.roundIndex === 5, "round history remains bounded");
assert(store.build().bounds.minimumRoundsAtPerRoundCap >= 33, "report cap retains 3 warmups plus 30 measured rounds");

const byteStore = new ReportStore(2_200, 2_000, 40);
for (let index = 0; index < 10; index += 1) {
  byteStore.add({
    roundIndex: index, phase: "measured", startedAtUnixMillis: index, completedAtUnixMillis: index + 1,
    settledAtUnixMillis: index + 2, settleCondition: "test settled",
    memorySampleProtocol: { start: "start", steady: "steady", complete: "complete", settled: "settled" },
    roundObservedSampleMaxima: { goHeapAllocBytes: 0, goHeapInuseBytes: 0, processRssBytes: null, jsHeapBytes: null },
    retainedReportAtStart: { rounds: 0, serializedBytes: 0 }, retainedReportAtEnd: { rounds: 0, serializedBytes: 0 },
    sessions: [],
  });
}
assert(byteStore.retainedSerializedBytes <= 2_200 && byteStore.size < 10, "serialized byte cap evicts oldest rounds");
assert(new TextEncoder().encode(JSON.stringify(byteStore.build())).byteLength <= 2_200,
  "complete exported report, including envelope, stays within serialized byte cap");

const stoppedIDs: string[] = [];
const disposedResources: number[] = [];
const cleanupError = await cleanupPartialResources(
  ["one", "two", "three"],
  [1, 2],
  async (sessionID) => {
    stoppedIDs.push(sessionID);
    if (sessionID === "two") throw new Error("stop failed");
  },
  (resource) => disposedResources.push(resource),
  new Error("construction failed"),
);
assert(stoppedIDs.join(",") === "one,two,three" && disposedResources.join(",") === "1,2" &&
  cleanupError.errors.length === 2, "partial cleanup attempts every backend stop and successful view disposal");

const autorunDisposals: number[] = [];
const autorunStops: string[] = [];
let autorunFailureReports = 0;
const originalAutorunFailure = new Error("original autorun failure");
const autorunCleanupError = await cleanupFailedAutorunResources(
  ["first", "second"],
  [1, 2, 3],
  async (sessionID) => {
    autorunStops.push(sessionID);
    if (sessionID === "first") throw new Error("first stop failed");
  },
  (resource) => {
    autorunDisposals.push(resource);
    if (resource === 1) throw new Error("first dispose failed");
  },
  async () => { autorunFailureReports += 1; },
  originalAutorunFailure,
);
assert(autorunDisposals.join(",") === "1,2,3" && autorunStops.join(",") === "first,second" &&
  autorunFailureReports === 1 && autorunCleanupError.errors[0] === originalAutorunFailure &&
  autorunCleanupError.errors.length === 3,
"autorun cleanup attempts every dispose and stop before reporting while preserving original failure");
const boundedAutorunCleanupError = await cleanupFailedAutorunResources(
  [], Array.from({ length: 20 }, (_, index) => index), async () => undefined,
  () => { throw new Error("dispose failed"); }, async () => undefined, originalAutorunFailure,
);
assert(boundedAutorunCleanupError.errors.length === 16 && boundedAutorunCleanupError.errors[0] === originalAutorunFailure,
  "autorun cleanup error aggregation remains bounded with original failure first");

const preparedItems: number[] = [];
const recoveredItems: number[] = [];
let rollbackRejected = false;
try {
  await prepareWithRollback(
    [1, 2, 3],
    async (item) => {
      if (item === 2) throw new Error("prepare failed");
      preparedItems.push(item);
    },
    async (item) => { recoveredItems.push(item); },
    () => { throw new Error("persist should not run"); },
  );
} catch {
  rollbackRejected = true;
}
assert(rollbackRejected && preparedItems.join(",") === "1" && recoveredItems.join(",") === "1",
  "partial reload preparation rebinds every successfully drained session");

const persistenceRecovered: number[] = [];
try {
  await prepareWithRollback(
    [4, 5],
    async () => undefined,
    async (item) => { persistenceRecovered.push(item); },
    () => { throw new Error("storage full"); },
  );
} catch {
  // Expected after both prepared resources are recovered.
}
assert(persistenceRecovered.join(",") === "4,5", "storage failure recovers every prepared session");

const routeOperation = new RouteOperationController();
routeOperation.acquire("popup-transfer");
let finalizeBlocked = false;
let stopBlocked = false;
try { routeOperation.assertAvailable("finalizeRound"); } catch { finalizeBlocked = true; }
try { routeOperation.assertAvailable("Stop"); } catch { stopBlocked = true; }
assert(finalizeBlocked && stopBlocked, "finalize and stop are blocked before popup drain begins");
routeOperation.release("popup-transfer");
routeOperation.assertAvailable("Run");
routeOperation.acquire("autorun");
routeOperation.assertOwned("autorun", "autorun finalize");
let wrongAutorunOwnerRejected = false;
try { routeOperation.assertOwned("reload", "autorun finalize"); } catch { wrongAutorunOwnerRejected = true; }
assert(wrongAutorunOwnerRejected, "autorun finalization requires its dedicated route-operation owner");
routeOperation.release("autorun");
assert(releasesCanSettle([{ stopSucceeded: true, errors: [new Error("telemetry failed")] }]),
  "telemetry failure does not block settled evidence after confirmed stop");
assert(!releasesCanSettle([{ stopSucceeded: false, errors: [new Error("stop failed")] }]),
  "StopSession failure blocks settled evidence and archival");

let waitClock = 0;
let waitSamples = 0;
const conditionValue = await waitForCondition({
  name: "eventual condition", timeoutMillis: 10, pollIntervalMillis: 2,
  sample: async () => ++waitSamples,
  accept: (value) => value === 3,
  now: () => waitClock,
  delay: async (milliseconds) => { waitClock += milliseconds; },
});
assert(conditionValue === 3 && waitClock === 4, "condition polling resolves on observed state without fixed protocol sleep");
let conditionTimedOut = false;
waitClock = 0;
try {
  await waitForCondition({
    name: "never", timeoutMillis: 5, pollIntervalMillis: 2,
    sample: async () => false,
    accept: Boolean,
    now: () => waitClock,
    delay: async (milliseconds) => { waitClock += milliseconds; },
  });
} catch (error) {
  conditionTimedOut = String(error).includes("never condition timed out");
}
assert(conditionTimedOut && waitClock === 5, "condition polling fails at exact timeout bound");
let hungSampleTimedOut = false;
try {
  await waitForCondition({
    name: "hung sample", timeoutMillis: 5, pollIntervalMillis: 1,
    sample: () => new Promise<boolean>(() => undefined),
    accept: Boolean,
  });
} catch (error) {
  hungSampleTimedOut = String(error).includes("hung sample condition timed out");
}
assert(hungSampleTimedOut, "condition deadline rejects a hung sample operation");
let delayedTrueAccepted = false;
let delayedTrueTimedOut = false;
waitClock = 0;
try {
  await waitForCondition({
    name: "late true", timeoutMillis: 10, pollIntervalMillis: 1,
    sample: async () => {
      waitClock = 11;
      return true;
    },
    accept: (value) => {
      delayedTrueAccepted = value;
      return value;
    },
    now: () => waitClock,
    delay: async (milliseconds) => { waitClock += milliseconds; },
  });
} catch (error) {
  delayedTrueTimedOut = String(error).includes("late true condition timed out");
}
assert(delayedTrueTimedOut && !delayedTrueAccepted, "condition rejects a true sample observed after its monotonic deadline");

const autorunSession = structuredClone(report);
autorunSession.workload = "sustained";
autorunSession.workloadComplete = true;
autorunSession.roundStopReason = "completed";
autorunSession.framesReceived = 1600;
autorunSession.framesApplied = 1600;
autorunSession.appliedSequence = 1600;
autorunSession.urgentRendererRttMicros = [100];
autorunSession.interruptions[0]!.kind = "same-window-rebind";
autorunSession.backend!.generation = 2;
autorunSession.backend!.sequence = 1600;
autorunSession.backend!.appliedSequence = 1600;
autorunSession.backend!.framesSent = 1600;
autorunSession.backend!.framesAcked = 1600;
autorunSession.backend!.expectedFrames = 1600;
autorunSession.backend!.pauseCount = 1;
autorunSession.backend!.resumeCount = 1;
autorunSession.backend!.urgentCount = 1;
autorunSession.backend!.drainCount = 1;
autorunSession.backend!.routeInterruptionCount = 1;
autorunSession.backend!.closeStatus = "stopped";
const autorunRound = {
  roundIndex: 1, phase: "autorun", startedAtUnixMillis: 1, completedAtUnixMillis: 2,
  settledAtUnixMillis: 3, settleCondition: "settled",
  memorySampleProtocol: { start: "start", steady: "steady", complete: "complete", settled: "settled" },
  roundObservedSampleMaxima: report.roundObservedSampleMaxima,
  retainedReportAtStart: { rounds: 0, serializedBytes: 0 }, retainedReportAtEnd: { rounds: 0, serializedBytes: 0 },
  sessions: [autorunSession],
};
const autorunResult = buildAutorunResult({
  enabled: true, workload: "sustained", sessionCount: 1, chunkCount: 1600,
  hardTimeoutMillis: 90_000, conditionTimeoutMillis: 15_000, pollIntervalMillis: 25,
}, autorunRound, 1_000);
assert(autorunResult.generation === 2 && autorunResult.rebindCount === 1 &&
  autorunResult.memorySettled && autorunResult.digestIntegrity, "autorun result requires complete bounded evidence");
autorunSession.memoryPhaseStatus.steady = "capture-failed";
let missingAutorunPhaseRejected = false;
try {
  buildAutorunResult({
    enabled: true, workload: "sustained", sessionCount: 1, chunkCount: 1600,
    hardTimeoutMillis: 90_000, conditionTimeoutMillis: 15_000, pollIntervalMillis: 25,
  }, autorunRound, 1_000);
} catch {
  missingAutorunPhaseRejected = true;
}
assert(missingAutorunPhaseRejected, "autorun result rejects missing required memory phase");

console.log("controller tests passed");
