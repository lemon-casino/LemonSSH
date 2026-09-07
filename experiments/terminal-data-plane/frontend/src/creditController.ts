export interface CreditAck {
  sequence: bigint;
  cost: number;
}

export interface CreditStats {
  acknowledgedSequence: bigint;
  pendingBytes: number;
  pendingRecords: number;
  queueHighWaterBytes: number;
  queueHighWaterRecords: number;
  stallCount: number;
  stallDurationMillis: number;
  stalled: boolean;
}

const MAX_ACK_RECORDS = 1024;
const MAX_DRAIN_WAITERS = 8;

export class CreditController {
  private acknowledged: bigint;
  private readonly completed = new Map<bigint, number>();
  private pendingBytes = 0;
  private highWaterBytes = 0;
  private highWaterRecords = 0;
  private stalled = false;
  private stallStartedAt = 0;
  private stallCount = 0;
  private stallDuration = 0;
  private readonly waiters = new Map<bigint, Array<() => void>>();

  constructor(
    initialSequence: bigint,
    private readonly windowBytes: number,
    private readonly send: (ack: CreditAck) => void,
    private readonly now: () => number = () => performance.now(),
  ) {
    this.acknowledged = initialSequence;
  }

  complete(sequence: bigint, cost: number): void {
    if (sequence <= this.acknowledged || this.completed.has(sequence)) {
      throw new Error(`duplicate credit completion for sequence ${sequence}`);
    }
    if (!Number.isInteger(cost) || cost <= 0 || cost > this.windowBytes) {
      throw new Error(`invalid credit cost ${cost}`);
    }
    if (this.completed.size >= MAX_ACK_RECORDS || this.pendingBytes + cost > this.windowBytes) {
      throw new Error("bounded credit ACK queue exceeded the receive window");
    }
    this.completed.set(sequence, cost);
    this.pendingBytes += cost;
    this.highWaterBytes = Math.max(this.highWaterBytes, this.pendingBytes);
    this.highWaterRecords = Math.max(this.highWaterRecords, this.completed.size);
    this.flush();
  }

  setStalled(stalled: boolean): void {
    if (stalled === this.stalled) return;
    this.stalled = stalled;
    if (stalled) {
      this.stallStartedAt = this.now();
      this.stallCount += 1;
      return;
    }
    this.stallDuration += Math.max(0, this.now() - this.stallStartedAt);
    this.stallStartedAt = 0;
    this.flush();
  }

  waitUntilAcknowledged(sequence: bigint): Promise<void> {
    if (this.acknowledged >= sequence) return Promise.resolve();
    const waiterCount = [...this.waiters.values()].reduce((total, items) => total + items.length, 0);
    if (waiterCount >= MAX_DRAIN_WAITERS) return Promise.reject(new Error("drain waiter limit reached"));
    return new Promise((resolve) => {
      const existing = this.waiters.get(sequence) ?? [];
      existing.push(resolve);
      this.waiters.set(sequence, existing);
    });
  }

  get acknowledgedSequence(): bigint {
    return this.acknowledged;
  }

  snapshot(): CreditStats {
    return {
      acknowledgedSequence: this.acknowledged,
      pendingBytes: this.pendingBytes,
      pendingRecords: this.completed.size,
      queueHighWaterBytes: this.highWaterBytes,
      queueHighWaterRecords: this.highWaterRecords,
      stallCount: this.stallCount,
      stallDurationMillis: this.stallDuration + (this.stalled ? Math.max(0, this.now() - this.stallStartedAt) : 0),
      stalled: this.stalled,
    };
  }

  private flush(): void {
    if (this.stalled) return;
    for (;;) {
      const next = this.acknowledged + 1n;
      const cost = this.completed.get(next);
      if (cost === undefined) break;
      this.send({ sequence: next, cost });
      this.completed.delete(next);
      this.pendingBytes -= cost;
      this.acknowledged = next;
      for (const [target, resolvers] of this.waiters) {
        if (target <= this.acknowledged) {
          this.waiters.delete(target);
          resolvers.forEach((resolve) => resolve());
        }
      }
    }
  }
}
