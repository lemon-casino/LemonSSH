type StreamOptions = Parameters<NonNullable<NetcattyBridge['startStreamTransfer']>>[0];
type TransferEvent = Parameters<Parameters<NonNullable<NetcattyBridge['onGlobalSftpTransferEvent']>>[0]>[0];
export interface TransferProgress {
  taskId: string;
  state: string;
  totalBytes: number;
  doneBytes: number;
  error?: string;
  phase?: 'compressing' | 'uploading';
  controlKind?: 'stream' | 'compressed-upload';
  lifecycleEpoch?: number;
  sourcePath?: string;
  targetPath?: string;
  sourceSessionId?: string;
  targetSessionId?: string;
  sourceHostId?: string;
  targetHostId?: string;
  parentTaskId?: string;
  directoryEntryIndex?: number;
  directoryEntryIdentity?: string;
  direction?: 'upload' | 'download' | 'remote-to-remote';
}
export interface TransferBindings {
  Start(request: { taskId: string; sourceSessionId: string; targetSessionId: string; sourcePath: string; targetPath: string; sourceHostId?: string; targetHostId?: string; parentTaskId?: string; directoryEntryIndex?: number; directoryEntryIdentity?: string }): Promise<TransferProgress>;
  StartCompressed?(request: Parameters<TransferBindings['Start']>[0]): Promise<TransferProgress>;
  Progress(taskId: string): Promise<TransferProgress>;
  List?(): Promise<TransferProgress[]>;
  Pause(taskId: string): Promise<unknown>;
  Resume(taskId: string): Promise<unknown>;
  Cancel(taskId: string): Promise<unknown>;
}

