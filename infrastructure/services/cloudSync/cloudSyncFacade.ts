import type { SyncedFile } from '../../../domain/sync';
import { getActiveRuntimeClient } from '../../runtime/runtimeClient';
import { netcattyBridge } from '../netcattyBridge';

type FileOptions = { accessToken: string; fileId?: string; fileName?: string; syncedFile?: SyncedFile; revision?: string };
type FileResult = { fileId: string | null };
type DownloadResult = { syncedFile: SyncedFile | null };
type UserInfo = { id: string; email: string; name: string; picture?: string };

// Additional typed GitHub ports fill the renderer-only REST gap. Kept beside
// this facade until the coordinator regenerates the central port inventory.
declare global {
  interface NetcattyBridge {
    githubGetUserInfo?(options: { accessToken: string }): Promise<UserInfo>;
    githubFindSyncFile?(options: FileOptions): Promise<FileResult>;
    githubUploadSyncFile?(options: FileOptions): Promise<FileResult>;
    githubDownloadSyncFile?(options: FileOptions): Promise<DownloadResult>;
    githubDeleteSyncFile?(options: FileOptions): Promise<{ ok: true }>;
    githubGetGistHistory?(options: FileOptions): Promise<Array<{ version: string; date: string }>>;
    googleDriveGetRevisionHistory?(options: FileOptions): Promise<Array<{ version: string; date: string }>>;
  }
}

export type CloudOAuthFacade = Required<Pick<NetcattyBridge,
  | 'prepareOAuthCallback' | 'awaitOAuthCallback' | 'cancelOAuthCallback' | 'openExternal'
  | 'githubStartDeviceFlow' | 'githubPollDeviceFlowToken' | 'githubCancelDeviceFlowPoll' | 'githubDownloadGistRawContent'
  | 'githubGetUserInfo' | 'githubFindSyncFile' | 'githubUploadSyncFile' | 'githubDownloadSyncFile' | 'githubDeleteSyncFile' | 'githubGetGistHistory'
  | 'googleDriveGetRevisionHistory'
  | 'googleExchangeCodeForTokens' | 'googleRefreshAccessToken' | 'googleGetUserInfo'
  | 'googleDriveFindSyncFile' | 'googleDriveCreateSyncFile' | 'googleDriveUpdateSyncFile' | 'googleDriveDownloadSyncFile' | 'googleDriveDeleteSyncFile'
  | 'onedriveExchangeCodeForTokens' | 'onedriveRefreshAccessToken' | 'onedriveGetUserInfo'
  | 'onedriveFindSyncFile' | 'onedriveUploadSyncFile' | 'onedriveDownloadSyncFile' | 'onedriveDeleteSyncFile'
>>;

type WireResult<T> = T extends { ok: true } ? { ok: boolean }
  : T extends { refreshToken: string } ? Omit<T, 'refreshToken'> & { refreshToken?: string }
  : T;
type BindingMethods = { [K in keyof CloudOAuthFacade as Capitalize<K>]:
  (...args: Parameters<CloudOAuthFacade[K]>) => Promise<WireResult<Awaited<ReturnType<CloudOAuthFacade[K]>>>> };
export type CloudOAuthBindings = Omit<BindingMethods, 'OpenExternal' | 'AwaitOAuthCallback' | 'CancelOAuthCallback' | 'GithubStartDeviceFlow' | 'GoogleDriveCreateSyncFile'> & {
  OpenOAuthExternal(url: string): Promise<void>;
  AwaitOAuthCallback(expectedState: string, sessionId: string): Promise<{ code: string; state?: string }>;
  CancelOAuthCallback(sessionId: string): Promise<void>;
  GithubStartDeviceFlow(options: { clientId?: string; scope?: string }): ReturnType<CloudOAuthFacade['githubStartDeviceFlow']>;
  GoogleDriveCreateSyncFile(options: Parameters<CloudOAuthFacade['googleDriveCreateSyncFile']>[0]): Promise<FileResult>;
};

