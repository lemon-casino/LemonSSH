import type { ZmodemDragDropFile } from '../../../lib/zmodemDragDrop';

type ZmodemEvent = Parameters<Parameters<NonNullable<NetcattyBridge['onZmodemEvent']>>[1]>[0];
export type ZmodemTerminalBindings = {
  SendZmodem?: (sessionID: string, filePath: string, remoteName: string, command: string) => Promise<unknown>;
  ReceiveZmodem?: (sessionID: string, destinationDir: string) => Promise<unknown>;
  CancelZmodem?: (sessionID: string) => Promise<unknown>;
};
type StagingBindings = {
  StageBegin?: (name: string) => Promise<string>;
  StageAppend?: (path: string, offset: number, data: string) => Promise<unknown>;
  StageDiscard?: (path: string) => Promise<unknown>;
};

export function createZmodemBridge(
  terminal: ZmodemTerminalBindings,
  filesystem?: StagingBindings,
  eventsOn?: (name: string, callback: (event: unknown) => void) => () => void,
  selectDirectory?: () => Promise<string | null>,
) {
  const pendingDownloads = new Set<string>();
  const subscribers = new Map<string, number>();
  async function acceptDownload(sessionID: string): Promise<void> {
    if (pendingDownloads.has(sessionID) || !selectDirectory || !terminal.ReceiveZmodem) return;
    pendingDownloads.add(sessionID);
    try {
      const directory = await selectDirectory();
      if (!directory || !subscribers.get(sessionID)) {
        await terminal.CancelZmodem?.(sessionID);
        return;
      }
      await terminal.ReceiveZmodem(sessionID, directory);
    } catch (error) {
      console.error('ZMODEM automatic receive failed', error);
      await terminal.CancelZmodem?.(sessionID).catch(() => undefined);
    } finally { pendingDownloads.delete(sessionID); }
  }
  return {
    async startZmodemDragDropUpload(sessionID: string, files: ZmodemDragDropFile[], uploadCommand = 'rz'): Promise<{ success: boolean; error?: string }> {
      if (!terminal.SendZmodem) return { success: false, error: 'ZMODEM upload unavailable' };
      if (!files.length) return { success: false, error: 'No files to send' };
      try {
        for (const file of files) {
          let staged: string | undefined;
          try {
            let path = file.path;
            if (!path) {
              if (!file.data) throw new Error('Upload file has neither a path nor data');
              if (!filesystem?.StageBegin || !filesystem.StageAppend || !filesystem.StageDiscard) throw new Error('Staged uploads unavailable');
              staged = await filesystem.StageBegin(file.name || 'upload.bin');
              path = staged;
              const bytes = new Uint8Array(file.data);
              for (let offset = 0; offset < bytes.length; offset += 4 * 1024 * 1024) {
                const chunk = bytes.subarray(offset, offset + 4 * 1024 * 1024);
                let binary = '';
                for (const byte of chunk) binary += String.fromCharCode(byte);
                await filesystem.StageAppend(staged, offset, btoa(binary));
              }
            }
            // Go captures the raw stream before writing this command. Do not
            // write rz separately via the terminal bridge (negotiation race).
            await terminal.SendZmodem(sessionID, path, file.remoteName || file.name, uploadCommand.replace(/[\r\n]+$/, '') || 'rz');
          } finally {
            if (staged) await filesystem!.StageDiscard!(staged);
          }
        }
        return { success: true };
      } catch (error) { return { success: false, error: String(error) }; }
    },
    async receiveZmodem(sessionID: string, destinationDir: string): Promise<{ success: boolean; error?: string }> {
      if (!terminal.ReceiveZmodem) return { success: false, error: 'ZMODEM receive unavailable' };
      try { await terminal.ReceiveZmodem(sessionID, destinationDir); return { success: true }; }
      catch (error) { return { success: false, error: String(error) }; }
    },
    onZmodemEvent(sessionID: string, receive: (event: ZmodemEvent) => void): () => void {
      subscribers.set(sessionID, (subscribers.get(sessionID) ?? 0) + 1);
      let disposed = false;
      const unsubscribe = eventsOn?.('terminal:zmodem', event => {
        if (disposed) return;
        const payload = (event as { data?: unknown })?.data ?? event;
        if (!payload || typeof payload !== 'object') return;
        const data = payload as ZmodemEvent;
        if (data.sessionId !== sessionID || !['detect', 'progress', 'complete', 'error'].includes(data.type)) return;
        if (data.type === 'detect' && data.transferType === 'download' && !data.filename) void acceptDownload(sessionID);
        receive(data);
      }) ?? (() => undefined);
      return () => {
        if (disposed) return;
        disposed = true;
        unsubscribe();
        const count = (subscribers.get(sessionID) ?? 1) - 1;
        if (count) subscribers.set(sessionID, count);
        else subscribers.delete(sessionID);
      };
    },
  };
}
