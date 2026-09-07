import { Terminal } from "@xterm/xterm";
import { WebglAddon } from "@xterm/addon-webgl";
import {
  ProbeService,
  type AutorunConfig,
  type MemorySnapshot,
  type PopupHandoffInfo,
  type RouteBootstrap,
  type SessionMetrics,
  type WorkloadDescriptor,
} from "../bindings/github.com/binaricat/netcatty/experiments/terminal-data-plane";
import { buildAutorunResult, waitForCondition } from "./autorun";
import {
  ClientMetrics,
  type BackendMemorySample,
  type BackendSessionEvidence,
  type ClientSessionReport,
  type MemoryPhase,
} from "./clientMetrics";
import {
	cleanupFailedAutorunResources,
  cleanupPartialResources,
  prepareWithRollback,
  releasesCanSettle,
  type ResourceReleaseResult,
} from "./cleanupController";
import { CreditController } from "./creditController";
import type { CreditStats } from "./creditController";
import { decodeFrame, encodeFrame, FrameKind, type TerminalFrame, UrgentAckLedger } from "./protocol";
import { ReportStore, type RoundReport } from "./reportStore";
import { RouteOperationController, type RouteOperation } from "./routeOperationController";
import "./style.css";

const EMPTY_PAYLOAD = new Uint8Array();
const ACTIVE_STORAGE_KEY = "netcatty-terminal-data-plane-active-v2";
const DRAIN_TIMEOUT_MILLIS = 10_000;
const ROUND_SETTLE_DELAY_MILLIS = 250;
const ROUND_SETTLE_CONDITION = "backend StopSession resolved, data/urgent sockets closed, xterm disposed, then 250ms elapsed";

interface StoredSession {
  sessionId: string;
  workload: string;
  phase: string;
  roundIndex: number;
  lastApplied: string;
  clientState?: string;
}

interface UrgentWaiter {
  startedAt: number;
  generation: number;
  sequence: bigint;
  correlation: number;
  resolve: (rtt: number) => void;
}

interface DrainWaiter {
  correlation: number;
  markerSequence?: bigint;
  resolve: () => void;
  reject: (error: Error) => void;
  timeout: number;
}

class SessionView {
  readonly sessionId: string;
  readonly workload: string;
  readonly terminal: Terminal;
  readonly panel: HTMLElement;
  readonly phase: string;
  readonly roundIndex: number;
  readonly roundStartedAt: number;
  private readonly host: HTMLElement;
  private readonly stateLabel: HTMLElement;
  private readonly clientMetrics: ClientMetrics;
  private dataSocket?: WebSocket;
  private urgentSocket?: WebSocket;
  private bootstrap?: RouteBootstrap;
  private webglAddon?: WebglAddon;
  private credit?: CreditController;
  private retiredCredit = {
    queueHighWaterBytes: 0,
    queueHighWaterRecords: 0,
    stallCount: 0,
    stallDurationMillis: 0,
  };
  private latestBackend?: SessionMetrics;
  private disposed = false;
  private creditStalled = false;
  private initialApplied: bigint;
  private lastReceived: bigint;
  private nextDrainCorrelation = 1;
  private urgentWaiters = new Map<number, UrgentWaiter>();
  private urgentAckLedger = new UrgentAckLedger();
  private drainWaiter?: DrainWaiter;
  private expectedCreditBytes = 0;
  private steadyRequested = false;
  private steadyCapture?: Promise<void>;
  private completeCapture?: Promise<void>;
  lastUrgentRTT = 0;

  private constructor(stored: StoredSession, clientMetrics: ClientMetrics) {
    this.sessionId = stored.sessionId;
    this.workload = stored.workload;
    this.phase = clientMetrics.phase;
    this.roundIndex = clientMetrics.roundIndex;
    this.roundStartedAt = clientMetrics.startedAtUnixMillis;
    this.initialApplied = BigInt(stored.lastApplied);
    this.lastReceived = this.initialApplied;
    this.clientMetrics = clientMetrics;
    this.panel = document.createElement("article");
    this.panel.className = "terminal-panel";
    const header = document.createElement("header");
    const name = document.createElement("span");
    name.textContent = `${this.workload} / ${this.phase} ${this.roundIndex}`;
    this.stateLabel = document.createElement("span");
    this.stateLabel.textContent = "binding";
    header.append(name, this.stateLabel);
    this.host = document.createElement("div");
    this.host.className = "terminal-host";
    this.panel.append(header, this.host);

    this.terminal = new Terminal({
      convertEol: true,
      cursorBlink: false,
      disableStdin: true,
      scrollback: 2_000,
      theme: { background: "#0d0f11", foreground: "#dce2e6", cursor: "#69d69b" },
    });
    this.terminal.open(this.host);
    try {
      this.webglAddon = new WebglAddon();
      this.webglAddon.onContextLoss(() => this.webglAddon?.dispose());
      this.terminal.loadAddon(this.webglAddon);
    } catch {
      this.webglAddon = undefined;
    }
    queueMicrotask(() => this.fit());
  }

  static async create(stored: StoredSession): Promise<SessionView> {
    const metrics = await ClientMetrics.create(
      stored.sessionId,
      stored.workload,
      stored.phase,
      stored.roundIndex,
      stored.clientState,
    );
    return new SessionView(stored, metrics);
  }

  get lastApplied(): bigint {
    return this.credit?.acknowledgedSequence ?? this.initialApplied;
  }

  get stalled(): boolean {
    return this.creditStalled;
  }

  get complete(): boolean {
    return this.clientMetrics.complete;
  }

  get routeReady(): boolean {
    return this.dataSocket?.readyState === WebSocket.OPEN && this.urgentSocket?.readyState === WebSocket.OPEN;
  }

  creditSnapshot(): CreditStats {
    if (!this.credit) throw new Error("credit controller is not ready");
		const current = this.credit.snapshot();
		return {
			...current,
			queueHighWaterBytes: Math.max(current.queueHighWaterBytes, this.retiredCredit.queueHighWaterBytes),
			queueHighWaterRecords: Math.max(current.queueHighWaterRecords, this.retiredCredit.queueHighWaterRecords),
			stallCount: current.stallCount + this.retiredCredit.stallCount,
			stallDurationMillis: current.stallDurationMillis + this.retiredCredit.stallDurationMillis,
		};
  }

  hasMemoryPhase(phase: MemoryPhase): boolean {
    return this.clientMetrics.hasMemoryPhase(phase);
  }

