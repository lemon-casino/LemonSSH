import {
  PLUGIN_WASM_ABI_VERSION,
  PLUGIN_WASM_DEFAULT_TIMEOUT_MS,
  PLUGIN_WASM_EXPORT_ALLOC,
  PLUGIN_WASM_EXPORT_DISPATCH,
  PLUGIN_WASM_EXPORT_FREE,
  PLUGIN_WASM_HOST_IMPORT_STATUS,
  PLUGIN_WASM_HOST_MODULE,
  PLUGIN_WASM_IMPORT_HOST_LOG,
  PLUGIN_WASM_IMPORT_HOST_SETTING_GET,
  PLUGIN_WASM_MAX_REQUEST_BYTES,
  PLUGIN_WASM_MAX_RESPONSE_BYTES,
} from "@lemonssh/plugin-contract";

import type {
  AuthenticationBeginPayload,
  AuthenticationResponsePayload,
  AuthenticationResult,
  ConnectionConfigurationPayload,
  ConnectionControlResult,
  ConnectionControlPayload,
  ConnectionOpenPayload,
  ConnectionOpenResult,
  ConnectionProbeResult,
  ConnectionResizePayload,
  ConnectionSignalPayload,
  ConnectionStatusResult,
  ConnectionValidateResult,
  CredentialRef,
  FeatureId,
  ImporterDetectPayload,
  ImporterDetectResult,
  ImporterParsePayload,
  ImporterParseResult,
  JsonValue,
  LocalizedText,
  PluginErrorData,
  PluginErrorName,
  PluginId,
  ProviderKind,
  PluginWireErrorCode,
  RpcErrorObject,
  SecretLeaseRef,
  SecretRef,
  SemanticVersion,
  SyncCapabilitiesResult,
  SyncConnectPayload,
  SyncConnectResult,
  SyncDeleteObjectPayload,
  SyncDeleteObjectResult,
  SyncDisconnectPayload,
  SyncDisconnectResult,
  SyncGetAccountPayload,
  SyncGetAccountResult,
  SyncGetCapabilitiesPayload,
  SyncReadObjectPayload,
  SyncReadObjectResult,
  SyncWriteObjectPayload,
  SyncWriteObjectResult,
  TerminalSessionSnapshot,
  WasmDispatchError,
  WasmDispatchRequest,
  WasmDispatchResponse,
} from "@lemonssh/plugin-contract";

export type * from "@lemonssh/plugin-contract";

export interface Disposable {
  dispose(): void;
}

export type CancellationListener = () => void;

export interface CancellationToken {
  readonly isCancellationRequested: boolean;
  onCancellationRequested(listener: CancellationListener): Disposable;
}

export interface PluginLogger {
  debug(message: string, fields?: Readonly<Record<string, JsonValue>>): void;
  info(message: string, fields?: Readonly<Record<string, JsonValue>>): void;
  warn(message: string, fields?: Readonly<Record<string, JsonValue>>): void;
  error(message: string, fields?: Readonly<Record<string, JsonValue>>): void;
}

export interface PluginKeyValueStore {
  get<T extends JsonValue>(key: string): Promise<T | undefined>;
  set(key: string, value: JsonValue): Promise<void>;
  delete(key: string): Promise<void>;
  keys(): Promise<readonly string[]>;
}

export interface PluginSecretStore {
  get(key: string): Promise<SecretRef | undefined>;
  set(key: string, value: string): Promise<SecretRef>;
  delete(key: string): Promise<void>;
}

export interface PluginSettingOptions {
  readonly scopeId?: string;
}

export interface PluginSettingChangeEvent {
  readonly settingId: string;
  readonly scope: string;
  readonly scopeId: string;
  readonly source: "host" | "plugin";
}

export interface PluginSettings {
  get<T extends JsonValue | SecretRef>(settingId: string, options?: PluginSettingOptions): Promise<T | undefined>;
  update(settingId: string, value: JsonValue, options?: PluginSettingOptions): Promise<Readonly<{ restartRequired: boolean }>>;
  onDidChange(listener: (event: PluginSettingChangeEvent) => void): Disposable;
}

export interface PluginCommandInvocation {
  readonly source: "host" | "plugin" | string;
  readonly context?: Readonly<Record<string, JsonValue>>;
}

export type PluginCommandHandler = (args: JsonValue | undefined, invocation: PluginCommandInvocation) => JsonValue | void | Promise<JsonValue | void>;

export interface PluginCommands {
  registerCommand(commandId: string, handler: PluginCommandHandler): Disposable;
  executeCommand<T extends JsonValue = JsonValue>(commandId: string, args?: JsonValue): Promise<T>;
}

export interface PluginContextKeys {
  set(key: string, value: JsonValue): Promise<void>;
}

export interface PluginViews {
  onDidReceiveMessage(viewId: string, listener: (message: JsonValue) => void): Disposable;
  postMessage(viewId: string, message: JsonValue): void;
  getState<T extends JsonValue = JsonValue>(viewId: string, scopeId: string): Promise<T | undefined>;
  setState(viewId: string, scopeId: string, state: JsonValue): Promise<void>;
}

export interface PluginProviderInvocation<TPayload extends JsonValue = JsonValue> {
  readonly providerId: string;
  readonly kind: ProviderKind;
  readonly operation: string;
  readonly requestId: string;
  readonly payload: TPayload | undefined;
  readonly deadlineMs: number | undefined;
  readonly cancellationToken: CancellationToken;
}

