import { SYNC_STORAGE_KEYS, type MasterKeyConfig } from '../../../domain/sync';
import { flushHostProfileWrites, hostStorageAdapter, refreshHostProfile } from '../../persistence/hostStorageAdapter';
import { nativeCloudSyncRequired } from './cloudSyncFacade';

const pending = new WeakMap<object, Promise<unknown>>();
const LOCK_NAME = 'netcatty-cloud-sync';

interface SyncOwner {
  getState(): { masterKeyConfig: MasterKeyConfig | null };
  lock(): void;
  adoptStoredMasterKeyConfig(config: MasterKeyConfig | null): void;
}

/** Serializes legacy sync and rotation; v2 keeps its existing Web Lock too. */
export function withSyncOperation<T>(owner: SyncOwner, task: () => Promise<T>): Promise<T> {
  const run = async () => {
    const guarded = async () => {
      await refreshHostProfile();
      const stored = hostStorageAdapter.read<MasterKeyConfig>(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG);
      if (JSON.stringify(stored) !== JSON.stringify(owner.getState().masterKeyConfig)) {
        owner.adoptStoredMasterKeyConfig(stored);
        throw new Error('Master key changed in another window; unlock again before syncing');
      }
      const result = await task();
      await flushHostProfileWrites();
      return result;
    };
    const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined;
    if (locks) return locks.request(LOCK_NAME, { mode: 'exclusive' }, guarded);
    if (nativeCloudSyncRequired()) throw new Error('Native cloud sync requires Web Locks');
    return guarded();
  };
  const result = (pending.get(owner) ?? Promise.resolve()).catch(() => undefined).then(run);
  pending.set(owner, result);
  return result;
}
