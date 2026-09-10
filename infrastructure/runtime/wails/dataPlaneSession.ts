// Loopback data-plane session client. Opens the authenticated WebSocket,
// grants the initial receive window, and delivers Output payloads as UTF-8
// strings. Completely independent of Wails so Node tests can drive it with a
// fake socket.

import {
  FRAME_COMPLETE,
  FRAME_CREDIT,
  FRAME_OUTPUT,
  RECEIVE_WINDOW_BYTES,
  marshalFrame,
  unmarshalFrame,
} from "../../terminal/dataplane/frame";
import {
  buildTerminalSocketUrl,
  terminalSocketSubprotocols,
} from "./terminalRoute";
import type { WailsRouteBootstrap } from "./terminalRoute";

export interface DataPlaneSocket {
  binaryType: string;
  onopen: ((event: unknown) => void) | null;
  onmessage: ((event: { data: ArrayBuffer | Uint8Array | Blob | string }) => void) | null;
  onerror: ((event: unknown) => void) | null;
  onclose: ((event: unknown) => void) | null;
  send(data: ArrayBuffer | Uint8Array): void;
  close(): void;
}

export interface DataPlaneSessionHandle {
  dispose: () => void;
}

export interface OpenDataPlaneSessionOptions {
  listenAddr: string;
  bootstrap: WailsRouteBootstrap;
  onData: (chunk: string) => void;
  onComplete?: () => void;
  decoder?: { decode(input: Uint8Array): string };
  openSocket?: (url: string, protocols: string[]) => DataPlaneSocket;
}

function defaultOpenSocket(url: string, protocols: string[]): DataPlaneSocket {
  return new WebSocket(url, protocols) as unknown as DataPlaneSocket;
}

function toBytes(data: ArrayBuffer | Uint8Array | Blob | string): Promise<Uint8Array> {
  if (data instanceof Uint8Array) return Promise.resolve(data);
  if (data instanceof ArrayBuffer) return Promise.resolve(new Uint8Array(data));
  if (typeof data === "string") return Promise.resolve(new TextEncoder().encode(data));
  return data.arrayBuffer().then((buffer) => new Uint8Array(buffer));
}

function sendCredit(socket: DataPlaneSocket, generation: number, sequence: number, credit: number): void {
  socket.send(marshalFrame({
    kind: FRAME_CREDIT,
    generation,
    sequence,
    creditCost: credit,
    correlation: 0,
    timestampMicros: 0,
    payload: new Uint8Array(),
  }));
}

export function openDataPlaneSession(options: OpenDataPlaneSessionOptions): DataPlaneSessionHandle {
  const decoder = options.decoder ?? new TextDecoder();
  const url = buildTerminalSocketUrl(
    options.listenAddr,
    options.bootstrap.SessionID,
    options.bootstrap.Generation,
    "data",
  );
  const socket = (options.openSocket ?? defaultOpenSocket)(
    url,
    terminalSocketSubprotocols(options.bootstrap.DataToken),
  );
  socket.binaryType = "arraybuffer";
  let disposed = false;

  socket.onopen = () => {
    if (disposed) return;
    sendCredit(socket, options.bootstrap.Generation, 0, RECEIVE_WINDOW_BYTES);
  };
  socket.onmessage = (event) => {
    if (disposed) return;
    void toBytes(event.data).then((bytes) => {
      if (disposed) return;
      const frame = unmarshalFrame(bytes);
      if (frame.kind === FRAME_OUTPUT) {
        options.onData(decoder.decode(frame.payload));
        sendCredit(socket, frame.generation, frame.sequence, frame.creditCost || frame.payload.length);
        return;
      }
      if (frame.kind === FRAME_COMPLETE) {
        options.onComplete?.();
        dispose();
      }
    }).catch(() => {
      dispose();
    });
  };
  socket.onerror = () => dispose();
  socket.onclose = () => {
    if (!disposed) options.onComplete?.();
  };

  function dispose(): void {
    if (disposed) return;
    disposed = true;
    try {
      socket.close();
    } catch {
      // already closed
    }
  }

  return { dispose };
}
