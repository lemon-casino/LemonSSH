/**
 * aiGoTurn — renderer-side driver of the Go turn runtime (W12 minimal
 * chain). Enabled only when the host reports the dev fixture driver
 * (AgentStatus.goRuntimeReady); the product Catty path stays on the
 * renderer AgentRuntime until live provider wiring lands (W13+). All
 * authoritative state lives in Go: this module only relays DTOs, polls
 * for reconciliation and routes Stop to the Go owner.
 */

import { generateId } from '../../infrastructure/ai/aiChatStreamingSupport';

/** Structural subset of AgentRuntimePort the runner needs. */
export interface GoTurnClient {
  agentPrepare(request: {
    requestId: string;
    chatSessionId: string;
    agentId: string;
    input: { text: string };
    requestedScope: { terminalRead: boolean };
  }): Promise<{ turnId: string; cursor: string }>;
  agentStart(command: { requestId: string; kind: string; turnId: string }): Promise<void>;
  agentStop(turnId: string, reason: string): Promise<unknown>;
  agentReadEvents(
    turnId: string,
    afterSequence: string,
    limit: number,
  ): Promise<{
    events?: ReadonlyArray<{ sequence: string; type: string; payload?: unknown }>;
    nextCursor?: string;
    hasMore?: boolean;
    cursorExpired?: boolean;
    snapshot?: { status: string; throughSequence?: string };
  }>;
  agentSnapshot(turnId: string): Promise<{ status: string }>;
}

export interface GoTurnCallbacks {
  onTextDelta: (text: string) => void;
  onFinished: (status: string) => void;
}

const TERMINAL_STATUSES = new Set(['completed', 'stopped', 'interrupted']);

export function isTerminalStatus(status: string | undefined): boolean {
  return !!status && TERMINAL_STATUSES.has(status);
}

function decodePayload(payload: unknown): string {
  if (payload instanceof Uint8Array) {
    return new TextDecoder().decode(payload);
  }
  if (Array.isArray(payload)) {
    return new TextDecoder().decode(new Uint8Array(payload));
  }
  if (typeof payload === 'string') {
    return payload;
  }
  return '';
}

/** Aborts are routed to the Go owner — the renderer never just walks away. */
async function stopTurn(client: GoTurnClient, turnId: string): Promise<void> {
  try {
    await client.agentStop(turnId, 'user requested');
  } catch (err) {
    console.error('[aiGoTurn] stop failed:', err);
  }
}

const delay = (ms: number, signal: AbortSignal): Promise<void> =>
  new Promise(resolve => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      'abort',
      () => {
        clearTimeout(timer);
        resolve();
      },
      { once: true },
    );
  });

export async function runGoTurn(
  client: GoTurnClient,
  options: { chatSessionId: string; userText: string; signal: AbortSignal },
  callbacks: GoTurnCallbacks,
): Promise<{ status: string }> {
  const requestId = `req_${generateId()}`;
  const prepared = await client.agentPrepare({
    requestId,
    chatSessionId: options.chatSessionId,
    agentId: 'catty',
    input: { text: options.userText },
    requestedScope: { terminalRead: true },
  });
  const turnId = prepared.turnId;

  const onAbort = (): void => {
    void stopTurn(client, turnId);
  };
  if (options.signal.aborted) {
    await stopTurn(client, turnId);
  } else {
    options.signal.addEventListener('abort', onAbort, { once: true });
  }

  try {
    await client.agentStart({ requestId, kind: 'start', turnId });
  } catch (err) {
    options.signal.removeEventListener('abort', onAbort);
    await stopTurn(client, turnId);
    throw err;
  }

  let cursor = '0';
  let terminal = '';
  const applyEvents = (page: Awaited<ReturnType<GoTurnClient['agentReadEvents']>>): void => {
    for (const event of page.events ?? []) {
      cursor = event.sequence;
      if (event.type === 'text_delta') {
        let text = '';
        try {
          const parsed = JSON.parse(decodePayload(event.payload)) as { text?: string };
          text = parsed.text ?? '';
        } catch {
          text = '';
        }
        if (text) callbacks.onTextDelta(text);
      }
      if (event.type === 'turn_end') {
        let status = '';
        try {
          const parsed = JSON.parse(decodePayload(event.payload)) as { status?: string };
          status = parsed.status ?? '';
        } catch {
          status = '';
        }
        if (status) terminal = status;
      }
    }
  };

  try {
    while (!terminal) {
      if (options.signal.aborted && !TERMINAL_STATUSES.has(terminal)) {
        // The stop above converges the turn; keep polling until the
        // terminal record arrives (bounded by the Go-side finalize).
      }
      const page = await client.agentReadEvents(turnId, cursor, 128);
      applyEvents(page);
      if (page.cursorExpired) {
        // Snapshot carries the authoritative through-sequence; events in
        // the evicted region are unrecoverable by design (T07).
        cursor = page.nextCursor ?? cursor;
        continue;
      }
      if (terminal) break;
      const snapshotStatus = page.snapshot?.status;
      if (isTerminalStatus(snapshotStatus)) {
        terminal = snapshotStatus;
        break;
      }
      if (page.hasMore) continue;
      const snapshot = await client.agentSnapshot(turnId);
      if (isTerminalStatus(snapshot.status)) {
        terminal = snapshot.status;
        break;
      }
      await delay(50, options.signal);
    }
  } finally {
    options.signal.removeEventListener('abort', onAbort);
  }

  callbacks.onFinished(terminal);
  return { status: terminal };
}
