type Stream = NonNullable<NetcattyBridge['startStreamTransfer']>;
export interface NativeFileBindings {
    TempFilePath?: (name: string) => Promise<string>;
    ValidateTempFile?: (path: string) => Promise<unknown>;
    DeleteTempFile?: (path: string) => Promise<unknown>;
    OpenWithSystemDefault?: (path: string) => Promise<unknown>;
    OpenWithApplication?: (path: string, application: string) => Promise<unknown>;
}
export interface NativeFileDialogs {
    OpenFile: (options: Record<string, unknown>) => Promise<string | string[]>;
    SaveFile: (options: Record<string, unknown>) => Promise<string>;
}
const selectedPath = (value: string | string[] | undefined) => (typeof value === 'string' ? value : value?.[0]) || null;
const filtersFor = (filters?: Array<{
    name: string;
    extensions: string[];
}>) => filters?.map(f => ({ DisplayName: f.name, Pattern: f.extensions.map(ext => ext === '*' ? '*' : `*.${ext}`).join(';') }));
const pathOptions = (path?: string) => {
    if (!path)
        return {};
    const index = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'));
    return index < 0 ? { Filename: path } : { Directory: path.slice(0, index + (index === 0 || index === 2 && path[1] === ':' ? 1 : 0)), Filename: path.slice(index + 1) };
};
export function createNativeFileActions(files: NativeFileBindings | undefined, dialogs: NativeFileDialogs | undefined, transfer: Stream, platform: string) {
    return {
        async selectApplication() {
            if (!dialogs)
                throw new Error('Native application picker unavailable');
            const path = selectedPath(await dialogs.OpenFile({ Title: 'Select application', CanChooseFiles: true, CanChooseDirectories: false,
                ...(platform === 'win32' ? { Filters: [{ DisplayName: 'Applications', Pattern: '*.exe' }] } : {}),
            }));
            return path ? { path, name: path.split(/[\\/]/).pop()! } : null;
        },
        async selectFile(title?: string, defaultPath?: string, filters?: Array<{
            name: string;
            extensions: string[];
        }>) {
            if (!dialogs)
                throw new Error('Native file picker unavailable');
            return selectedPath(await dialogs.OpenFile({ Title: title, ...pathOptions(defaultPath), Filters: filtersFor(filters), CanChooseFiles: true, CanChooseDirectories: false }));
        },
        async selectDirectory(title?: string, defaultPath?: string) {
            if (!dialogs)
                throw new Error('Native directory picker unavailable');
            return selectedPath(await dialogs.OpenFile({ Title: title, Directory: defaultPath, CanChooseFiles: false, CanChooseDirectories: true }));
        },
        async showSaveDialog(defaultPath: string, filters?: Array<{
            name: string;
            extensions: string[];
        }>) {
            if (!dialogs)
                throw new Error('Native save dialog unavailable');
            return await dialogs.SaveFile({ ...pathOptions(defaultPath), Filters: filtersFor(filters) }) || null;
        },
        async openWithSystemDefault(path: string) {
            try {
                if (!files?.OpenWithSystemDefault)
                    throw new Error('System default opening unavailable');
                await files.OpenWithSystemDefault(path);
                return { success: true };
            }
            catch (error) {
                return { success: false, error: error instanceof Error ? error.message : String(error) };
            }
        },
        async openWithApplication(path: string, app: string) {
            if (!files?.OpenWithApplication)
                throw new Error('Application opening unavailable');
            await files.OpenWithApplication(path, app);
            return true;
        },
        async deleteTempFile(path: string) {
            if (!files?.DeleteTempFile)
                throw new Error('Managed temp cleanup unavailable');
            await files.DeleteTempFile(path);
            return { success: true };
        },
        async downloadSftpToTempWithProgress(sftpId: string, remotePath: string, fileName: string, encoding: Parameters<NonNullable<NetcattyBridge['downloadSftpToTempWithProgress']>>[3], transferId: string) {
            if (!files?.TempFilePath || !files.ValidateTempFile || !files.DeleteTempFile)
                throw new Error('Managed temp download unavailable');
            const localPath = await files.TempFilePath(fileName);
            try {
                const result = await transfer({ transferId, sourceType: 'sftp', sourceSftpId: sftpId, sourcePath: remotePath, sourceEncoding: encoding, targetType: 'local', targetPath: localPath });
                if (result.error)
                    throw new Error(result.error);
                if (result.cancelled) {
                    await files.DeleteTempFile(localPath);
                    return { localPath: '', cancelled: true };
                }
                await files.ValidateTempFile(localPath);
                return { localPath, cancelled: false };
            }
            catch (error) {
                await files.DeleteTempFile(localPath).catch(() => undefined);
                throw error;
            }
        },
    };
}