export type PluginProviderHandler<
  TPayload extends JsonValue = JsonValue,
  TResult extends JsonValue = JsonValue,
> = (invocation: PluginProviderInvocation<TPayload>) => TResult | void | Promise<TResult | void>;

type TypedPluginProviderInvocation<TPayload> = Omit<PluginProviderInvocation, "payload"> & {
  readonly payload: TPayload;
};

type ProviderHandlerForKind<
  K extends ProviderKind,
  TPayload extends JsonValue,
  TResult extends JsonValue,
> = K extends TerminalInterceptorKind
  ? TerminalInterceptorHandler
  : K extends OrdinaryTerminalProviderKind
    ? OrdinaryTerminalProviderHandler<K>
    : K extends "connection"
      ? ConnectionProviderHandler
      : K extends "authentication"
        ? AuthenticationProviderHandler
        : K extends "importer"
          ? ImporterProviderHandler
          : K extends "sync"
            ? SyncProviderHandler
    : PluginProviderHandler<TPayload, TResult>;

export interface PluginProviders {
  register<K extends OrdinaryTerminalProviderKind>(
    providerId: string,
    kind: K,
    handler: OrdinaryTerminalProviderHandler<K>,
  ): Disposable;
  register(
    providerId: string,
    kind: TerminalInterceptorKind,
    handler: TerminalInterceptorHandler,
  ): Disposable;
  register(
    providerId: string,
    kind: "connection",
    handler: ConnectionProviderHandler,
  ): Disposable;
  register(
    providerId: string,
    kind: "authentication",
    handler: AuthenticationProviderHandler,
  ): Disposable;
  register(
    providerId: string,
    kind: "importer",
    handler: ImporterProviderHandler,
  ): Disposable;
  register(
    providerId: string,
    kind: "sync",
    handler: SyncProviderHandler,
  ): Disposable;
  register<TPayload extends JsonValue = JsonValue, TResult extends JsonValue = JsonValue>(
    providerId: string,
    kind: Exclude<
      ProviderKind,
      TerminalInterceptorKind | OrdinaryTerminalProviderKind | "connection" | "authentication" | "importer" | "sync"
    >,
    handler: PluginProviderHandler<TPayload, TResult>,
  ): Disposable;
  register<
    K extends ProviderKind,
    TPayload extends JsonValue = JsonValue,
    TResult extends JsonValue = JsonValue,
  >(
    providerId: string,
    kind: K,
    handler: ProviderHandlerForKind<NoInfer<K>, TPayload, TResult>,
  ): Disposable;
}

export type TerminalInterceptorKind = "terminal.interceptor.input" | "terminal.interceptor.output";

export interface TerminalInterceptorInvocation {
  readonly providerId: string;
  readonly kind: TerminalInterceptorKind;
  readonly direction: "input" | "output";
  readonly sequence: number;
  readonly session: TerminalSessionSnapshot;
  /** UTF-8 terminal data. The buffer is owned by this invocation. */
  readonly data: Uint8Array;
}

export type TerminalInterceptorHandler = (
  invocation: TerminalInterceptorInvocation,
) => Uint8Array | ArrayBuffer | Promise<Uint8Array | ArrayBuffer>;

export interface TerminalSessionEvent {
  readonly type:
    | "snapshot"
    | "created"
    | "connected"
    | "reconnected"
    | "cwdChanged"
    | "titleChanged"
    | "resized"
    | "alternateScreenChanged"
    | "commandSubmitted"
    | "commandCompleted"
    | "disconnected"
    | "disposed";
  readonly session: TerminalSessionSnapshot;
  readonly exitCode?: number;
}

export interface TerminalProviderPayload {
  /** Immutable host snapshot bound to this exact invocation. */
  readonly session: TerminalSessionSnapshot;
}

export interface TerminalCompletionPayload extends TerminalProviderPayload {
  readonly input: string;
  readonly cursor: number;
  readonly hostOs: "linux" | "windows" | "macos";
  readonly cwdSource: "prompt" | "fallback" | "none" | null;
  readonly maximum: number;
}

export interface TerminalCompletionItem {
  readonly text: string;
  /** When supplied, it must equal text; the host always displays the inserted command. */
  readonly displayText?: string;
  readonly description?: string;
  readonly score?: number;
}

export interface TerminalCompletionResult {
  readonly items: readonly TerminalCompletionItem[];
}

export interface TerminalDecorationPayload extends TerminalProviderPayload {
  readonly reason: string;
}

export interface TerminalDecorationRule {
  readonly id: string;
  readonly label: string;
  readonly patterns: readonly string[];
  readonly color: string;
}

export interface TerminalDecorationResult {
  readonly rules: readonly TerminalDecorationRule[];
}

export interface TerminalTextRange {
  readonly start: number;
  readonly length: number;
}

export interface TerminalLinkItem extends TerminalTextRange {
  readonly uri: string;
  readonly label?: string;
}

export interface TerminalLineProviderPayload extends TerminalProviderPayload {
  readonly line: string;
  readonly bufferLineNumber: number;
}

export interface TerminalLinkResult {
  readonly links: readonly TerminalLinkItem[];
}

export interface TerminalHoverItem extends TerminalTextRange {
  readonly contents: string;
}