  async bind(providedBootstrap?: RouteBootstrap): Promise<void> {
    const applied = this.lastApplied;
    const replacement = providedBootstrap ?? await ProbeService.ResumeSession(this.sessionId, Number(applied));
		this.retireCreditStats();
    const oldData = this.dataSocket;
    const oldUrgent = this.urgentSocket;
    this.dataSocket = undefined;
    this.urgentSocket = undefined;
    oldData?.close();
    oldUrgent?.close();
    this.bootstrap = replacement;
    this.urgentAckLedger = new UrgentAckLedger();
    this.initialApplied = applied;
    this.lastReceived = applied;
    this.creditStalled = false;
    this.credit = new CreditController(applied, replacement.windowBytes, (ack) => {
      if (!this.bootstrap || this.bootstrap.generation !== replacement.generation) return;
      this.sendData({
        kind: FrameKind.Credit,
        generation: replacement.generation,
        sequence: ack.sequence,
        creditCost: ack.cost,
        correlation: 0,
        timestampMicros: 0,
        payload: EMPTY_PAYLOAD,
      });
      this.clientMetrics.recordCreditSent(ack.sequence, ack.cost);
      this.maybeCaptureSteady();
    });

    const dataSocket = await this.openSocket(replacement.dataUrl, replacement.dataToken);
    let urgentSocket: WebSocket;
    try {
      urgentSocket = await this.openSocket(replacement.urgentUrl, replacement.urgentToken);
    } catch (error) {
      dataSocket.close();
      throw error;
    }
    if (this.disposed) {
      dataSocket.close();
      urgentSocket.close();
      return;
    }
    this.dataSocket = dataSocket;
    this.urgentSocket = urgentSocket;
    dataSocket.binaryType = "arraybuffer";
    urgentSocket.binaryType = "arraybuffer";
    dataSocket.addEventListener("message", (event) => this.onData(dataSocket, event));
    urgentSocket.addEventListener("message", (event) => this.onUrgent(urgentSocket, event));
    dataSocket.addEventListener("close", () => {
      if (this.dataSocket === dataSocket) this.stateLabel.textContent = "data closed";
    });
    urgentSocket.addEventListener("close", () => {
      if (this.urgentSocket === urgentSocket) this.rejectUrgentWaiters();
    });
    this.sendData({
      kind: FrameKind.Credit,
      generation: replacement.generation,
      sequence: applied,
      creditCost: replacement.windowBytes,
      correlation: 0,
      timestampMicros: 0,
      payload: EMPTY_PAYLOAD,
    });
    this.stateLabel.textContent = `gen ${replacement.generation} / seq ${applied}`;
  }

  async rebind(): Promise<void> {
    const correlationID = this.clientMetrics.startInterruption("same-window-rebind");
    this.stateLabel.textContent = "draining";
    try {
      await this.drain();
      this.stateLabel.textContent = "rebinding";
      await this.bind();
    } finally {
      this.clientMetrics.finishInterruption(correlationID);
    }
  }

  async prepareHandoff(kind: string): Promise<string> {
    this.clientMetrics.startInterruption(kind);
    let drained = false;
    try {
      await this.drain();
      drained = true;
      if (this.clientMetrics.complete) {
        await this.waitForCompleteSample();
      }
    } catch (error) {
      const errors: unknown[] = [error];
      if (drained) {
        try {
          await this.bind();
          await ProbeService.StartSession(this.sessionId);
        } catch (recoveryError) {
          errors.push(recoveryError);
        }
      }
      this.finishOpenInterruption(kind);
      throw new AggregateError(errors, `${kind} preparation failed after source recovery`);
    }
    return this.clientMetrics.handoffState();
  }

  finishOpenInterruption(kind: string): void {
    const correlationID = this.clientMetrics.findOpenInterruption(kind);
    if (!correlationID) throw new Error(`open interruption ${kind} was not found`);
    this.clientMetrics.finishInterruption(correlationID);
  }

  async recoverPreparedHandoff(kind: string): Promise<void> {
    await this.bind();
    await ProbeService.StartSession(this.sessionId);
    this.finishOpenInterruption(kind);
  }

  async sendInterrupt(): Promise<number> {
    if (!this.bootstrap || !this.urgentSocket || this.urgentSocket.readyState !== WebSocket.OPEN) {
      throw new Error("urgent route is not open");
    }
    const correlation = this.urgentAckLedger.issue(this.bootstrap.generation, this.lastApplied);
    const frame: TerminalFrame = {
      kind: FrameKind.Urgent,
      generation: this.bootstrap.generation,
      sequence: this.lastApplied,
      creditCost: 0,
      correlation,
      timestampMicros: 0,
      payload: new Uint8Array([3]),
    };
    const promise = new Promise<number>((resolve, reject) => {
      const timeout = window.setTimeout(() => {
        this.urgentWaiters.delete(correlation);
        this.urgentAckLedger.expire(correlation);
        reject(new Error("urgent ACK timed out"));
      }, 3_000);
      this.urgentWaiters.set(correlation, {
        startedAt: performance.now(),
        generation: this.bootstrap!.generation,
        sequence: this.lastApplied,
        correlation,
        resolve: (rtt) => {
          clearTimeout(timeout);
          resolve(rtt);
        },
      });
    });
    this.urgentSocket.send(encodeFrame(frame));
    return promise;
  }

  setCreditStalled(stalled: boolean): void {
    if (!this.credit) throw new Error("credit controller is not ready");
    this.credit.setStalled(stalled);
    this.creditStalled = stalled;
    this.stateLabel.textContent = stalled ? "credit stalled" : `credit resumed / seq ${this.lastApplied}`;
  }

  fit(): void {
    if (!this.disposed) {
      const cols = Math.max(20, Math.floor((this.host.clientWidth - 12) / 8.5));
      const rows = Math.max(5, Math.floor((this.host.clientHeight - 12) / 17));
      if (cols !== this.terminal.cols || rows !== this.terminal.rows) this.terminal.resize(cols, rows);
    }
  }

  updateMetrics(metrics: SessionMetrics): void {
    this.latestBackend = metrics;
    if (this.expectedCreditBytes === 0) this.expectedCreditBytes = metrics.expectedCreditBytes;
    const phase = metrics.complete ? "complete" : this.creditStalled ? "stalled" : metrics.error ? "error" : metrics.running ? "running" : "idle";
    const urgent = this.lastUrgentRTT > 0 ? ` / ^C ${this.lastUrgentRTT.toFixed(1)} ms` : "";
    const queued = this.credit?.snapshot().pendingBytes ?? 0;
    this.stateLabel.textContent = `${phase} / g${metrics.generation} s${metrics.appliedSequence} / q ${formatBytes(queued)}${urgent}`;
  }

  resumeBackendMetrics(metrics: SessionMetrics): void {
    this.latestBackend = metrics;
    this.expectedCreditBytes = metrics.expectedCreditBytes;
  }

  recordStartSample(metrics: SessionMetrics, memory: MemorySnapshot): void {
    this.latestBackend = metrics;
    this.expectedCreditBytes = metrics.expectedCreditBytes;
    this.recordMemoryPhase("start", memory);
  }

  async waitForCompleteSample(): Promise<void> {
    if (!this.clientMetrics.complete || this.clientMetrics.hasMemoryPhase("complete")) return;
    await this.ensureCompleteCapture();
  }

  async releaseResources(reason: string): Promise<ResourceReleaseResult> {
    const errors: unknown[] = [];
    let stopSucceeded = true;
    try {
      await this.steadyCapture;
    } catch (error) {
      errors.push(error);
    }
    try {
      await this.waitForCompleteSample();
    } catch (error) {
      errors.push(error);
    }
    try {
      await ProbeService.StopSession(this.sessionId);
    } catch (error) {
      stopSucceeded = false;
      errors.push(error);
    } finally {
      this.clientMetrics.markStopped(reason);
      this.dispose();
    }
    return { stopSucceeded, errors };
  }

