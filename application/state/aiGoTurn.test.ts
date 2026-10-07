import assert from "node:assert/strict";
import { test } from "node:test";
import { runGoTurn, type GoTurnClient } from "./aiGoTurn";

interface ScriptedTurn {
  events: Array<{ sequence: string; type: string; payload?: Uint8Array }>;
  stopAfter?: number;
  snapshotStatus: string;
}

function scriptedClient(turn: ScriptedTurn): { client: GoTurnClient; calls: string[]; stopped: boolean } {
  const calls: string[] = [];
  let stopRequested = false;
  const client: GoTurnClient = {
    agentPrepare: async () => {
      calls.push("prepare");
      return { turnId: "turn_1", cursor: "0" };
    },
    agentStart: async () => {
      calls.push("start");
    },
    agentStop: async () => {
      calls.push("stop");
      stopRequested = true;
    },
    agentReadEvents: async (_turnId, afterSequence) => {
      calls.push(`read:${afterSequence}`);
      if (stopRequested && turn.stopAfter !== undefined) {
        return {
          events: turn.events.slice(turn.stopAfter),
          nextCursor: turn.events[turn.events.length - 1]?.sequence ?? "0",
          hasMore: false,
        };
      }
      return { events: turn.events, nextCursor: turn.events[turn.events.length - 1]?.sequence ?? "0", hasMore: false };
    },
    agentSnapshot: async () => {
      calls.push("snapshot");
      return { status: stopRequested ? "stopped" : turn.snapshotStatus };
    },
  };
  return { client, calls, get stopped() { return stopRequested; } };
}

const encoder = new TextEncoder();
const textEvent = (sequence: string, text: string) => ({
  sequence,
  type: "text_delta",
  payload: encoder.encode(JSON.stringify({ text })),
});

test("runGoTurn streams deltas and reports terminal status", async () => {
  const { client, calls } = scriptedClient({
    events: [textEvent("1", "你好"), textEvent("2", " 🌍"), { sequence: "3", type: "turn_end", payload: encoder.encode('{"status":"completed"}') }],
    snapshotStatus: "completed",
  });
  const deltas: string[] = [];
  let finished = "";
  const result = await runGoTurn(client, { chatSessionId: "chat_1", userText: "hi", signal: new AbortController().signal }, {
    onTextDelta: (text) => deltas.push(text),
    onFinished: (status) => { finished = status; },
  });
  assert.equal(result.status, "completed");
  assert.equal(finished, "completed");
  assert.deepEqual(deltas, ["你好", " 🌍"]);
  assert.equal(calls[0], "prepare");
  assert.equal(calls[1], "start");
});

test("runGoTurn routes abort through the Go owner stop", async () => {
  const controller = new AbortController();
  let reads = 0;
  const client: GoTurnClient = {
    agentPrepare: async () => ({ turnId: "turn_1", cursor: "0" }),
    agentStart: async () => {},
    agentStop: async () => {},
    agentReadEvents: async () => {
      reads += 1;
      if (reads >= 2) controller.abort();
      return { events: [], hasMore: false };
    },
    agentSnapshot: async () => ({ status: "stopped" }),
  };
  const result = await runGoTurn(client, { chatSessionId: "chat_1", userText: "hi", signal: controller.signal }, {
    onTextDelta: () => {},
    onFinished: () => {},
  });
  assert.equal(result.status, "stopped");
});

test("runGoTurn reconciles from snapshot when the terminal notification is lost", async () => {
  const client: GoTurnClient = {
    agentPrepare: async () => ({ turnId: "turn_1", cursor: "0" }),
    agentStart: async () => {},
    agentStop: async () => {},
    agentReadEvents: async (_turnId, afterSequence) => {
      if (afterSequence === "0") {
        return {
          events: [textEvent("1", "partial")],
          nextCursor: "1",
          hasMore: false,
          snapshot: { status: "completed", throughSequence: "2" },
        };
      }
      return { events: [], hasMore: false };
    },
    agentSnapshot: async () => ({ status: "completed" }),
  };
  const deltas: string[] = [];
  const result = await runGoTurn(client, { chatSessionId: "chat_1", userText: "hi", signal: new AbortController().signal }, {
    onTextDelta: (text) => deltas.push(text),
    onFinished: () => {},
  });
  assert.deepEqual(deltas, ["partial"]);
  assert.equal(result.status, "completed");
});
