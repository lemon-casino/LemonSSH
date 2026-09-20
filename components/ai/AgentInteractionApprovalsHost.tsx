/**
 * Always-mounted host for Go agent interaction (capability approval) cards.
 * The Go interaction router blocks a dispatch until the user decides, so the
 * prompt must be approvable even when the Catty AI side panel has never been
 * opened (W13).
 */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { ToolCall } from '../ai-elements/tool-call';
import { useI18n } from '../../application/i18n/I18nProvider';
import { getAgentInteractionBridge } from '../../application/agentInteractionBridge';

/** Wire payload of the Go "agent:interaction" event and pending list. */
export interface AgentInteractionRequest {
  interactionId: string;
  capabilityId: string;
  description?: string;
  summary?: Record<string, unknown>;
  deadlineMs?: number;
}

/**
 * Accepts the unwrapped event payload, the Go pending-list entry and a
 * single-element array envelope; returns null for anything without both ids.
 */
export function normalizeAgentInteraction(raw: unknown): AgentInteractionRequest | null {
  const candidate = Array.isArray(raw) ? raw[0] : raw;
  if (!candidate || typeof candidate !== 'object') return null;
  const record = candidate as Record<string, unknown>;
  const interactionId = typeof record.interactionId === 'string' ? record.interactionId : '';
  const capabilityId = typeof record.capabilityId === 'string' ? record.capabilityId : '';
  if (!interactionId || !capabilityId) return null;
  return {
    interactionId,
    capabilityId,
    description: typeof record.description === 'string' ? record.description : undefined,
    summary: record.summary && typeof record.summary === 'object' && !Array.isArray(record.summary)
      ? record.summary as Record<string, unknown>
      : undefined,
    deadlineMs: typeof record.deadlineMs === 'number' && Number.isFinite(record.deadlineMs) && record.deadlineMs > 0
      ? record.deadlineMs
      : undefined,
  };
}

/** Milliseconds from now until the Go auto-reject deadline; null when unknown. */
export function agentInteractionExpiryDelayMs(deadlineMs: number | undefined, now: number): number | null {
  if (deadlineMs === undefined) return null;
  return Math.max(0, deadlineMs - now);
}

/**
 * Forwards one decision to the Go gate. Returns false when nothing was
 * forwarded: a typed "not pending"/"already resolved" error only means Go
 * settled the interaction first (deadline or cancellation), and a missing
 * bridge means there is no Go gate in this shell. Never surfaces as a crash.
 */
export async function respondAgentInteraction(interactionId: string, approved: boolean): Promise<boolean> {
  try {
    const respond = getAgentInteractionBridge()?.agentRespondInteraction;
    if (!respond) return false;
    await respond(interactionId, approved);
    return true;
  } catch {
    return false;
  }
}

export const AgentInteractionApprovalCards: React.FC<{
  requests: AgentInteractionRequest[];
  onRespond: (interactionId: string, approved: boolean) => void;
}> = ({ requests, onRespond }) => {
  if (requests.length === 0) return null;
  return (
    <div
      className="pointer-events-auto fixed bottom-4 right-4 z-[80] flex w-[min(420px,calc(100vw-2rem))] flex-col gap-2"
      data-testid="agent-interaction-approvals-host"
    >
      <div className="rounded-lg border border-border/60 bg-background/95 p-3 shadow-lg backdrop-blur-sm">
        <CardTitle />
        <div className="space-y-2">
          {requests.map((request) => (
            <ToolCall
              key={request.interactionId}
              name={request.capabilityId}
              args={request.summary}
              isLoading={false}
              isInterrupted={false}
              approvalStatus="pending"
              approvalId={request.interactionId}
              title={request.description || undefined}
              onApproveOnce={() => onRespond(request.interactionId, true)}
              onReject={() => onRespond(request.interactionId, false)}
            />
          ))}
        </div>
      </div>
    </div>
  );
};

const CardTitle: React.FC = () => {
  const { t } = useI18n();
  return (
    <div className="mb-2 text-xs font-medium text-muted-foreground">
      {t('ai.agentApproval.title')}
    </div>
  );
};

export const AgentInteractionApprovalsHost: React.FC = () => {
  const [pending, setPending] = useState<Map<string, AgentInteractionRequest>>(new Map());
  const expiryTimers = useRef(new Map<string, ReturnType<typeof setTimeout>>());

  const removeInteraction = useCallback((interactionId: string) => {
    const timer = expiryTimers.current.get(interactionId);
    if (timer) {
      clearTimeout(timer);
      expiryTimers.current.delete(interactionId);
    }
    setPending((prev) => {
      const next = new Map(prev);
      next.delete(interactionId);
      return next;
    });
  }, []);

  // One-shot timer mirroring the Go deadline: Go auto-rejects there, this
  // only dismisses the card. Replays/late events never re-arm an existing one.
  const armExpiry = useCallback((request: AgentInteractionRequest) => {
    if (expiryTimers.current.has(request.interactionId)) return;
    const delay = agentInteractionExpiryDelayMs(request.deadlineMs, Date.now());
    if (delay === null) return;
    expiryTimers.current.set(
      request.interactionId,
      setTimeout(() => removeInteraction(request.interactionId), delay),
    );
  }, [removeInteraction]);

  useEffect(() => {
    const bridge = getAgentInteractionBridge();
    const timers = expiryTimers.current;
    const unsubscribe = bridge?.onAgentInteraction?.((payload) => {
      const request = normalizeAgentInteraction(payload);
      if (!request) return;
      setPending((prev) => new Map(prev).set(request.interactionId, request));
      armExpiry(request);
    }) ?? (() => undefined);
    const unsubscribeCleared = bridge?.onAgentInteractionCleared?.(({ interactionId }) => removeInteraction(interactionId));
    // Replay approvals that opened before this host mounted (settings window).
    void bridge?.agentPendingInteractions?.().then((entries) => {
      for (const entry of entries ?? []) {
        const request = normalizeAgentInteraction(entry);
        if (!request) continue;
        setPending((prev) => prev.has(request.interactionId)
          ? prev
          : new Map(prev).set(request.interactionId, request));
        armExpiry(request);
      }
    }).catch(() => undefined);
    return () => {
      unsubscribe();
      unsubscribeCleared?.();
      for (const timer of timers.values()) clearTimeout(timer);
      timers.clear();
    };
  }, [armExpiry, removeInteraction]);

  const respond = useCallback((interactionId: string, approved: boolean) => {
    void respondAgentInteraction(interactionId, approved);
    removeInteraction(interactionId);
  }, [removeInteraction]);

  const requests = Array.from(pending.values());
  if (requests.length === 0) return null;
  return <AgentInteractionApprovalCards requests={requests} onRespond={respond} />;
};