export interface TerminalHoverResult {
  readonly hovers: readonly TerminalHoverItem[];
}

export interface TerminalMatcherLine {
  readonly lineId: string;
  readonly line: string;
  readonly bufferLineNumber: number;
}

export interface TerminalMatcherPayload extends TerminalProviderPayload {
  readonly lines: readonly TerminalMatcherLine[];
}

export interface TerminalOutputMatchItem extends TerminalTextRange {
  /** Host-provided line identifier from the provideMatches request batch. */
  readonly lineId: string;
  readonly label: string;
  readonly severity?: "info" | "warning" | "error" | "success";
  readonly color?: string;
}

export interface TerminalMatcherResult {
  readonly matches: readonly TerminalOutputMatchItem[];
}

export interface TerminalAnnotationItem {
  readonly text: string;
  readonly color?: string;
}

export interface TerminalSemanticResult {
  readonly classification?: string;
  readonly description?: string;
  readonly destructive?: boolean;
  readonly idempotent?: boolean;
  readonly annotations?: readonly TerminalAnnotationItem[];
}

export interface TerminalSemanticPayload extends TerminalProviderPayload {
  readonly command: string;
}

export interface TerminalPromptPayload extends TerminalProviderPayload {
  readonly reason: "commandCompleted";
  readonly promptLine?: string;
  readonly bufferLineNumber?: number;
}

export interface TerminalPromptResult {
  readonly annotations: readonly TerminalAnnotationItem[];
}

export interface TerminalBackgroundLayer {
  readonly id: string;
  readonly color: string;
  /** Defaults to a host-owned safe opacity of 0.15. */
  readonly opacity?: number;
}

export interface TerminalBackgroundResult {
  readonly layers: readonly TerminalBackgroundLayer[];
  /** Optional bounded host refresh cadence. The host clamps this to 250-60000 ms. */
  readonly refreshAfterMs?: number;
}

export interface TerminalBackgroundPayload extends TerminalProviderPayload {
  readonly reason: string;
  readonly terminalBackground?: string;
}

export type TerminalThemeColorName =
  | "background" | "foreground" | "cursor" | "selection"
  | "black" | "red" | "green" | "yellow" | "blue" | "magenta" | "cyan" | "white"
  | "brightBlack" | "brightRed" | "brightGreen" | "brightYellow"
  | "brightBlue" | "brightMagenta" | "brightCyan" | "brightWhite";

export interface TerminalThemePayload extends TerminalProviderPayload {
  readonly reason: string;
  readonly currentTheme: {
    readonly type: "dark" | "light";
    readonly colors: Readonly<Record<TerminalThemeColorName, string>>;
  };
}

export interface TerminalThemeResult {
  readonly colors: Readonly<Partial<Record<TerminalThemeColorName, string>>>;
}

export interface OrdinaryTerminalProviderPayloadByKind {
  readonly "terminal.completion": TerminalCompletionPayload;
  readonly "terminal.decoration": TerminalDecorationPayload;
  readonly "terminal.link": TerminalLineProviderPayload;
  readonly "terminal.hover": TerminalLineProviderPayload;
  readonly "terminal.matcher": TerminalMatcherPayload;
  readonly "terminal.semantic": TerminalSemanticPayload;
  readonly "terminal.prompt": TerminalPromptPayload;
  readonly "terminal.background": TerminalBackgroundPayload;
  readonly "terminal.theme": TerminalThemePayload;
}

export interface OrdinaryTerminalProviderResultByKind {
  readonly "terminal.completion": TerminalCompletionResult;
  readonly "terminal.decoration": TerminalDecorationResult;
  readonly "terminal.link": TerminalLinkResult;
  readonly "terminal.hover": TerminalHoverResult;
  readonly "terminal.matcher": TerminalMatcherResult;
  readonly "terminal.semantic": TerminalSemanticResult;
  readonly "terminal.prompt": TerminalPromptResult;
  readonly "terminal.background": TerminalBackgroundResult;
  readonly "terminal.theme": TerminalThemeResult;
}

export interface OrdinaryTerminalProviderOperationByKind {
  readonly "terminal.completion": "provideCompletions";
  readonly "terminal.decoration": "provideDecorations";
  readonly "terminal.link": "provideLinks";
  readonly "terminal.hover": "provideHovers";
  readonly "terminal.matcher": "provideMatches";
  readonly "terminal.semantic": "provideSemantics";
  readonly "terminal.prompt": "provideAnnotations";
  readonly "terminal.background": "provideBackgrounds";
  readonly "terminal.theme": "provideTheme";
}

export type OrdinaryTerminalProviderKind = keyof OrdinaryTerminalProviderPayloadByKind;

export interface OrdinaryTerminalProviderInvocation<K extends OrdinaryTerminalProviderKind> {
  readonly providerId: string;
  readonly kind: K;
  readonly operation: OrdinaryTerminalProviderOperationByKind[K];
  readonly requestId: string;
  readonly payload: OrdinaryTerminalProviderPayloadByKind[K];
  readonly deadlineMs: number | undefined;
  readonly cancellationToken: CancellationToken;
}

export type OrdinaryTerminalProviderHandler<K extends OrdinaryTerminalProviderKind> = (
  invocation: OrdinaryTerminalProviderInvocation<K>,
) => OrdinaryTerminalProviderResultByKind[K] | Promise<OrdinaryTerminalProviderResultByKind[K]>;