  recordSettledSample(metrics: SessionMetrics | undefined, memory: MemorySnapshot): void {
    if (metrics) this.latestBackend = metrics;
    this.recordMemoryPhase("settled", memory);
  }

  async report(): Promise<ClientSessionReport> {
    const expectation: BackendSessionEvidence | undefined = this.latestBackend ? {
      generation: this.latestBackend.generation,
      sequence: this.latestBackend.sequence,
      appliedSequence: this.latestBackend.appliedSequence,
      payloadBytes: this.latestBackend.payloadBytes,
      creditBytes: this.latestBackend.creditBytes,
      framesSent: this.latestBackend.framesSent,
      framesAcked: this.latestBackend.framesAcked,
      framesReplayed: this.latestBackend.framesReplayed,
      backendMaxOutstanding: this.latestBackend.backendMaxOutstanding,
      pauseCount: this.latestBackend.pauseCount,
      resumeCount: this.latestBackend.resumeCount,
      pauseDurationMillis: this.latestBackend.pauseDurationMillis,
      urgentCount: this.latestBackend.urgentCount,
      urgentServiceMicros: this.latestBackend.urgentServiceMicros,
      drainCount: this.latestBackend.drainCount,
      drainDurationMillis: this.latestBackend.drainDurationMillis,
      routeInterruptionCount: this.latestBackend.routeInterruptionCount,
      frameWriteAttempts: this.latestBackend.frameWriteAttempts,
      preStopOutstandingBytes: this.latestBackend.preStopOutstandingBytes,
      startedAtUnixMillis: this.latestBackend.startedAtUnixMillis,
      completedAtUnixMillis: this.latestBackend.completedAtUnixMillis,
      error: this.latestBackend.error,
      closeStatus: this.latestBackend.closeStatus,
      expectedPayloadBytes: this.latestBackend.expectedPayloadBytes,
      expectedCreditBytes: this.latestBackend.expectedCreditBytes,
      expectedFrames: this.latestBackend.expectedFrames,
      expectedPayloadSha256: this.latestBackend.expectedPayloadSha256,
    } : undefined;
    return this.clientMetrics.report(expectation, this.credit ? this.creditSnapshot() : {
      acknowledgedSequence: this.initialApplied,
      pendingBytes: 0,
      pendingRecords: 0,
      queueHighWaterBytes: 0,
      queueHighWaterRecords: 0,
      stallCount: 0,
      stallDurationMillis: 0,
      stalled: false,
    });
  }

  stored(): StoredSession {
    return {
      sessionId: this.sessionId,
      workload: this.workload,
      phase: this.phase,
      roundIndex: this.roundIndex,
      lastApplied: this.lastApplied.toString(),
      clientState: this.clientMetrics.handoffState(),
    };
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.closeSockets();
    this.rejectUrgentWaiters();
    if (this.drainWaiter) {
      clearTimeout(this.drainWaiter.timeout);
      this.drainWaiter.reject(new Error("session disposed during drain"));
      this.drainWaiter = undefined;
    }
    this.webglAddon?.dispose();
    this.terminal.dispose();
    this.panel.remove();
  }

  private drain(): Promise<void> {
    if (!this.bootstrap || !this.credit || !this.dataSocket || this.dataSocket.readyState !== WebSocket.OPEN) {
      return Promise.reject(new Error("data route is not open for drain"));
    }
    if (this.drainWaiter) return Promise.reject(new Error("route drain is already active"));
    if (this.creditStalled) this.setCreditStalled(false);
    if (this.nextDrainCorrelation > 0xffff_ffff) return Promise.reject(new Error("drain correlation space exhausted"));
    const correlation = this.nextDrainCorrelation++;
    const promise = new Promise<void>((resolve, reject) => {
      const timeout = window.setTimeout(() => {
        if (this.drainWaiter?.correlation === correlation) this.drainWaiter = undefined;
        reject(new Error("ordered route drain timed out"));
      }, DRAIN_TIMEOUT_MILLIS);
      this.drainWaiter = { correlation, resolve, reject, timeout };
    });
    this.sendData({
      kind: FrameKind.DrainRequest,
      generation: this.bootstrap.generation,
      sequence: this.lastReceived,
      creditCost: 0,
      correlation,
      timestampMicros: 0,
      payload: EMPTY_PAYLOAD,
    });
    return promise;
  }

	private retireCreditStats(): void {
		if (!this.credit) return;
		const current = this.credit.snapshot();
		this.retiredCredit.queueHighWaterBytes = Math.max(this.retiredCredit.queueHighWaterBytes, current.queueHighWaterBytes);
		this.retiredCredit.queueHighWaterRecords = Math.max(this.retiredCredit.queueHighWaterRecords, current.queueHighWaterRecords);
		this.retiredCredit.stallCount += current.stallCount;
		this.retiredCredit.stallDurationMillis += current.stallDurationMillis;
	}

  private openSocket(url: string, token: string): Promise<WebSocket> {
    return new Promise((resolve, reject) => {
      const socket = new WebSocket(url, ["netcatty-terminal-v1", `route.${token}`]);
      const onError = () => reject(new Error(`WebSocket handshake failed for ${url}`));
      socket.addEventListener("error", onError, { once: true });
      socket.addEventListener("open", () => {
        socket.removeEventListener("error", onError);
        resolve(socket);
      }, { once: true });
    });
  }

  private onData(socket: WebSocket, event: MessageEvent): void {
    if (socket !== this.dataSocket) return;
    if (!(event.data instanceof ArrayBuffer) || !this.bootstrap || !this.credit) {
      this.failProtocol("non-binary data frame");
      return;
    }
    let frame: TerminalFrame;
    try {
      frame = decodeFrame(event.data);
    } catch (error) {
      this.failProtocol(String(error));
      return;
    }
    if (frame.generation !== this.bootstrap.generation) {
      this.failProtocol("stale output generation");
      return;
    }
    if (frame.kind === FrameKind.DrainMarker) {
      if (!this.drainWaiter || frame.correlation !== this.drainWaiter.correlation || frame.sequence !== this.lastReceived ||
        frame.creditCost !== 0 || frame.payload.byteLength !== 0) {
        this.failProtocol("invalid ordered drain marker");
        return;
      }
      this.drainWaiter.markerSequence = frame.sequence;
      void this.credit.waitUntilAcknowledged(frame.sequence).catch((error) => this.failProtocol(String(error)));
      return;
    }
    if (frame.kind === FrameKind.DrainReady) {
      const waiter = this.drainWaiter;
      if (!waiter || waiter.markerSequence !== frame.sequence || frame.correlation !== waiter.correlation ||
        frame.creditCost !== 0 || frame.payload.byteLength !== 0 || this.credit.acknowledgedSequence !== frame.sequence) {
        this.failProtocol("invalid ordered drain completion");
        return;
      }
      clearTimeout(waiter.timeout);
      this.drainWaiter = undefined;
      waiter.resolve();
      return;
    }
    if (frame.kind === FrameKind.Complete) {
      if (frame.sequence !== this.lastApplied || frame.creditCost !== 0 || frame.correlation !== 0 || frame.payload.byteLength !== 0) {
        this.failProtocol("invalid completion frame");
        return;
      }
      this.clientMetrics.markComplete();
      void this.ensureCompleteCapture().catch(() => undefined);
      this.stateLabel.textContent = `complete / seq ${this.lastApplied}`;
      return;
    }
    if (frame.kind !== FrameKind.Output || frame.sequence !== this.lastReceived + 1n || frame.creditCost === 0) {
      this.failProtocol("output ordering or credit violation");
      return;
    }
    this.lastReceived = frame.sequence;
    const receipt = this.clientMetrics.recordReceive(frame);
    const apply = () => {
      if (!this.bootstrap || frame.generation !== this.bootstrap.generation || this.disposed || !this.credit) return;
      this.clientMetrics.recordApplied(receipt, frame.payload);
      try {
        this.credit.complete(frame.sequence, frame.creditCost);
      } catch (error) {
        this.failProtocol(String(error));
      }
    };
    if (frame.payload.byteLength === 0) {
      queueMicrotask(apply);
    } else {
      this.terminal.write(frame.payload, apply);
    }
  }

