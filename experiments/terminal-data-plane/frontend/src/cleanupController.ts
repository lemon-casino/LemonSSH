export async function cleanupPartialResources<T>(
  sessionIDs: string[],
  resources: T[],
  stopSession: (sessionID: string) => Promise<unknown>,
  dispose: (resource: T) => void,
  cause: unknown,
): Promise<AggregateError> {
  const errors: unknown[] = [cause];
  for (const resource of resources) {
    try {
      dispose(resource);
    } catch (error) {
      errors.push(error);
    }
  }
  const stopResults = await Promise.allSettled(sessionIDs.map((sessionID) => stopSession(sessionID)));
  stopResults.forEach((result) => {
    if (result.status === "rejected") errors.push(result.reason);
  });
  return new AggregateError(errors, "partial session setup failed after cleanup");
}

const MAX_AUTORUN_FAILURE_ERRORS = 16;

export async function cleanupFailedAutorunResources<T>(
  sessionIDs: string[],
  resources: T[],
  stopSession: (sessionID: string) => Promise<unknown>,
  dispose: (resource: T) => void,
  reportFailure: () => Promise<unknown>,
  cause: unknown,
): Promise<AggregateError> {
  const errors: unknown[] = [cause];
  const record = (error: unknown) => {
    if (errors.length < MAX_AUTORUN_FAILURE_ERRORS) errors.push(error);
  };
  for (const resource of resources) {
    try {
      dispose(resource);
    } catch (error) {
      record(error);
    }
  }
  const stopResults = await Promise.allSettled(sessionIDs.map(async (sessionID) => stopSession(sessionID)));
  stopResults.forEach((result) => {
    if (result.status === "rejected") record(result.reason);
  });
  try {
    await reportFailure();
  } catch (error) {
    record(error);
  }
  return new AggregateError(errors, "autorun failed after bounded cleanup");
}

export async function prepareWithRollback<T>(
  resources: T[],
  prepare: (resource: T) => Promise<unknown>,
  recover: (resource: T) => Promise<unknown>,
  persist: () => void,
): Promise<void> {
  const prepared: T[] = [];
  try {
    for (const resource of resources) {
      await prepare(resource);
      prepared.push(resource);
    }
    persist();
  } catch (cause) {
    const errors: unknown[] = [cause];
    const recoveryResults = await Promise.allSettled(prepared.map((resource) => recover(resource)));
    recoveryResults.forEach((result) => {
      if (result.status === "rejected") errors.push(result.reason);
    });
    throw new AggregateError(errors, "partial preparation failed after rollback");
  }
}

export interface ResourceReleaseResult {
  stopSucceeded: boolean;
  errors: unknown[];
}

export function releasesCanSettle(results: ResourceReleaseResult[]): boolean {
  return results.every((result) => result.stopSucceeded);
}