export interface ConnectionProviderInvocationByOperation {
  readonly validateConfiguration: TypedPluginProviderInvocation<ConnectionConfigurationPayload> & {
    readonly kind: "connection";
    readonly operation: "validateConfiguration";
  };
  readonly probe: TypedPluginProviderInvocation<ConnectionConfigurationPayload> & {
    readonly kind: "connection";
    readonly operation: "probe";
  };
  readonly open: TypedPluginProviderInvocation<ConnectionOpenPayload> & {
    readonly kind: "connection";
    readonly operation: "open";
    readonly input: Promise<PluginReadableByteStream>;
    readonly output: PluginWritableByteStream;
  };
  readonly resize: TypedPluginProviderInvocation<ConnectionResizePayload> & {
    readonly kind: "connection";
    readonly operation: "resize";
  };
  readonly signal: TypedPluginProviderInvocation<ConnectionSignalPayload> & {
    readonly kind: "connection";
    readonly operation: "signal";
  };
  readonly reconnect: TypedPluginProviderInvocation<ConnectionControlPayload> & {
    readonly kind: "connection";
    readonly operation: "reconnect";
  };
  readonly close: TypedPluginProviderInvocation<ConnectionControlPayload> & {
    readonly kind: "connection";
    readonly operation: "close";
  };
  readonly getStatus: TypedPluginProviderInvocation<ConnectionControlPayload> & {
    readonly kind: "connection";
    readonly operation: "getStatus";
  };
}

export interface ConnectionProviderResultByOperation {
  readonly validateConfiguration: ConnectionValidateResult;
  readonly probe: ConnectionProbeResult;
  readonly open: ConnectionOpenResult;
  readonly resize: ConnectionControlResult;
  readonly signal: ConnectionControlResult;
  readonly reconnect: ConnectionControlResult;
  readonly close: ConnectionControlResult;
  readonly getStatus: ConnectionStatusResult;
}

export type ConnectionProviderOperation = keyof ConnectionProviderInvocationByOperation;

export type ConnectionProviderInvocation =
  ConnectionProviderInvocationByOperation[ConnectionProviderOperation];

export type ConnectionProviderResult =
  ConnectionProviderResultByOperation[ConnectionProviderOperation];

export type ConnectionProviderOperationHandler<TOperation extends ConnectionProviderOperation> = (
  invocation: ConnectionProviderInvocationByOperation[TOperation],
) => ConnectionProviderResultByOperation[TOperation] | Promise<ConnectionProviderResultByOperation[TOperation]>;

export type ConnectionProviderHandler = Readonly<{
  [TOperation in ConnectionProviderOperation]: ConnectionProviderOperationHandler<TOperation>;
}>;

export type AuthenticationProviderInvocation =
  | (TypedPluginProviderInvocation<AuthenticationBeginPayload> & {
      readonly kind: "authentication";
      readonly operation: "begin";
    })
  | (TypedPluginProviderInvocation<AuthenticationResponsePayload> & {
      readonly kind: "authentication";
      readonly operation: "respond";
    })
  | (TypedPluginProviderInvocation<Readonly<{ operationId: string }>> & {
      readonly kind: "authentication";
      readonly operation: "cancel";
    });

export type AuthenticationProviderHandler = (
  invocation: AuthenticationProviderInvocation,
) => AuthenticationResult | Promise<AuthenticationResult>;

export interface ImporterProviderInvocationByOperation {
  readonly detect: TypedPluginProviderInvocation<ImporterDetectPayload> & {
    readonly kind: "importer";
    readonly operation: "detect";
  };
  readonly parse: TypedPluginProviderInvocation<ImporterParsePayload> & {
    readonly kind: "importer";
    readonly operation: "parse";
    readonly input: Promise<PluginReadableByteStream>;
    readonly output: PluginWritableByteStream;
  };
}

export interface ImporterProviderResultByOperation {
  readonly detect: ImporterDetectResult;
  readonly parse: ImporterParseResult;
}

export type ImporterProviderOperation = keyof ImporterProviderInvocationByOperation;
export type ImporterProviderInvocation =
  ImporterProviderInvocationByOperation[ImporterProviderOperation];
export type ImporterProviderResult =
  ImporterProviderResultByOperation[ImporterProviderOperation];
export type ImporterProviderOperationHandler<TOperation extends ImporterProviderOperation> = (
  invocation: ImporterProviderInvocationByOperation[TOperation],
) => ImporterProviderResultByOperation[TOperation] | Promise<ImporterProviderResultByOperation[TOperation]>;
export type ImporterProviderHandler = Readonly<{
  [TOperation in ImporterProviderOperation]: ImporterProviderOperationHandler<TOperation>;
}>;