export function createTransferBridge(bindings: TransferBindings | undefined, pollMs = 200) {
  const listeners = new Set<(event: TransferEvent) => void>();
  const active = new Set<string>();
  const observed = new Map<string, { state: string; bytes: number; epoch?: number }>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let pollGeneration = 0;
  const emit = (event: TransferEvent) => {
    for (const listener of listeners) {
      try { listener(event); } catch (error) { console.error('Transfer event listener failed', error); }
    }
  };
  const requireBindings = () => {
    if (!bindings?.Start || !bindings.Progress) throw new Error('Go transfer scheduler is not available');
    return bindings;
  };
  const publish = (progress: TransferProgress, force = false) => {
    const previous = observed.get(progress.taskId);
    if (previous && ['completed', 'cancelled', 'failed'].includes(previous.state)) return;
    if (previous?.epoch !== undefined && progress.lifecycleEpoch !== undefined && progress.lifecycleEpoch < previous.epoch) return;
    if (!force && previous?.state === progress.state && previous.bytes === progress.doneBytes && previous.epoch === progress.lifecycleEpoch) return;
    const direction = progress.direction ?? (progress.sourceSessionId ? progress.targetSessionId ? 'remote-to-remote' : 'download' : 'upload');
    const base = {
      transferId: progress.taskId, sourcePath: progress.sourcePath, targetPath: progress.targetPath,
      fileName: progress.sourcePath?.split(/[\\/]/).pop(), direction,
      sessionId: direction === 'upload' ? progress.targetSessionId : progress.sourceSessionId,
      sourceHostId: progress.sourceHostId, targetHostId: progress.targetHostId,
      parentTaskId: progress.parentTaskId, directoryEntryIndex: progress.directoryEntryIndex,
      directoryEntryIdentity: progress.directoryEntryIdentity,
      controlKind: progress.controlKind ?? 'stream' as const, resumable: true, phase: progress.phase,
      transferred: progress.phase === 'compressing' ? 0 : progress.doneBytes, totalBytes: progress.phase === 'compressing' ? 0 : progress.totalBytes, checkpointBytes: progress.phase === 'compressing' ? 0 : progress.doneBytes,
      lifecycleEpoch: progress.lifecycleEpoch,
      lifecycleState: progress.state === 'paused' ? 'paused' as const : progress.state === 'pending' ? 'queued' as const : 'transferring' as const,
    };
    // Seed a row after reload even when the first observed state is terminal/paused.
    if (!previous && progress.sourcePath) emit({ ...base, type: 'started' });
    const type: TransferEvent['type'] = progress.state === 'failed' ? 'failed' : progress.state === 'completed' ? 'completed'
      : progress.state === 'cancelled' ? 'cancelled' : progress.state === 'paused' ? 'paused'
      : progress.state === 'pending' ? 'queued' : previous?.state === 'paused' ? 'resumed' : previous || progress.sourcePath ? 'progress' : 'started';
    if (previous || !progress.sourcePath || !['running'].includes(progress.state)) emit({ ...base, type, error: progress.error });
    observed.set(progress.taskId, { state: progress.state, bytes: progress.doneBytes, epoch: progress.lifecycleEpoch });
  };
  const pollExisting = async (generation: number) => {
    try {
      const snapshots = await bindings?.List?.() ?? [];
      if (generation !== pollGeneration) return;
      for (const snapshot of snapshots) publish(snapshot);
    } catch (error) { console.error('Transfer snapshot refresh failed', error); }
    finally { if (generation === pollGeneration && listeners.size && bindings?.List) timer = setTimeout(() => { void pollExisting(generation); }, pollMs); }
  };
  return {
    onGlobalSftpTransferEvent(callback: (event: TransferEvent) => void) {
      const first = listeners.size === 0;
      listeners.add(callback);
      if (first && bindings?.List) { observed.clear(); void pollExisting(++pollGeneration); }
      return () => { listeners.delete(callback); if (!listeners.size) { pollGeneration++; if (timer) clearTimeout(timer); timer = undefined; } };
    },
    async startStreamTransfer(options: StreamOptions): Promise<{ transferId: string; totalBytes?: number; cancelled?: boolean }> {
      const service = requireBindings();
      const id = options.transferId;
      if (active.has(id)) throw new Error(`Transfer already running: ${id}`);
      if (options.sourceType === 'sftp' && !options.sourceSftpId) throw new Error('Missing source SFTP session');
      if (options.targetType === 'sftp' && !options.targetSftpId) throw new Error('Missing target SFTP session');
      active.add(id);
      const request = { taskId: id,
        sourceSessionId: options.sourceType === 'sftp' ? options.sourceSftpId! : '',
        targetSessionId: options.targetType === 'sftp' ? options.targetSftpId! : '',
        sourcePath: options.sourcePath, targetPath: options.targetPath,
        sourceHostId: options.sourceHostId, targetHostId: options.targetHostId,
        parentTaskId: options.parentTaskId, directoryEntryIndex: options.directoryEntryIndex, directoryEntryIdentity: options.directoryEntryIdentity };
      try {
        let progress = await service.Start(request);
        for (;;) {
          publish({ ...request, ...progress });
          if (progress.state === 'failed') throw new Error(progress.error || 'Transfer failed');
          if (progress.state === 'completed') return { transferId: id, totalBytes: progress.totalBytes };
          if (progress.state === 'cancelled') return { transferId: id, cancelled: true };
          await new Promise(resolve => setTimeout(resolve, pollMs));
          progress = await service.Progress(id);
        }
      } catch (error) {
        if (observed.get(id)?.state !== 'failed') emit({ transferId: id, type: 'failed', error: error instanceof Error ? error.message : String(error) });
        throw error;
      } finally { active.delete(id); }
    },
    async startCompressedUpload(options: Parameters<NonNullable<NetcattyBridge['startCompressedUpload']>>[0]) {
      const id=options.compressionId;
      try {
        const service=requireBindings();
        if (!service.StartCompressed) throw new Error('Compressed upload scheduler is unavailable');
        if (active.has(id)) throw new Error(`Transfer already running: ${id}`);
        active.add(id);
        let progress=await service.StartCompressed({ taskId:id, sourceSessionId:'', targetSessionId:options.sftpId, sourcePath:options.folderPath,
          targetPath:`${options.targetPath.replace(/\/$/,'')}/${options.folderName}.zip` });
        for (;;) {
          publish(progress);
          if (progress.state==='completed') return { compressionId:id, success:true };
          if (progress.state==='cancelled') return { compressionId:id, success:false, error:'Cancelled' };
          if (progress.state==='failed') throw new Error(progress.error || 'Compressed upload failed');
          await new Promise(resolve=>setTimeout(resolve,pollMs)); progress=await service.Progress(id);
        }
      } catch(error) { return { compressionId:id, success:false, error:error instanceof Error ? error.message : String(error) }; }
      finally { active.delete(id); }
    },
    async pauseTransfer(id: string) {
      try {
        const service = requireBindings();
        await service.Pause(id);
        const progress = await service.Progress(id);
        publish(progress);
        return { success: true, checkpointBytes: progress.doneBytes, ...(progress.lifecycleEpoch === undefined ? {} : { lifecycleEpoch: progress.lifecycleEpoch }) };
      } catch (error) { return { success: false, reason: error instanceof Error ? error.message : String(error) }; }
    },
    async resumeTransfer(id: string) {
      try {
        const service = requireBindings();
        await service.Resume(id);
        const progress = await service.Progress(id);
        publish(progress);
        return { success: true, ...(progress.lifecycleEpoch === undefined ? {} : { lifecycleEpoch: progress.lifecycleEpoch }) };
      } catch (error) { return { success: false, reason: error instanceof Error ? error.message : String(error) }; }
    },
    async cancelTransfer(id: string) { await requireBindings().Cancel(id); },
  };
}