/** Generated bindings are injected by the Wails boundary, never imported here. */
export function createCloudOAuthFacade(bindings: CloudOAuthBindings): CloudOAuthFacade {
  const requireOK = async (result: Promise<{ ok: boolean }>): Promise<{ ok: true }> => {
    if (!(await result).ok) throw new Error('Native cloud operation did not complete');
    return { ok: true };
  };
  return {
    prepareOAuthCallback: () => bindings.PrepareOAuthCallback(),
    awaitOAuthCallback: (state, sessionId) => bindings.AwaitOAuthCallback(state ?? '', sessionId ?? ''),
    cancelOAuthCallback: sessionId => bindings.CancelOAuthCallback(sessionId ?? ''),
    openExternal: url => bindings.OpenOAuthExternal(url),
    githubStartDeviceFlow: options => bindings.GithubStartDeviceFlow(options ?? {}),
    githubPollDeviceFlowToken: options => bindings.GithubPollDeviceFlowToken(options),
    githubCancelDeviceFlowPoll: id => bindings.GithubCancelDeviceFlowPoll(id),
    githubDownloadGistRawContent: options => bindings.GithubDownloadGistRawContent(options),
    githubGetUserInfo: options => bindings.GithubGetUserInfo(options),
    githubFindSyncFile: options => bindings.GithubFindSyncFile(options),
    githubUploadSyncFile: options => bindings.GithubUploadSyncFile(options),
    githubDownloadSyncFile: options => bindings.GithubDownloadSyncFile(options),
    githubDeleteSyncFile: options => requireOK(bindings.GithubDeleteSyncFile(options)),
    githubGetGistHistory: options => bindings.GithubGetGistHistory(options),
    googleDriveGetRevisionHistory: options => bindings.GoogleDriveGetRevisionHistory(options),
    googleExchangeCodeForTokens: options => bindings.GoogleExchangeCodeForTokens(options),
    googleRefreshAccessToken: async options => {
      const tokens = await bindings.GoogleRefreshAccessToken(options);
      return { ...tokens, refreshToken: tokens.refreshToken || options.refreshToken };
    },
    googleGetUserInfo: options => bindings.GoogleGetUserInfo(options),
    googleDriveFindSyncFile: options => bindings.GoogleDriveFindSyncFile(options),
    googleDriveCreateSyncFile: async options => {
      const result = await bindings.GoogleDriveCreateSyncFile(options);
      if (!result.fileId) throw new Error('Google Drive did not return a file ID');
      return { fileId: result.fileId };
    },
    googleDriveUpdateSyncFile: options => requireOK(bindings.GoogleDriveUpdateSyncFile(options)),
    googleDriveDownloadSyncFile: options => bindings.GoogleDriveDownloadSyncFile(options),
    googleDriveDeleteSyncFile: options => requireOK(bindings.GoogleDriveDeleteSyncFile(options)),
    onedriveExchangeCodeForTokens: options => bindings.OnedriveExchangeCodeForTokens(options),
    onedriveRefreshAccessToken: async options => {
      const tokens = await bindings.OnedriveRefreshAccessToken(options);
      return { ...tokens, refreshToken: tokens.refreshToken || options.refreshToken };
    },
    onedriveGetUserInfo: options => bindings.OnedriveGetUserInfo(options),
    onedriveFindSyncFile: options => bindings.OnedriveFindSyncFile(options),
    onedriveUploadSyncFile: options => bindings.OnedriveUploadSyncFile(options),
    onedriveDownloadSyncFile: options => bindings.OnedriveDownloadSyncFile(options),
    onedriveDeleteSyncFile: options => requireOK(bindings.OnedriveDeleteSyncFile(options)),
  };
}

export function nativeCloudSyncRequired(): boolean {
  return typeof window !== 'undefined' && '_wails' in window;
}

/** Wails never falls back to renderer fetch when a provider port is missing. */
export const cloudSyncBridge = {
  get(): Partial<NetcattyBridge> | undefined {
    if (!nativeCloudSyncRequired()) return netcattyBridge.get();
    const port = getActiveRuntimeClient()?.sync;
    return new Proxy(port ?? {}, {
      get(target, key) {
        const value = Reflect.get(target, key);
        if (typeof value !== 'function') throw new Error(`Native cloud sync method unavailable: ${String(key)}`);
        return value;
      },
    });
  },
};