export interface SyncProviderInvocationByOperation {
  readonly connect: TypedPluginProviderInvocation<SyncConnectPayload> & {
    readonly kind: "sync";
    readonly operation: "connect";
  };
  readonly disconnect: TypedPluginProviderInvocation<SyncDisconnectPayload | undefined> & {
    readonly kind: "sync";
    readonly operation: "disconnect";
  };
  readonly getAccount: TypedPluginProviderInvocation<SyncGetAccountPayload | undefined> & {
    readonly kind: "sync";
    readonly operation: "getAccount";
  };
  readonly getCapabilities: TypedPluginProviderInvocation<SyncGetCapabilitiesPayload | undefined> & {
    readonly kind: "sync";
    readonly operation: "getCapabilities";
  };
  readonly readObject: TypedPluginProviderInvocation<SyncReadObjectPayload> & {
    readonly kind: "sync";
    readonly operation: "readObject";
    readonly output?: PluginWritableByteStream;
  };
  readonly writeObject: TypedPluginProviderInvocation<SyncWriteObjectPayload> & {
    readonly kind: "sync";
    readonly operation: "writeObject";
    readonly input?: Promise<PluginReadableByteStream>;
  };
  readonly deleteObject: TypedPluginProviderInvocation<SyncDeleteObjectPayload> & {
    readonly kind: "sync";
    readonly operation: "deleteObject";
  };
}

export interface SyncProviderResultByOperation {
  readonly connect: SyncConnectResult;
  readonly disconnect: SyncDisconnectResult;
  readonly getAccount: SyncGetAccountResult;
  readonly getCapabilities: SyncCapabilitiesResult;
  readonly readObject: SyncReadObjectResult;
  readonly writeObject: SyncWriteObjectResult;
  readonly deleteObject: SyncDeleteObjectResult;
}

export type SyncProviderOperation = keyof SyncProviderInvocationByOperation;
export type SyncProviderInvocation =
  SyncProviderInvocationByOperation[SyncProviderOperation];
export type SyncProviderResult =
  SyncProviderResultByOperation[SyncProviderOperation];
export type SyncProviderOperationHandler<TOperation extends SyncProviderOperation> = (
  invocation: SyncProviderInvocationByOperation[TOperation],
) => SyncProviderResultByOperation[TOperation] | Promise<SyncProviderResultByOperation[TOperation]>;
export type SyncProviderHandler = Readonly<{
  [TOperation in SyncProviderOperation]: SyncProviderOperationHandler<TOperation>;
}>;

export interface PluginTerminalSessions {
  onDidChange(listener: (event: TerminalSessionEvent) => void): Disposable;
}

export interface PluginEnvironmentChangeEvent {
  readonly locale: string;
  readonly theme: string;
  readonly reducedMotion: boolean;
  readonly highContrast: boolean;
  readonly themeTokens: Readonly<Record<string, string>>;
}

export interface PluginEnvironment extends PluginEnvironmentChangeEvent {
  onDidChange(listener: (event: PluginEnvironmentChangeEvent) => void): Disposable;
}

export interface PluginCredentialLeaseOptions {
  readonly operationId: string;
  readonly purpose: string;
  readonly ttlMs?: number;
}

export interface PluginCredentialBroker {
  createLease(credential: SecretRef | CredentialRef, options: PluginCredentialLeaseOptions): Promise<SecretLeaseRef>;
}

export interface PluginNetworkRequest {
  readonly url: string;
  readonly method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE" | "HEAD";
  readonly headers?: Readonly<Record<string, string>>;
  readonly body?: Readonly<{ encoding: "utf8" | "base64"; data: string }>;
  readonly timeoutMs?: number;
}

export interface PluginNetworkResponse {
  readonly url: string;
  readonly status: number;
  readonly headers: Readonly<Record<string, string>>;
  readonly body: Readonly<{ encoding: "base64"; data: string }>;
}

export interface PluginNetworkClient {
  request(request: PluginNetworkRequest): Promise<PluginNetworkResponse>;
}

export interface PluginFilesystemEntry {
  readonly name: string;
  readonly kind: "file" | "directory" | "other";
}

export interface PluginFilesystemStat {
  readonly kind: "file" | "directory" | "other";
  readonly size: number;
  readonly modifiedAt: number;
}

export interface PluginFilesystemClient {
  readFile(path: string, options?: Readonly<{ encoding?: "utf8" | "base64"; maxBytes?: number }>): Promise<string>;
  writeFile(path: string, data: string, options: Readonly<{
    encoding?: "utf8" | "base64";
    overwrite: true;
  }>): Promise<void>;
  stat(path: string): Promise<PluginFilesystemStat>;
  readDirectory(path: string): Promise<readonly PluginFilesystemEntry[]>;
}

export interface PluginCompanionRequestOptions {
  readonly timeoutMs?: number;
  /**
   * Operation-bound one-use leases consumed by the host immediately before
   * dispatching this request to the isolated companion. When present, the
   * companion receives `{ payload, credentials }` instead of the raw params.
   */
  readonly credentialLeases?: Readonly<Record<string, SecretLeaseRef>>;
  readonly operationId?: string;
}

export interface PluginCompanionHandle extends Disposable {
  readonly id: string;
  request<T extends JsonValue = JsonValue>(
    method: string,
    params?: JsonValue,
    options?: PluginCompanionRequestOptions,
  ): Promise<T>;
  stop(): Promise<void>;
}

export interface PluginCompanionService {
  start(companionId: string): Promise<PluginCompanionHandle>;
}

export interface PluginReadableByteStream extends Disposable {
  readonly id: string;
  /**
   * Returns the next owned byte chunk or null after a normal end. Calling read
   * again releases receive credit for the previous chunk, so consumers should
   * finish processing one chunk before requesting the next.
   */
  read(): Promise<Uint8Array | null>;
  cancel(): void;
}

export interface PluginWritableByteStream extends Disposable {
  readonly id: string;
  write(data: Uint8Array | ArrayBuffer): Promise<void>;
  end(): Promise<void>;
  fail(error: Readonly<{ message: string }>): void;
  cancel(): void;
}

