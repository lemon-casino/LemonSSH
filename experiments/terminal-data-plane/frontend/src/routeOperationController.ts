export type RouteOperation = "reload" | "popup-transfer" | "autorun";

export class RouteOperationController {
  private current?: RouteOperation;

  acquire(operation: RouteOperation): void {
    if (this.current) throw new Error(`route operation ${this.current} is already active`);
    this.current = operation;
  }

  release(operation: RouteOperation): void {
    if (this.current !== operation) throw new Error(`route operation ${operation} does not own the active state`);
    this.current = undefined;
  }

  assertAvailable(action: string): void {
    if (this.current) throw new Error(`${action} is disabled during ${this.current}`);
  }

  assertOwned(operation: RouteOperation, action: string): void {
    if (this.current !== operation) throw new Error(`${action} requires route operation ${operation}`);
  }

  get active(): boolean { return this.current !== undefined; }
  get operation(): RouteOperation | undefined { return this.current; }
}
