import type { CloudSyncHook } from './useCloudSync';
import { isProviderReadyForSync, type SyncPayload } from '../../domain/sync';
import { assertMasterKeySyncSucceeded } from '../../infrastructure/services/cloudSync/masterKeyPropagation';

/** Local commit and remote propagation have distinct failure boundaries. */
export async function updateCloudSyncMasterKey(
  sync: Pick<CloudSyncHook, 'providers' | 'changeMasterKey' | 'propagateMasterKeyRotation' | 'syncNow'>,
  oldPassword: string,
  newPassword: string,
  payload: SyncPayload | null,
  applyConvergentPayload: NonNullable<Parameters<CloudSyncHook['syncNow']>[1]>['applyConvergentPayload'],
): Promise<boolean> {
  const expected = Object.values(sync.providers).filter(isProviderReadyForSync).map(connection => connection.provider);
  if (!await sync.changeMasterKey(oldPassword, newPassword)) return false;
  if (payload) {
    await sync.propagateMasterKeyRotation(oldPassword, newPassword);
    const results = await sync.syncNow(payload, { applyConvergentPayload });
    assertMasterKeySyncSucceeded(results, expected);
  }
  return true;
}