  private onUrgent(socket: WebSocket, event: MessageEvent): void {
    if (socket !== this.urgentSocket) return;
    if (!(event.data instanceof ArrayBuffer) || !this.bootstrap) {
      this.failProtocol("non-binary urgent ACK");
      return;
    }
    let frame: TerminalFrame;
    try {
      frame = decodeFrame(event.data);
    } catch (error) {
      this.failProtocol(`urgent ACK decode failed: ${String(error)}`);
      return;
    }
    const waiter = this.urgentWaiters.get(frame.correlation);
    const classification = this.urgentAckLedger.consume(frame);
    if (classification === "late") return;
    if (!waiter || classification !== "live") {
      this.failProtocol("invalid urgent ACK semantics");
      return;
    }
    this.urgentWaiters.delete(frame.correlation);
    this.lastUrgentRTT = performance.now() - waiter.startedAt;
      this.clientMetrics.recordUrgentRTT(this.lastUrgentRTT);
    waiter.resolve(this.lastUrgentRTT);
  }

  private sendData(frame: TerminalFrame): void {
    if (!this.dataSocket || this.dataSocket.readyState !== WebSocket.OPEN) {
      throw new Error("data route is not open");
    }
    this.dataSocket.send(encodeFrame(frame));
  }

  private failProtocol(reason: string): void {
    this.stateLabel.textContent = `protocol error: ${reason}`;
    this.closeSockets();
    if (this.drainWaiter) {
      clearTimeout(this.drainWaiter.timeout);
      this.drainWaiter.reject(new Error(reason));
      this.drainWaiter = undefined;
    }
  }

  private closeSockets(): void {
    this.dataSocket?.close();
    this.urgentSocket?.close();
    this.dataSocket = undefined;
    this.urgentSocket = undefined;
  }

  private rejectUrgentWaiters(): void {
    this.urgentWaiters.clear();
  }

  private backendMemory(memory: MemorySnapshot): BackendMemorySample {
    return {
      sampledAtUnixMillis: memory.sampledAtUnixMillis,
      goHeapAllocBytes: memory.goHeapAllocBytes,
      goHeapInuseBytes: memory.goHeapInuseBytes,
      serviceLifetimeObservedMaxGoHeapAllocBytes: memory.serviceLifetimeObservedMaxGoHeapAllocBytes,
      processRssBytes: memory.processRssBytes,
      processLifetimePeakRssBytes: memory.processLifetimePeakRssBytes,
      processRssAvailable: memory.processRssAvailable,
      processRssError: memory.processRssError,
    };
  }

  private recordMemoryPhase(phase: MemoryPhase, memory: MemorySnapshot): void {
    this.clientMetrics.recordMemoryPhase(phase, this.backendMemory(memory));
  }

  private maybeCaptureSteady(force = false): Promise<void> | undefined {
    if (this.clientMetrics.hasMemoryPhase("steady")) return undefined;
    if (this.clientMetrics.hasMemoryPhaseError("steady") && !force) return undefined;
    if (this.steadyCapture) return this.steadyCapture;
    if (!force && (this.steadyRequested || this.expectedCreditBytes <= 0 ||
      this.clientMetrics.appliedCreditBytes * 2 < this.expectedCreditBytes)) return undefined;
    this.steadyRequested = true;
    const capture = ProbeService.Snapshot().then((snapshot) => {
      const metrics = (snapshot.sessions ?? []).find((item) => item.sessionId === this.sessionId);
      if (metrics) this.latestBackend = metrics;
      this.recordMemoryPhase("steady", snapshot.memory);
    }).catch((error) => {
      this.clientMetrics.recordMemoryPhaseError("steady", error);
      throw error;
    }).finally(() => {
      if (this.steadyCapture === capture) this.steadyCapture = undefined;
    });
    this.steadyCapture = capture;
    void capture.catch(() => undefined);
    return capture;
  }

  private ensureCompleteCapture(): Promise<void> {
    if (this.clientMetrics.hasMemoryPhase("complete")) return Promise.resolve();
    if (this.completeCapture) return this.completeCapture;
    const capture = (async () => {
      try {
        await this.maybeCaptureSteady(true);
      } catch {
        // The explicit steady error outcome permits complete/settled cleanup.
      }
      try {
        const snapshot = await ProbeService.Snapshot();
        const metrics = (snapshot.sessions ?? []).find((item) => item.sessionId === this.sessionId);
        if (metrics) this.latestBackend = metrics;
        this.recordMemoryPhase("complete", snapshot.memory);
      } catch (error) {
        this.clientMetrics.recordMemoryPhaseError("complete", error);
        throw error;
      }
    })().finally(() => {
      if (this.completeCapture === capture) this.completeCapture = undefined;
    });
    this.completeCapture = capture;
    void capture.catch(() => undefined);
    return capture;
  }
}

const workloadSelect = document.querySelector<HTMLSelectElement>("#workload")!;
const phaseSelect = document.querySelector<HTMLSelectElement>("#phase")!;
const terminalGrid = document.querySelector<HTMLElement>("#terminals")!;
const runButton = document.querySelector<HTMLButtonElement>("#run")!;
const stopButton = document.querySelector<HTMLButtonElement>("#stop")!;
const rebindButton = document.querySelector<HTMLButtonElement>("#rebind")!;
const reloadButton = document.querySelector<HTMLButtonElement>("#reload")!;
const stallButton = document.querySelector<HTMLButtonElement>("#stall")!;
const moveButton = document.querySelector<HTMLButtonElement>("#move")!;
const interruptButton = document.querySelector<HTMLButtonElement>("#interrupt")!;
const exportButton = document.querySelector<HTMLButtonElement>("#export")!;
const statusLabel = document.querySelector<HTMLElement>("#status")!;
const summaryLabel = document.querySelector<HTMLElement>("#summary")!;
const listenerLabel = document.querySelector<HTMLElement>("#listener")!;
const reportStore = new ReportStore();
const query = new URLSearchParams(location.search);
const popupHandoffID = query.get("handoff");
const popupMode = Boolean(popupHandoffID);
let sessions: SessionView[] = [];
let refreshTimer = 0;
let nextRoundIndex = 1;
let roundFinalization: Promise<RoundReport | undefined> | undefined;
let popupMoveWatchdog: { interval: number; timeout: number; recovering: boolean } | undefined;
const routeOperation = new RouteOperationController();
let roundReportRetentionAtStart = reportStore.retentionSnapshot();
let autorunActive = false;
let autorunStep = "initialize";

