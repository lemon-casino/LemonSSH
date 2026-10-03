export const PLUGIN_API_VERSION = "0.1.0-internal" as const;
export const PLUGIN_MANIFEST_FILE = "lemonssh.plugin.json" as const;
// Legacy manifest file names accepted for backward compatibility (mirrors the
// Go host's three-way acceptance in cmd/lemonssh/pluginService.go).
export const PLUGIN_LEGACY_MANIFEST_FILES = [
  "netcatty.plugin.json",
  "manifest.json",
] as const;
export const PLUGIN_PACKAGE_EXTENSION = ".ncpkg" as const;

export type * from "./generated/plugin-contract.js";
export {
  PLUGIN_JSON_MAX_DEPTH,
  PLUGIN_JSON_MAX_NODES,
  assertJsonValue,
  serializeJsonValue,
} from "./jsonValue.js";
export {
  PLUGIN_IMPORTER_MAX_INPUT_BYTES,
  PLUGIN_IMPORTER_MAX_OUTPUT_BYTES,
  PLUGIN_IMPORTER_MAX_RECORD_BYTES,
  PLUGIN_IMPORTER_MAX_RECORDS,
  PLUGIN_RPC_ERROR_CODES,
  PLUGIN_RPC_MAX_JSON_BYTES,
  PLUGIN_SYNC_INLINE_OBJECT_BYTES,
  PLUGIN_SYNC_MAX_OBJECT_BYTES,
  PLUGIN_SYNC_MAX_OBJECT_KEY_LENGTH,
  PLUGIN_SYNC_MAX_REVISION_LENGTH,
  PLUGIN_TERMINAL_INTERCEPTOR_MAX_CHUNK_BYTES,
  PLUGIN_TERMINAL_INTERCEPTOR_MAX_WINDOW_BYTES,
  PLUGIN_WASM_ABI_VERSION,
  PLUGIN_WASM_DEFAULT_TIMEOUT_MS,
  PLUGIN_WASM_EXPORT_ALLOC,
  PLUGIN_WASM_EXPORT_DISPATCH,
  PLUGIN_WASM_EXPORT_FREE,
  PLUGIN_WASM_HOST_IMPORT_STATUS,
  PLUGIN_WASM_HOST_MODULE,
  PLUGIN_WASM_IMPORT_HOST_LOG,
  PLUGIN_WASM_IMPORT_HOST_SETTING_GET,
  PLUGIN_WASM_MAX_LOG_BYTES,
  PLUGIN_WASM_MAX_LOG_ENTRIES,
  PLUGIN_WASM_MAX_REQUEST_BYTES,
  PLUGIN_WASM_MAX_RESPONSE_BYTES,
  PLUGIN_WASM_MAX_SETTING_KEY_BYTES,
  PLUGIN_WIRE_MAX_SAFE_INTEGER,
} from "./generated/plugin-contract-limits.js";
export {
  COMPANION_STDIO_MAX_CONTENT_BYTES,
  COMPANION_STDIO_MAX_HEADER_BYTES,
  ContentLengthFrameDecoder,
  encodeContentLengthFrame,
  type ContentLengthFrameDecoderOptions,
} from "./stdioFraming.js";
export {
  PLUGIN_STREAM_MAX_CHUNK_BYTES,
  PLUGIN_STREAM_MAX_CREDIT_BYTES,
  PLUGIN_STREAM_MAX_FRAME_JSON_BYTES,
  PLUGIN_STREAM_MAX_ID_LENGTH,
  PLUGIN_STREAM_MAX_WINDOW_BYTES,
  PLUGIN_STREAM_MIN_WINDOW_BYTES,
  assertStreamChunkData,
  assertStreamFrame,
  createBase64StreamChunk,
  createJsonStreamChunk,
  createMessagePortStreamEnvelope,
  materializeStreamChunk,
  type MaterializedStreamChunk,
  type MessagePortStreamEnvelope,
} from "./streamTransport.js";
