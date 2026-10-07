/**
 * Shared fixed-position stacking container for the always-mounted approval
 * hosts (Go agent interactions and External MCP). Both hosts used to paint a
 * byte-identical `fixed bottom-4 right-4 z-[80]` wrapper, so two pending
 * groups overlapped and the lower card's buttons were unreachable. Each group
 * now portals into one bottom-anchored column, so concurrent groups stack and
 * grow upward instead of hiding each other.
 */

import React, { type ReactNode } from 'react';
import { createPortal } from 'react-dom';

const STACK_ANCHOR_ID = 'lemonssh-approvals-overlay-stack';

/** The single bottom-anchored column every approval group portals into. */
const STACK_ANCHOR_CLASS =
  'pointer-events-none fixed bottom-4 right-4 z-[80] flex w-[min(420px,calc(100vw-2rem))] flex-col-reverse gap-2';

const STACK_GROUP_CLASS = 'pointer-events-auto flex w-full flex-col gap-2';

/** Fallback for non-DOM renders (react-dom/server tests): legacy positioning. */
const STACK_GROUP_FALLBACK_CLASS =
  'pointer-events-auto fixed bottom-4 right-4 z-[80] flex w-[min(420px,calc(100vw-2rem))] flex-col gap-2';

/** One bottom-anchored column shared by every mounted approval host. */
function getStackAnchor(): HTMLElement {
  const existing = document.getElementById(STACK_ANCHOR_ID);
  if (existing) return existing;
  const anchor = document.createElement('div');
  anchor.id = STACK_ANCHOR_ID;
  anchor.className = STACK_ANCHOR_CLASS;
  document.body.appendChild(anchor);
  return anchor;
}

/**
 * Renders one approval group inside the shared stack. `stackId` keeps the
 * legacy per-host data-testid so existing selectors keep working.
 */
export const ApprovalOverlayStack: React.FC<{ stackId: string; children: ReactNode }> = ({
  stackId,
  children,
}) => {
  if (typeof document === 'undefined') {
    return (
      <div className={STACK_GROUP_FALLBACK_CLASS} data-testid={stackId}>
        {children}
      </div>
    );
  }
  return createPortal(
    <div className={STACK_GROUP_CLASS} data-testid={stackId}>
      {children}
    </div>,
    getStackAnchor(),
  );
};