function setControls(active: boolean): void {
	const operationActive = routeOperation.active || autorunActive;
  runButton.disabled = active || popupMode || operationActive;
  stopButton.disabled = !active || operationActive;
  rebindButton.disabled = !active || operationActive;
  reloadButton.disabled = !active || operationActive;
  stallButton.disabled = !active || operationActive;
  interruptButton.disabled = !active || operationActive;
  exportButton.disabled = operationActive;
  workloadSelect.disabled = active || popupMode || operationActive;
  phaseSelect.disabled = active || popupMode || operationActive;
  document.querySelectorAll<HTMLInputElement>('input[name="sessions"]').forEach((input) => {
    input.disabled = active || popupMode || operationActive;
  });
  moveButton.disabled = !active || popupMode || operationActive || sessions.length !== 1;
}

function requireNoRouteOperation(action: string): void {
  routeOperation.assertAvailable(action);
}

function releaseRouteOperation(operation: RouteOperation): void {
  routeOperation.release(operation);
}

function sessionCount(): number {
  return Number(document.querySelector<HTMLInputElement>('input[name="sessions"]:checked')?.value ?? 1);
}

async function cleanupPartialSessions(sessionIDs: string[], views: SessionView[], cause: unknown): Promise<AggregateError> {
  return cleanupPartialResources(
    sessionIDs,
    views,
    (sessionID) => ProbeService.StopSession(sessionID),
    (view) => view.dispose(),
    cause,
  );
}

async function run(): Promise<void> {
  requireNoRouteOperation("Run");
  try {
    await stop(false);
    setControls(true);
    statusLabel.textContent = "Starting";
    const count = sessionCount();
    const phase = phaseSelect.value;
    const roundIndex = nextRoundIndex++;
    roundReportRetentionAtStart = reportStore.retentionSnapshot();
    const backendSessionIDs: string[] = [];
    const createdViews: SessionView[] = [];
    try {
      for (let index = 0; index < count; index += 1) {
        const info = await ProbeService.CreateSession(workloadSelect.value, {
          chunkCount: 0, totalBytes: 0, lineCount: 0, metadataFrames: 0,
        });
        backendSessionIDs.push(info.sessionId);
        createdViews.push(await SessionView.create({
          sessionId: info.sessionId, workload: info.workload, phase, roundIndex, lastApplied: "0",
        }));
      }
    } catch (error) {
      sessions = [];
      throw await cleanupPartialSessions(backendSessionIDs, createdViews, error);
    }
    sessions = createdViews;
    terminalGrid.replaceChildren(...sessions.map((session) => session.panel));
    await Promise.all(sessions.map((session) => session.bind()));
    const startSnapshot = await ProbeService.Snapshot();
    const startMetrics = new Map((startSnapshot.sessions ?? []).map((metrics) => [metrics.sessionId, metrics]));
    sessions.forEach((session) => {
      const metrics = startMetrics.get(session.sessionId);
      if (!metrics) throw new Error(`start metrics missing for ${session.sessionId}`);
      session.recordStartSample(metrics, startSnapshot.memory);
    });
    await Promise.all(sessions.map((session) => ProbeService.StartSession(session.sessionId)));
    statusLabel.textContent = "Running";
    setControls(true);
    persistActiveSessions();
    sessions.forEach((session) => session.fit());
  } catch (error) {
    statusLabel.textContent = "Start failed";
    summaryLabel.textContent = String(error);
    await stop();
  }
}

async function buildCurrentRound(target = sessions, settledAtUnixMillis = 0, completedAtUnixMillis = Date.now()): Promise<RoundReport | undefined> {
  const reportable = target.filter((session) => session.hasMemoryPhase("start"));
  if (reportable.length === 0) return undefined;
  const reports = await Promise.all(reportable.map((session) => session.report()));
  const availableMaximum = (values: Array<number | null>): number | null => {
    const available = values.filter((value): value is number => value !== null);
    return available.length > 0 ? Math.max(...available) : null;
  };
  return {
    roundIndex: reportable[0].roundIndex,
    phase: reportable[0].phase,
    startedAtUnixMillis: Math.min(...reportable.map((session) => session.roundStartedAt)),
    completedAtUnixMillis,
    settledAtUnixMillis,
    settleCondition: settledAtUnixMillis > 0 ? ROUND_SETTLE_CONDITION : "not settled; live preview only",
    memorySampleProtocol: {
      start: "after xterm and both WebSocket routes are ready, before StartSession",
      steady: "first snapshot requested when acknowledged credit reaches at least 50% of deterministic expected credit while output is active",
      complete: "after the completion frame confirms the full acknowledged sequence and digest state",
      settled: ROUND_SETTLE_CONDITION,
    },
    roundObservedSampleMaxima: {
      goHeapAllocBytes: Math.max(...reports.map((report) => report.roundObservedSampleMaxima.goHeapAllocBytes)),
      goHeapInuseBytes: Math.max(...reports.map((report) => report.roundObservedSampleMaxima.goHeapInuseBytes)),
      processRssBytes: availableMaximum(reports.map((report) => report.roundObservedSampleMaxima.processRssBytes)),
      jsHeapBytes: availableMaximum(reports.map((report) => report.roundObservedSampleMaxima.jsHeapBytes)),
    },
    retainedReportAtStart: roundReportRetentionAtStart,
    retainedReportAtEnd: reportStore.retentionSnapshot(),
    sessions: reports,
  };
}

function settleDelay(): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ROUND_SETTLE_DELAY_MILLIS));
}

