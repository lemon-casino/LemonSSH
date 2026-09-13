/* eslint-disable @typescript-eslint/no-explicit-any */
import { isProviderReadyForSync, type CloudProvider, type SyncResult, type SyncedFile } from '../../../domain/sync';
import { EncryptionService, verifySyncedFile } from '../EncryptionService';
import { flushHostProfileWrites } from '../../persistence/hostStorageAdapter';
type RemoteFile = SyncedFile & { etag?: string };

/** Remote providers cannot participate in the profile transaction. Retry uses
 * both user-supplied passwords; neither password is persisted in this owner. */
export async function propagateMasterKeyRotationImpl(this: any, oldPassword: string, newPassword: string): Promise<void> {
  if (!await this.verifyPassword(newPassword)) throw new Error('New password does not match the local master key');
  const providers = (Object.keys(this.state.providers) as CloudProvider[]).filter(provider => isProviderReadyForSync(this.state.providers[provider]));
  const signature = (file: SyncedFile | null) => file ? JSON.stringify({ meta: file.meta, payload: file.payload }) : null;
  const generation = this.getSyncSecurityGeneration();
  this.state.pendingLocalSync = true;
  this.state.syncState = 'SYNCING';
  this.notifyStateChange();
  try {
    // Prepare every provider before making any remote write.
    const prepared = (await Promise.allSettled(providers.map(async provider => {
      const adapter = await this.getConnectedAdapter(provider);
      const original: RemoteFile | null = await adapter.download();
      if (!original) return { provider, adapter, original, replacement: null };
      if (await verifySyncedFile(original, newPassword)) return { provider, adapter, original, replacement: null };
      const payload = await EncryptionService.decryptPayload(original, oldPassword);
      const replacement: RemoteFile = await EncryptionService.encryptPayload(payload, newPassword, original.meta.deviceId, original.meta.deviceName, original.meta.appVersion, original.meta.version);
      // Rotation changes encryption only. Preserve lineage and metadata clocks.
      replacement.meta = { ...original.meta, iv: replacement.meta.iv, salt: replacement.meta.salt, kdf: replacement.meta.kdf, kdfIterations: replacement.meta.kdfIterations };
      replacement.etag = original.etag;
      return { provider, adapter, original, replacement };
    }))).map(result => {
      if (result.status === 'rejected') throw result.reason;
      return result.value;
    });
    for (const { provider, adapter, original, replacement } of prepared) {
      this.assertSyncSecurityGeneration(generation);
      if (!replacement) continue;
      const current = await adapter.download();
      this.assertSyncSecurityGeneration(generation);
      if (signature(current) !== signature(original)) throw new Error(`${provider}: cloud snapshot changed during master key rotation`);
      replacement.etag = current?.etag;
      await adapter.upload(replacement);
      const verified = await adapter.download();
      this.assertSyncSecurityGeneration(generation);
      if (signature(verified) !== signature(replacement)) throw new Error(`${provider}: unable to verify rotated cloud snapshot`);
      await this.saveSyncAnchor(provider, verified, adapter.resourceId);
      await flushHostProfileWrites();
    }
    this.state.syncState = 'IDLE';
    this.state.lastError = null;
    this.notifyStateChange();
  } catch (error) {
    const message = `Master key changed locally. Cloud update failed: ${error instanceof Error ? error.message : String(error)}. Keep both passwords and retry the key update.`;
    this.state.syncState = 'ERROR';
    this.state.lastError = message;
    this.notifyStateChange();
    throw new Error(message);
  }
}

export function assertMasterKeySyncSucceeded(results: Map<CloudProvider, SyncResult>, expectedProviders: CloudProvider[]): void {
  const failed = expectedProviders.filter(provider => !results.get(provider)?.success);
  if (failed.length) {
    const details = failed.map(provider => `${provider}: ${results.get(provider)?.error ?? 'sync did not complete'}`).join('; ');
    throw new Error(`Master key changed locally. Cloud sync failed: ${details}. Retry sync before using another device.`);
  }
}