export interface PluginStreams {
  acceptReadable(streamId: string): Promise<PluginReadableByteStream>;
  openWritable(streamId: string, options?: Readonly<{ windowBytes?: number }>): Promise<PluginWritableByteStream>;
}

export interface PluginContext {
  readonly pluginId: PluginId;
  readonly lemonsshVersion: SemanticVersion;
  readonly apiVersion: SemanticVersion;
  readonly enabledFeatures: ReadonlySet<FeatureId>;
  readonly subscriptions: DisposableStore;
  readonly storage: PluginKeyValueStore;
  readonly settings: PluginSettings;
  readonly commands: PluginCommands;
  readonly contextKeys: PluginContextKeys;
  readonly views: PluginViews;
  readonly providers: PluginProviders;
  readonly terminals: PluginTerminalSessions;
  readonly environment: PluginEnvironment;
  readonly secrets: PluginSecretStore;
  readonly credentials: PluginCredentialBroker;
  readonly network: PluginNetworkClient;
  readonly filesystem: PluginFilesystemClient;
  readonly companions: PluginCompanionService;
  readonly streams: PluginStreams;
  readonly logger: PluginLogger;
}

export interface LemonsshPlugin {
  activate(context: PluginContext): void | Disposable | Promise<void | Disposable>;
  deactivate?(): void | Promise<void>;
}

export type PluginErrorCode = PluginErrorName;

export const PLUGIN_ERROR_WIRE_CODES = {
  cancelled: -32001,
  unknown: -32002,
  invalid_argument: -32003,
  deadline_exceeded: -32004,
  not_found: -32005,
  already_exists: -32006,
  permission_denied: -32007,
  resource_exhausted: -32008,
  failed_precondition: -32009,
  aborted: -32010,
  out_of_range: -32011,
  unsupported: -32012,
  internal: -32013,
  unavailable: -32014,
  data_loss: -32015,
  unauthenticated: -32016,
} as const satisfies Readonly<Record<PluginErrorCode, PluginWireErrorCode>>;

export class PluginError extends Error {
  readonly code: PluginErrorCode;
  readonly details?: JsonValue;

  constructor(code: PluginErrorCode, message: string, details?: JsonValue) {
    super(message);
    this.name = "PluginError";
    this.code = code;
    this.details = details;
  }
}

export function pluginErrorToRpcError(error: PluginError): RpcErrorObject {
  const data: PluginErrorData = error.details === undefined
    ? { pluginCode: error.code }
    : { pluginCode: error.code, details: error.details };
  return {
    code: PLUGIN_ERROR_WIRE_CODES[error.code],
    message: error.message,
    data,
  };
}

export class CancellationError extends PluginError {
  constructor(message = "The operation was cancelled") {
    super("cancelled", message);
    this.name = "CancellationError";
  }
}

export class DisposableStore implements Disposable {
  readonly #items = new Set<Disposable>();
  #isDisposed = false;

  get isDisposed(): boolean {
    return this.#isDisposed;
  }

  add<T extends Disposable>(disposable: T): T {
    if (this.#isDisposed) {
      disposable.dispose();
      throw new PluginError("unavailable", "Cannot add to a disposed DisposableStore");
    }
    this.#items.add(disposable);
    return disposable;
  }

  delete(disposable: Disposable): boolean {
    return this.#items.delete(disposable);
  }