async function finalizeRound(
  reason: "completed" | "user-stop" | "replaced",
  clearStorage = true,
  owner?: RouteOperation,
): Promise<RoundReport | undefined> {
  if (owner) routeOperation.assertOwned(owner, "Round finalization");
  else requireNoRouteOperation("Round finalization");
  if (roundFinalization) return roundFinalization;
  const previous = sessions;
  if (previous.length === 0) return;
	roundFinalization = (async () => {
    const errors: unknown[] = [];
		let settledRound: RoundReport | undefined;
    setControls(false);
    statusLabel.textContent = reason === "completed" ? "Settling completed round" : "Stopping and settling";
    const cleanupResults = await Promise.allSettled(previous.map((session) => session.releaseResources(reason)));
    const releaseResults: ResourceReleaseResult[] = [];
    cleanupResults.forEach((result) => {
      if (result.status === "rejected") {
        errors.push(result.reason);
        releaseResults.push({ stopSucceeded: false, errors: [result.reason] });
      } else {
        releaseResults.push(result.value);
        errors.push(...result.value.errors);
      }
    });
    const resourcesReleasedAt = Date.now();
    await settleDelay();
    try {
      if (!releasesCanSettle(releaseResults)) {
        throw new Error("backend StopSession failure prevents settled evidence and archival");
      }
      const settledSnapshot = await ProbeService.Snapshot();
      const settledMetrics = new Map((settledSnapshot.sessions ?? []).map((metrics) => [metrics.sessionId, metrics]));
      previous.forEach((session) => {
        if (!session.hasMemoryPhase("start")) return;
        try {
          session.recordSettledSample(settledMetrics.get(session.sessionId), settledSnapshot.memory);
        } catch (error) {
          errors.push(error);
        }
      });
      const settledAt = Date.now();
      const round = await buildCurrentRound(previous, settledAt, resourcesReleasedAt);
			if (round) {
				reportStore.add(round);
				settledRound = round;
			}
    } catch (error) {
      errors.push(error);
    } finally {
      if (sessions === previous) sessions = [];
      terminalGrid.replaceChildren();
      if (clearStorage) window.sessionStorage.removeItem(ACTIVE_STORAGE_KEY);
      setControls(false);
      statusLabel.textContent = reason === "completed" ? "Complete and settled" : "Idle";
      summaryLabel.textContent = reportStore.size > 0 ? `${reportStore.size} bounded settled round record(s)` : "No active sessions";
    }
    if (errors.length > 0) throw new AggregateError(errors, "round cleanup completed with errors");
		return settledRound;
  })().finally(() => { roundFinalization = undefined; });
  return roundFinalization;
}

async function stop(clearStorage = true): Promise<void> {
  await finalizeRound("user-stop", clearStorage);
}

async function rebind(): Promise<void> {
  requireNoRouteOperation("Rebind");
  rebindButton.disabled = true;
  statusLabel.textContent = "Draining";
  try {
    await Promise.all(sessions.map((session) => session.rebind()));
    statusLabel.textContent = "Running";
  } catch (error) {
    statusLabel.textContent = "Rebind failed";
    summaryLabel.textContent = String(error);
  } finally {
    setControls(sessions.length > 0);
  }
}

async function reloadAfterDrain(): Promise<void> {
  requireNoRouteOperation("Reload");
  routeOperation.acquire("reload");
  setControls(true);
  reloadButton.disabled = true;
  statusLabel.textContent = "Draining for reload";
  try {
    await prepareWithRollback(
      sessions,
      (session) => session.prepareHandoff("reload"),
      (session) => session.recoverPreparedHandoff("reload"),
      persistActiveSessions,
    );
    location.reload();
  } catch (error) {
    statusLabel.textContent = "Reload drain failed";
    summaryLabel.textContent = String(error);
    if (sessions.every((session) => session.routeReady)) {
      releaseRouteOperation("reload");
      setControls(true);
    }
  }
}

async function moveToPopup(): Promise<void> {
  requireNoRouteOperation("Move");
  if (sessions.length !== 1 || popupMode) return;
  routeOperation.acquire("popup-transfer");
  setControls(true);
  const session = sessions[0];
  let prepared = false;
  moveButton.disabled = true;
  statusLabel.textContent = "Draining for popup";
  try {
    const clientState = await session.prepareHandoff("main-to-popup");
    prepared = true;
    const handoff = await ProbeService.CreatePopupHandoff(session.sessionId, Number(session.lastApplied), clientState);
    startPopupMoveWatchdog(session, handoff);
    setControls(true);
    statusLabel.textContent = "Moving to popup";
  } catch (error) {
    const errors: unknown[] = [error];
    if (prepared) {
      try {
        await session.bind();
        await ProbeService.StartSession(session.sessionId);
        session.finishOpenInterruption("main-to-popup");
      } catch (recoveryError) {
        errors.push(recoveryError);
      }
    }
    statusLabel.textContent = errors.length === 1 ? "Move failed; source recovered" : "Move recovery failed";
    summaryLabel.textContent = String(new AggregateError(errors, "popup creation failed"));
    if (session.routeReady) {
      releaseRouteOperation("popup-transfer");
      setControls(true);
    }
  }
}

function clearPopupMoveWatchdog(): void {
  if (!popupMoveWatchdog) return;
  clearInterval(popupMoveWatchdog.interval);
  clearTimeout(popupMoveWatchdog.timeout);
  popupMoveWatchdog = undefined;
}

function startPopupMoveWatchdog(session: SessionView, handoff: PopupHandoffInfo): void {
  clearPopupMoveWatchdog();
  const recover = async (reason: string) => {
    if (!popupMoveWatchdog || popupMoveWatchdog.recovering) return;
    popupMoveWatchdog.recovering = true;
    try {
      const aborted = await ProbeService.AbortPopupHandoff(handoff.handoffId, handoff.cancelToken);
      await session.bind(aborted.bootstrap);
      await ProbeService.StartSession(session.sessionId);
      await ProbeService.ConfirmPopupRecovery(
        handoff.handoffId,
        handoff.cancelToken,
        aborted.confirmationToken,
        aborted.bootstrap.generation,
      );
      session.finishOpenInterruption("main-to-popup");
      releaseRouteOperation("popup-transfer");
      statusLabel.textContent = "Source recovered";
      summaryLabel.textContent = reason;
      clearPopupMoveWatchdog();
      setControls(true);
    } catch (error) {
      popupMoveWatchdog.recovering = false;
      summaryLabel.textContent = `Popup recovery failed: ${String(error)}`;
    }
  };
  const interval = window.setInterval(() => {
    void ProbeService.PopupHandoffStatus(handoff.handoffId, handoff.cancelToken)
      .then((status) => { if (status.state === "aborted") void recover("Popup closed or expired before completion"); })
      .catch(() => undefined);
  }, 250);
  const timeoutDelay = Math.max(1_000, handoff.expiresAtUnixMillis - Date.now() + 1_000);
  const timeout = window.setTimeout(() => void recover("Popup handoff watchdog expired"), timeoutDelay);
  popupMoveWatchdog = { interval, timeout, recovering: false };
}

function toggleStall(): void {
  requireNoRouteOperation("Credit stall");
  const next = !sessions.every((session) => session.stalled);
  sessions.forEach((session) => session.setCreditStalled(next));
  stallButton.textContent = next ? "Resume credit" : "Stall credit";
  statusLabel.textContent = next ? "Credit stalled" : "Running";
}

async function interrupt(): Promise<void> {
  requireNoRouteOperation("Interrupt");
  interruptButton.disabled = true;
  try {
    const rtts = await Promise.all(sessions.map((session) => session.sendInterrupt()));
    summaryLabel.textContent = `Urgent renderer RTT ${rtts.map((value) => `${value.toFixed(1)} ms`).join(" / ")}`;
  } catch (error) {
    summaryLabel.textContent = String(error);
  } finally {
    interruptButton.disabled = sessions.length === 0;
  }
}

