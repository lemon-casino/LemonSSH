import assert from "node:assert/strict";
import { test } from "node:test";

import {
  FRAME_COMPLETE,
  FRAME_CREDIT,
  FRAME_OUTPUT,
  RECEIVE_WINDOW_BYTES,
  marshalFrame,
  unmarshalFrame,
} from "../../terminal/dataplane/frame";
import {
  createDataPlaneDecoder,
  openDataPlaneSession,
  StreamingTextDecoder,
} from "./dataPlaneSession";
import type { DataPlaneSocket } from "./dataPlaneSession";

function fakeSocket(): DataPlaneSocket & { sent: Uint8Array[]; closed: boolean } {
  const socket: DataPlaneSocket & { sent: Uint8Array[]; closed: boolean } = {
    binaryType: "",
    onopen: null,
    onmessage: null,
    onerror: null,
    onclose: null,
    sent: [],
    closed: false,
    send(data) {
      this.sent.push(data instanceof Uint8Array ? data : new Uint8Array(data));
    },
    close() {
      this.closed = true;
    },
  };
  return socket;
}

test("openDataPlaneSession grants the window then delivers output", async () => {
  const socket = fakeSocket();
  const chunks: string[] = [];
  let completed = false;
  const handle = openDataPlaneSession({
    listenAddr: "127.0.0.1:9",
    bootstrap: { SessionID: "s1", Generation: 1, DataToken: "tok", UrgentToken: "urg", WindowBytes: RECEIVE_WINDOW_BYTES },
    onData: (chunk) => chunks.push(chunk),
    onComplete: () => {
      completed = true;
    },
    openSocket: () => socket,
  });

  socket.onopen?.(undefined);
  assert.equal(socket.sent.length, 1);
  const grant = unmarshalFrame(socket.sent[0]);
  assert.equal(grant.kind, FRAME_CREDIT);
  assert.equal(grant.sequence, 0);
  assert.equal(grant.creditCost, RECEIVE_WINDOW_BYTES);

  socket.onmessage?.({
    data: marshalFrame({
      kind: FRAME_OUTPUT,
      generation: 1,
      sequence: 1,
      creditCost: 5,
      correlation: 0,
      timestampMicros: 0,
      payload: new TextEncoder().encode("hello"),
    }),
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(chunks, ["hello"]);
  const ack = unmarshalFrame(socket.sent[1]);
  assert.equal(ack.kind, FRAME_CREDIT);
  assert.equal(ack.sequence, 1);
  assert.equal(ack.creditCost, 5);

  socket.onmessage?.({
    data: marshalFrame({
      kind: FRAME_COMPLETE,
      generation: 1,
      sequence: 0,
      creditCost: 0,
      correlation: 0,
      timestampMicros: 0,
      payload: new Uint8Array(),
    }),
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(completed, true);
  assert.equal(socket.closed, true);
  handle.dispose();
});

test('socket loss signals disconnect once instead of terminal completion', () => {
  const socket = fakeSocket();
  let disconnected = 0;
  let completed = 0;
  openDataPlaneSession({
    listenAddr: '127.0.0.1:9',
    bootstrap: { SessionID: 's', Generation: 1, DataToken: 'd', UrgentToken: 'u', WindowBytes: RECEIVE_WINDOW_BYTES },
    onData: () => undefined, onComplete: () => { completed++; }, onDisconnect: () => { disconnected++; },
    openSocket: () => socket,
  });
  socket.onerror?.(undefined);
  socket.onclose?.(undefined);
  assert.equal(disconnected, 1);
  assert.equal(completed, 0);
});

test("dispose closes the socket and ignores later frames", async () => {
  const socket = fakeSocket();
  const chunks: string[] = [];
  const handle = openDataPlaneSession({
    listenAddr: "127.0.0.1:9",
    bootstrap: { SessionID: "s1", Generation: 1, DataToken: "tok", UrgentToken: "urg", WindowBytes: RECEIVE_WINDOW_BYTES },
    onData: (chunk) => chunks.push(chunk),
    openSocket: () => socket,
  });
  handle.dispose();
  assert.equal(socket.closed, true);
  socket.onmessage?.({
    data: marshalFrame({
      kind: FRAME_OUTPUT,
      generation: 1,
      sequence: 1,
      creditCost: 1,
      correlation: 0,
      timestampMicros: 0,
      payload: new TextEncoder().encode("x"),
    }),
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(chunks, []);
});

test("createDataPlaneDecoder decodes GB18030 output and falls back to UTF-8", () => {
  const gbkShuJu = new Uint8Array([0xca, 0xfd, 0xbe, 0xdd]); // "数据" in GB18030
  assert.equal(createDataPlaneDecoder("gb18030").decode(gbkShuJu), "数据");
  assert.equal(createDataPlaneDecoder("GB18030").decode(gbkShuJu), "数据");
  assert.equal(createDataPlaneDecoder().decode(new TextEncoder().encode("hello")), "hello");
  assert.equal(createDataPlaneDecoder("utf-8").decode(new TextEncoder().encode("中文")), "中文");
  // Unknown labels never break the data plane.
  assert.equal(createDataPlaneDecoder("klingon").decode(new TextEncoder().encode("x")), "x");
});

test("StreamingTextDecoder keeps state across frame boundaries", () => {
  // "数据" splits across two frames mid-sequence: 0xca | 0xfd 0xbe 0xdd.
  const decoder = new StreamingTextDecoder("gb18030");
  const first = decoder.decode(new Uint8Array([0xca]));
  const second = decoder.decode(new Uint8Array([0xfd, 0xbe, 0xdd]));
  assert.equal(first + second, "数据");
  // Each fresh instance resets the stream state.
  const reset = new StreamingTextDecoder("gb18030");
  assert.equal(reset.decode(new Uint8Array([0xca, 0xfd, 0xbe, 0xdd])), "数据");
});

test("decoder option decodes output frames with the session charset", async () => {
  const socket = fakeSocket();
  const chunks: string[] = [];
  const gbkShuJu = new Uint8Array([0xca, 0xfd, 0xbe, 0xdd]);
  const handle = openDataPlaneSession({
    listenAddr: "127.0.0.1:9",
    bootstrap: { SessionID: "s1", Generation: 1, DataToken: "tok", UrgentToken: "urg", WindowBytes: RECEIVE_WINDOW_BYTES },
    onData: (chunk) => chunks.push(chunk),
    decoder: createDataPlaneDecoder("gb18030"),
    openSocket: () => socket,
  });
  socket.onmessage?.({
    data: marshalFrame({
      kind: FRAME_OUTPUT,
      generation: 1,
      sequence: 1,
      creditCost: gbkShuJu.length,
      correlation: 0,
      timestampMicros: 0,
      payload: gbkShuJu,
    }),
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(chunks, ["数据"]);
  handle.dispose();
});