  clear(): void {
    const items = [...this.#items];
    this.#items.clear();
    const errors: unknown[] = [];
    for (const item of items) {
      try {
        item.dispose();
      } catch (error) {
        errors.push(error);
      }
    }
    if (errors.length > 0) {
      throw new AggregateError(errors, "One or more plugin disposables failed");
    }
  }

  dispose(): void {
    if (this.#isDisposed) return;
    this.#isDisposed = true;
    this.clear();
  }
}

class MutableCancellationToken implements CancellationToken {
  readonly #listeners = new Set<CancellationListener>();
  #isCancellationRequested = false;

  get isCancellationRequested(): boolean {
    return this.#isCancellationRequested;
  }

  onCancellationRequested(listener: CancellationListener): Disposable {
    if (this.#isCancellationRequested) {
      queueMicrotask(listener);
      return { dispose() {} };
    }
    this.#listeners.add(listener);
    return {
      dispose: () => {
        this.#listeners.delete(listener);
      },
    };
  }

  cancel(): void {
    if (this.#isCancellationRequested) return;
    this.#isCancellationRequested = true;
    const listeners = [...this.#listeners];
    this.#listeners.clear();
    const errors: unknown[] = [];
    for (const listener of listeners) {
      try {
        listener();
      } catch (error) {
        errors.push(error);
      }
    }
    if (errors.length > 0) {
      throw new AggregateError(errors, "One or more cancellation listeners failed");
    }
  }

  dispose(): void {
    this.#listeners.clear();
  }
}

export class CancellationTokenSource implements Disposable {
  readonly #token = new MutableCancellationToken();
  #isDisposed = false;

  get token(): CancellationToken {
    return this.#token;
  }

  cancel(): void {
    if (!this.#isDisposed) this.#token.cancel();
  }

  dispose(cancel = false): void {
    if (this.#isDisposed) return;
    try {
      if (cancel) this.#token.cancel();
    } finally {
      this.#token.dispose();
      this.#isDisposed = true;
    }
  }
}

export function definePlugin<T extends LemonsshPlugin>(plugin: T): T {
  return plugin;
}

export function throwIfCancellationRequested(token: CancellationToken): void {
  if (token.isCancellationRequested) throw new CancellationError();
}

// ---------------------------------------------------------------------------
// lemonssh-wasm-abi v1 (Go WASM host dispatch channel)
//
// The Go host (internal/plugin/wasm) exchanges one JSON envelope per call
// through the guest's lemonssh_alloc/lemonssh_dispatch/lemonssh_free exports;
// the host module lemonssh provides the broker-gated lemonssh_host_* imports.
// These helpers give tooling and test harnesses a typed encoding surface;
// see docs/plugin-platform/isolated-runtime.md.
// ---------------------------------------------------------------------------

export const WASM_ABI = {
  version: PLUGIN_WASM_ABI_VERSION,
  hostModule: PLUGIN_WASM_HOST_MODULE,
  guestExports: {
    alloc: PLUGIN_WASM_EXPORT_ALLOC,
    free: PLUGIN_WASM_EXPORT_FREE,
    dispatch: PLUGIN_WASM_EXPORT_DISPATCH,
  },
  hostImports: {
    log: PLUGIN_WASM_IMPORT_HOST_LOG,
    settingGet: PLUGIN_WASM_IMPORT_HOST_SETTING_GET,
  },
  maxRequestBytes: PLUGIN_WASM_MAX_REQUEST_BYTES,
  maxResponseBytes: PLUGIN_WASM_MAX_RESPONSE_BYTES,
  defaultTimeoutMs: PLUGIN_WASM_DEFAULT_TIMEOUT_MS,
} as const;

export const WASM_HOST_IMPORT_STATUS = PLUGIN_WASM_HOST_IMPORT_STATUS;

/** Encodes one dispatch request envelope for the host to write into guest memory. */
export function buildWasmDispatchRequest(method: string, payload?: JsonValue): Uint8Array {
  if (method.length === 0) {
    throw new PluginError("invalid_argument", "dispatch method is required");
  }
  const request: WasmDispatchRequest = payload === undefined ? { method } : { method, payload };
  const bytes = new TextEncoder().encode(JSON.stringify(request));
  if (bytes.byteLength > PLUGIN_WASM_MAX_REQUEST_BYTES) {
    throw new PluginError(
      "invalid_argument",
      `dispatch request exceeds ${PLUGIN_WASM_MAX_REQUEST_BYTES} bytes`,
    );
  }
  return bytes;
}

function assertWasmDispatchError(value: unknown, path: string): WasmDispatchError {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new PluginError("internal", `${path} must be an object`);
  }
  const { code, message, data } = value as Record<string, unknown>;
  if (typeof code !== "string" || code.length === 0) {
    throw new PluginError("internal", `${path}.code must be a non-empty string`);
  }
  if (typeof message !== "string") {
    throw new PluginError("internal", `${path}.message must be a string`);
  }
  if (data !== undefined) {
    return { code, message, data } as WasmDispatchError;
  }
  return { code, message };
}

/** Validates an already-parsed dispatch response envelope. */
export function assertWasmDispatchResponse(value: unknown): WasmDispatchResponse {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new PluginError("internal", "dispatch response must be an object");
  }
  const { ok, result, error } = value as Record<string, unknown>;
  if (ok === true) {
    if (error !== undefined) {
      throw new PluginError("internal", "successful dispatch response must not carry an error");
    }
    return result === undefined ? { ok: true } : { ok: true, result } as WasmDispatchResponse;
  }
  if (ok === false) {
    if (result !== undefined) {
      throw new PluginError("internal", "failed dispatch response must not carry a result");
    }
    return { ok: false, error: assertWasmDispatchError(error, "error") };
  }
  throw new PluginError("internal", `dispatch response ok must be boolean, got ${String(ok)}`);
}

/** Decodes and validates the response payload the host copied out of guest memory. */
export function parseWasmDispatchResponse(bytes: Uint8Array): WasmDispatchResponse {
  if (bytes.byteLength > PLUGIN_WASM_MAX_RESPONSE_BYTES) {
    throw new PluginError(
      "invalid_argument",
      `dispatch response exceeds ${PLUGIN_WASM_MAX_RESPONSE_BYTES} bytes`,
    );
  }
  let value: unknown;
  try {
    value = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
  } catch (error) {
    throw new PluginError(
      "internal",
      "dispatch response is not valid UTF-8 JSON",
      error instanceof Error ? error.message : String(error),
    );
  }
  return assertWasmDispatchResponse(value);
}

// ---------------------------------------------------------------------------
// Declarative view data (host pull channel)
//
// The host renderer pulls ViewDef.Bindings data from an enabled plugin through
// the same WASM dispatch channel using the canonical "view.data" method. The
// request payload is { viewId, bindings }; a successful response result is a
// plain object mapping binding keys to display values (list rows are objects
// keyed by the view's declared columns). When the dispatch fails or omits a
// binding the host falls back to the plugin's declared non-secret settings
// values. See docs/plugin-platform/ui-contributions.md.
// ---------------------------------------------------------------------------

export const VIEW_DATA_DISPATCH_METHOD = "view.data";

/** Mirrors the Go host's ui.maxViewKeys install-time cap. */
export const PLUGIN_VIEW_DATA_MAX_BINDINGS = 64 as const;

export interface PluginViewDataRequest {
  viewId: string;
  bindings: ReadonlyArray<string>;
}

/** Encodes one view.data dispatch request for the host to write into guest memory. */
export function buildPluginViewDataRequest({ viewId, bindings }: PluginViewDataRequest): Uint8Array {
  if (viewId.length === 0) {
    throw new PluginError("invalid_argument", "view data request requires a view id");
  }
  const unique = [...new Set(bindings)].filter((binding) => binding.length > 0);
  if (unique.length > PLUGIN_VIEW_DATA_MAX_BINDINGS) {
    throw new PluginError(
      "invalid_argument",
      `view data request exceeds ${PLUGIN_VIEW_DATA_MAX_BINDINGS} bindings`,
    );
  }
  return buildWasmDispatchRequest(VIEW_DATA_DISPATCH_METHOD, { viewId, bindings: unique });
}

/**
 * Narrows a view.data dispatch response to the binding map the host renderer
 * consumes. Failed envelopes and non-object results yield null so the caller
 * can fall back to settings values.
 */
export function parsePluginViewDataResult(response: WasmDispatchResponse): Record<string, JsonValue> | null {
  if (!response.ok) return null;
  const { result } = response;
  if (result === undefined || result === null || typeof result !== "object" || Array.isArray(result)) {
    return null;
  }
  return result as Record<string, JsonValue>;
}

// ---------------------------------------------------------------------------
// Provider registry protocol (host pull channel)
//
// The Go host registry (internal/plugin/providers) enumerates provider
// declarations over the same lemonssh-wasm-abi v1 dispatch channel:
//   providers.list          -> {"providers": [ProviderContribution, …]}
//   provider.invoke         -> one provider operation; the result is the
//                              operation-specific JSON value
//   provider.sessionEvent   -> terminal session lifecycle notification
// A declaration is only accepted while the plugin's manifest declares the
// kind ("provider" permission) and the fail-closed broker holds the grant.
// ---------------------------------------------------------------------------

export const PROVIDERS_LIST_DISPATCH_METHOD = "providers.list";
export const PROVIDER_INVOKE_DISPATCH_METHOD = "provider.invoke";
export const PROVIDER_SESSION_EVENT_DISPATCH_METHOD = "provider.sessionEvent";

/** One provider declaration a plugin returns for providers.list. */
export interface PluginProviderDeclaration {
  readonly id: string;
  readonly label: LocalizedText;
  readonly description?: LocalizedText;
  readonly kind: ProviderKind;
  readonly capabilities?: readonly string[];
  readonly configurationSchema?: JsonValue;
}

function isProviderDeclaration(value: unknown): value is PluginProviderDeclaration {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const candidate = value as Record<string, unknown>;
  return typeof candidate.id === "string" && candidate.id.length > 0
    && (typeof candidate.label === "string"
      || (candidate.label !== null && typeof candidate.label === "object" && !Array.isArray(candidate.label)))
    && typeof candidate.kind === "string" && candidate.kind.length > 0;
}

/**
 * Narrows a providers.list dispatch response to the declared providers.
 * Failed envelopes and malformed entries yield an empty list so the host's
 * fail-closed registry keeps the plugin unlisted.
 */
export function parsePluginProviderListResult(response: WasmDispatchResponse): readonly PluginProviderDeclaration[] {
  if (!response.ok) return [];
  const { result } = response;
  if (!result || typeof result !== "object" || Array.isArray(result)) return [];
  const providers = (result as { providers?: unknown }).providers;
  if (!Array.isArray(providers)) return [];
  return providers.filter(isProviderDeclaration);
}

/** The provider.invoke request payload the host writes into guest memory. */
export interface PluginProviderInvokeRequest {
  readonly providerId: string;
  readonly kind: ProviderKind;
  readonly operation: string;
  readonly requestId: string;
  readonly session: TerminalSessionSnapshot;
  readonly payload?: JsonValue;
  readonly deadlineMs?: number;
}

/** Decodes and validates one provider.invoke request payload. */
export function parsePluginProviderInvokeRequest(payload: JsonValue | undefined): PluginProviderInvokeRequest {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new PluginError("invalid_argument", "provider invoke payload must be an object");
  }
  const candidate = payload as Record<string, unknown>;
  for (const field of ["providerId", "kind", "operation", "requestId"] as const) {
    if (typeof candidate[field] !== "string" || (candidate[field] as string).length === 0) {
      throw new PluginError("invalid_argument", `provider invoke payload.${field} must be a non-empty string`);
    }
  }
  const session = candidate.session;
  if (!session || typeof session !== "object" || Array.isArray(session)
    || typeof (session as Record<string, unknown>).sessionId !== "string"
    || ((session as Record<string, unknown>).sessionId as string).length === 0) {
    throw new PluginError("invalid_argument", "provider invoke payload.session requires a sessionId");
  }
  return {
    providerId: candidate.providerId as string,
    kind: candidate.kind as ProviderKind,
    operation: candidate.operation as string,
    requestId: candidate.requestId as string,
    session: session as TerminalSessionSnapshot,
    ...(candidate.payload !== undefined ? { payload: candidate.payload as JsonValue } : {}),
    ...(candidate.deadlineMs !== undefined ? { deadlineMs: candidate.deadlineMs as number } : {}),
  };
}