async function exportReport(): Promise<void> {
  requireNoRouteOperation("Export");
  if (sessions.length > 0) await finalizeRound("user-stop");
  const report = reportStore.build();
  const blob = new Blob([JSON.stringify(report)], { type: "application/json" });
  const href = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = href;
  anchor.download = `terminal-data-plane-report-${Date.now()}.json`;
  anchor.click();
  URL.revokeObjectURL(href);
}

async function refresh(): Promise<void> {
  try {
    const snapshot = await ProbeService.Snapshot();
    listenerLabel.textContent = `${snapshot.listenerAddress} / ${formatBytes(snapshot.windowBytes)} receive window`;
    const byID = new Map((snapshot.sessions ?? []).map((metrics) => [metrics.sessionId, metrics]));
    sessions.forEach((session) => {
      const metrics = byID.get(session.sessionId);
      if (metrics) session.updateMetrics(metrics);
    });
    const active = sessions.map((session) => byID.get(session.sessionId)).filter((value): value is SessionMetrics => Boolean(value));
    if (active.length > 0) {
      const payload = active.reduce((sum, item) => sum + item.payloadBytes, 0);
      const credit = active.reduce((sum, item) => sum + item.creditBytes, 0);
      const replayed = active.reduce((sum, item) => sum + item.framesReplayed, 0);
      const pauses = active.reduce((sum, item) => sum + item.pauseCount, 0);
      const maximum = Math.max(...active.map((item) => item.backendMaxOutstanding));
      const rss = snapshot.memory.processRssAvailable && snapshot.memory.processRssBytes !== null
        ? formatBytes(snapshot.memory.processRssBytes) : "RSS unavailable";
      summaryLabel.textContent = `payload ${formatBytes(payload)} / credit ${formatBytes(credit)} / replay ${replayed} / pauses ${pauses} / max ${formatBytes(maximum)} / ${rss}`;
		if (!autorunActive && active.every((item) => item.complete) && sessions.every((session) => session.complete) && !roundFinalization) {
        void finalizeRound("completed").catch((error) => { summaryLabel.textContent = String(error); });
      }
    }
  } catch (error) {
    summaryLabel.textContent = String(error);
  }
}

function persistActiveSessions(): void {
	if (autorunActive || sessions.length === 0) return;
  window.sessionStorage.setItem(ACTIVE_STORAGE_KEY, JSON.stringify(sessions.map((session) => session.stored())));
}

function readStoredSessions(): StoredSession[] {
  try {
    const parsed: unknown = JSON.parse(window.sessionStorage.getItem(ACTIVE_STORAGE_KEY) ?? "[]");
    if (!Array.isArray(parsed)) return [];
    return parsed.slice(0, 8).filter((item): item is StoredSession => {
      if (!item || typeof item !== "object") return false;
      const value = item as Partial<StoredSession>;
      return typeof value.sessionId === "string" && typeof value.workload === "string" &&
        typeof value.phase === "string" && Number.isInteger(value.roundIndex) &&
        typeof value.lastApplied === "string" && /^\d+$/.test(value.lastApplied) &&
        (value.clientState === undefined || (typeof value.clientState === "string" && value.clientState.length <= 128 * 1024));
    });
  } catch {
    return [];
  }
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
  return `${(value / (1024 * 1024)).toFixed(2)} MiB`;
}

function reportActionError(error: unknown): void {
  summaryLabel.textContent = String(error);
}

async function restorePopup(handoffID: string): Promise<void> {
  const claimed = await ProbeService.ClaimPopupHandoff(handoffID);
  let session: SessionView | undefined;
  try {
    session = await SessionView.create({
      sessionId: claimed.sessionId,
      workload: claimed.workload,
      phase: "measured",
      roundIndex: 1,
      lastApplied: String(claimed.lastApplied),
      clientState: claimed.clientState,
    });
    sessions = [session];
    terminalGrid.replaceChildren(session.panel);
    const popupRoute = await ProbeService.ResumePopupHandoff(claimed.handoffId, claimed.completionToken, claimed.lastApplied);
    await session.bind(popupRoute);
    const reboundSnapshot = await ProbeService.Snapshot();
    const reboundMetrics = (reboundSnapshot.sessions ?? []).find((metrics) => metrics.sessionId === session!.sessionId);
    if (!reboundMetrics) throw new Error("popup rebound metrics are unavailable");
    session.resumeBackendMetrics(reboundMetrics);
    await ProbeService.StartSession(session.sessionId);
    await ProbeService.CompletePopupHandoff(claimed.handoffId, claimed.completionToken);
    session.finishOpenInterruption("main-to-popup");
    setControls(true);
    statusLabel.textContent = "Running in popup";
    session.fit();
  } catch (error) {
    sessions = [];
    session?.dispose();
    const errors: unknown[] = [error];
    try {
      await ProbeService.RejectPopupHandoff(claimed.handoffId, claimed.completionToken);
    } catch (rejectError) {
      errors.push(rejectError);
    }
    throw new AggregateError(errors, "popup restore failed after recovery handoff");
  }
}

async function restoreOrInitialize(): Promise<void> {
	const autorunConfig = await ProbeService.GetAutorunConfig();
	if (autorunConfig.enabled) {
		await runAutorun(autorunConfig);
		return;
	}
  const workloads: WorkloadDescriptor[] = (await ProbeService.Workloads()) ?? [];
  workloadSelect.replaceChildren(...workloads.map((workload) => {
    const option = document.createElement("option");
    option.value = workload.id;
    option.textContent = workload.label;
    return option;
  }));
  if (popupHandoffID) {
    try {
      await restorePopup(popupHandoffID);
    } catch (error) {
      statusLabel.textContent = "Popup handoff failed";
      summaryLabel.textContent = String(error);
    }
    return;
  }
  const stored = readStoredSessions();
  if (stored.length === 0) {
    setControls(false);
    await refresh();
    return;
  }
  try {
    const restoredViews: SessionView[] = [];
    try {
      for (const item of stored) restoredViews.push(await SessionView.create(item));
    } catch (error) {
      throw await cleanupPartialSessions(stored.map((item) => item.sessionId), restoredViews, error);
    }
    sessions = restoredViews;
    terminalGrid.replaceChildren(...sessions.map((session) => session.panel));
    setControls(true);
    statusLabel.textContent = "Restoring";
    await Promise.all(sessions.map((session) => session.bind()));
    const reboundSnapshot = await ProbeService.Snapshot();
    const reboundMetrics = new Map((reboundSnapshot.sessions ?? []).map((metrics) => [metrics.sessionId, metrics]));
    sessions.forEach((session) => {
      const metrics = reboundMetrics.get(session.sessionId);
      if (!metrics) throw new Error(`reload rebound metrics missing for ${session.sessionId}`);
      session.resumeBackendMetrics(metrics);
    });
    await Promise.all(sessions.map((session) => ProbeService.StartSession(session.sessionId)));
    sessions.forEach((session) => session.finishOpenInterruption("reload"));
    statusLabel.textContent = "Running";
  } catch (error) {
    summaryLabel.textContent = `Restore failed: ${String(error)}`;
    await stop();
  }
}

