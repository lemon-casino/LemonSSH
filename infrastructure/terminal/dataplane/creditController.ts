// Credit controller (P3-01 renderer side): tracks applied sequence, grants
// credit back to the backend only after xterm has applied the bytes, and
// bounds the granted window to the initial 1 MiB receive window contract.

export const RECEIVE_WINDOW_BYTES = 1024 * 1024;

export class CreditController {
  private applied = 0;
  private granted = 0;
  private receivedBytes = 0;

  constructor(private readonly windowBytes: number = RECEIVE_WINDOW_BYTES) {}

  /** First grant must open exactly the receive window (Go contract). */
  openWindow(): { appliedSequence: number; credit: number } {
    if (this.granted !== 0) throw new Error("window already open");
    this.granted = this.windowBytes;
    return { appliedSequence: this.applied, credit: this.windowBytes };
  }

  /** Records applied bytes and returns the credit to return (may be 0). */
  onApplied(bytes: number): number {
    this.receivedBytes += bytes;
    this.applied += bytes;
    const credit = bytes;
    this.granted += credit;
    return credit;
  }

  /** Output admission check for renderer-authorised re-requests. */
  canAdmit(bytes: number): boolean {
    return bytes <= this.granted;
  }

  get appliedSequence(): number {
    return this.applied;
  }

  get outstanding(): number {
    return this.granted;
  }
}