async function runAutorun(config: AutorunConfig): Promise<void> {
	autorunActive = true;
	routeOperation.acquire("autorun");
	setControls(false);
	const startedAt = performance.now();
	let view: SessionView | undefined;
	let backendSessionID = "";
	try {
		autorunStep = "create-session";
		roundReportRetentionAtStart = reportStore.retentionSnapshot();
		const info = await ProbeService.CreateSession(config.workload, {
			chunkCount: config.chunkCount, totalBytes: 0, lineCount: 0, metadataFrames: 0,
		});
		backendSessionID = info.sessionId;
		view = await SessionView.create({
			sessionId: info.sessionId, workload: info.workload, phase: "autorun", roundIndex: nextRoundIndex++, lastApplied: "0",
		});
		sessions = [view];
		terminalGrid.replaceChildren(view.panel);

		autorunStep = "bind-stalled-route";
		await view.bind();
		view.setCreditStalled(true);
		const startSnapshot = await ProbeService.Snapshot();
		const startMetrics = (startSnapshot.sessions ?? []).find((metrics) => metrics.sessionId === view!.sessionId);
		if (!startMetrics) throw new Error("autorun start metrics are unavailable");
		view.recordStartSample(startMetrics, startSnapshot.memory);
		await ProbeService.StartSession(view.sessionId);
		statusLabel.textContent = "Autorun stalled transport";

		autorunStep = "observe-stall";
		const stalled = await waitForCondition({
			name: "frontend queue and backend pause",
			timeoutMillis: config.conditionTimeoutMillis,
			pollIntervalMillis: config.pollIntervalMillis,
			sample: async () => ({ metrics: autorunMetrics(await ProbeService.Snapshot(), view!.sessionId), credit: view!.creditSnapshot() }),
			accept: ({ metrics, credit }) => credit.pendingBytes > 0 && metrics.pauseCount > 0 && metrics.outstandingBytes > 0,
		});
		const stalledSequence = stalled.metrics.appliedSequence;

		autorunStep = "urgent-while-stalled";
		const urgentRTT = await view.sendInterrupt();
		if (!view.stalled || urgentRTT <= 0) throw new Error("urgent renderer ACK did not arrive while credit was stalled");
		await waitForCondition({
			name: "backend urgent receipt",
			timeoutMillis: config.conditionTimeoutMillis,
			pollIntervalMillis: config.pollIntervalMillis,
			sample: async () => autorunMetrics(await ProbeService.Snapshot(), view!.sessionId),
			accept: (metrics) => metrics.urgentCount === 1,
		});

		autorunStep = "resume-progress";
		view.setCreditStalled(false);
		await waitForCondition({
			name: "resumed acknowledged progress",
			timeoutMillis: config.conditionTimeoutMillis,
			pollIntervalMillis: config.pollIntervalMillis,
			sample: async () => autorunMetrics(await ProbeService.Snapshot(), view!.sessionId),
			accept: (metrics) => metrics.appliedSequence > stalledSequence && metrics.resumeCount > 0,
		});

		autorunStep = "ordered-rebind";
		await view.rebind();
		await waitForCondition({
			name: "same-window replacement generation",
			timeoutMillis: config.conditionTimeoutMillis,
			pollIntervalMillis: config.pollIntervalMillis,
			sample: async () => autorunMetrics(await ProbeService.Snapshot(), view!.sessionId),
			accept: (metrics) => metrics.generation === 2 && metrics.routeInterruptionCount === 1 && view!.routeReady,
		});

		autorunStep = "exact-completion";
		await waitForCondition({
			name: "exact terminal workload completion",
			timeoutMillis: config.conditionTimeoutMillis,
			pollIntervalMillis: config.pollIntervalMillis,
			sample: async () => autorunMetrics(await ProbeService.Snapshot(), view!.sessionId),
			accept: (metrics) => view!.complete && metrics.complete && metrics.error === "" &&
				metrics.sequence === config.chunkCount && metrics.appliedSequence === config.chunkCount &&
				metrics.framesAcked === config.chunkCount && metrics.outstandingBytes === 0,
		});
		await view.waitForCompleteSample();

		autorunStep = "finalize-settle";
		const round = await finalizeRound("completed", true, "autorun");
		if (!round) throw new Error("autorun settled report is unavailable");
		const result = buildAutorunResult(config, round, Math.max(1, performance.now() - startedAt));

		autorunStep = "report-success";
		await ProbeService.ReportAutorunSuccess(result);
		statusLabel.textContent = "Autorun complete";
	} catch (error) {
		statusLabel.textContent = "Autorun failed";
		summaryLabel.textContent = String(error);
		await cleanupFailedAutorun(backendSessionID, error);
	} finally {
		if (routeOperation.operation === "autorun") routeOperation.release("autorun");
		setControls(false);
	}
}

function autorunMetrics(snapshot: { sessions: SessionMetrics[] | null }, sessionID: string): SessionMetrics {
	const metrics = (snapshot.sessions ?? []).find((item) => item.sessionId === sessionID);
	if (!metrics) throw new Error("autorun session metrics are unavailable");
	return metrics;
}

async function cleanupFailedAutorun(backendSessionID: string, cause: unknown): Promise<AggregateError> {
	const previous = sessions;
	sessions = [];
	const sessionIDs = new Set(previous.map((session) => session.sessionId));
	if (backendSessionID) sessionIDs.add(backendSessionID);
	const cleanupError = await cleanupFailedAutorunResources(
		[...sessionIDs],
		previous,
		(sessionID) => ProbeService.StopSession(sessionID),
		(session) => session.dispose(),
		() => ProbeService.ReportAutorunFailure({ formatVersion: 1, step: autorunStep, code: "frontend-step-failed" }),
		cause,
	);
	terminalGrid.replaceChildren();
	window.sessionStorage.removeItem(ACTIVE_STORAGE_KEY);
	return cleanupError;
}

runButton.addEventListener("click", () => void run().catch(reportActionError));
stopButton.addEventListener("click", () => void stop().catch(reportActionError));
rebindButton.addEventListener("click", () => void rebind().catch(reportActionError));
reloadButton.addEventListener("click", () => void reloadAfterDrain().catch(reportActionError));
stallButton.addEventListener("click", toggleStall);
moveButton.addEventListener("click", () => void moveToPopup().catch(reportActionError));
interruptButton.addEventListener("click", () => void interrupt().catch(reportActionError));
exportButton.addEventListener("click", () => void exportReport().catch(reportActionError));
window.addEventListener("resize", () => sessions.forEach((session) => session.fit()));
window.addEventListener("beforeunload", persistActiveSessions);
refreshTimer = window.setInterval(() => void refresh(), 250);
window.addEventListener("pagehide", () => {
  clearInterval(refreshTimer);
  clearPopupMoveWatchdog();
}, { once: true });
void restoreOrInitialize().catch(async (error) => {
	statusLabel.textContent = "Initialization failed";
	summaryLabel.textContent = String(error);
	try {
		await ProbeService.ReportAutorunFailure({ formatVersion: 1, step: "initialize", code: "frontend-step-failed" });
	} catch {
		// Interactive mode and an already-decided autorun outcome both reject this call.
	}
});
