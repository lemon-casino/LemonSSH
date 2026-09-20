import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { I18nProvider } from "../../application/i18n/I18nProvider.tsx";
import { TooltipProvider } from "../ui/tooltip.tsx";
import { getActiveRuntimeClient, setActiveRuntimeClient } from "../../infrastructure/runtime/runtimeClient.ts";
import type { RuntimeClient } from "../../infrastructure/runtime/runtimeClient.ts";
import {
  AgentInteractionApprovalCards,
  AgentInteractionApprovalsHost,
  agentInteractionExpiryDelayMs,
  normalizeAgentInteraction,
  respondAgentInteraction,
} from "./AgentInteractionApprovalsHost.tsx";

const renderWithProviders = (ui: React.ReactElement) =>
  renderToStaticMarkup(
    React.createElement(
      I18nProvider,
      { locale: "en" },
      React.createElement(TooltipProvider, null, ui),
    ),
  );

test("normalizeAgentInteraction accepts event payloads, pending entries and array envelopes", () => {
  assert.deepEqual(
    normalizeAgentInteraction({
      interactionId: "ia_1",
      capabilityId: "netcatty.exec",
      description: "Run a command",
      summary: { method: "netcatty/exec", command: "reboot" },
      deadlineMs: 4102444800000,
    }),
    {
      interactionId: "ia_1",
      capabilityId: "netcatty.exec",
      description: "Run a command",
      summary: { method: "netcatty/exec", command: "reboot" },
      deadlineMs: 4102444800000,
    },
  );
  // Pending-list entries carry no description; array envelopes unwrap.
  assert.deepEqual(
    normalizeAgentInteraction([{ interactionId: "ia_2", capabilityId: "netcatty.sftp.write" }]),
    { interactionId: "ia_2", capabilityId: "netcatty.sftp.write", description: undefined, summary: undefined, deadlineMs: undefined },
  );
  const expired = normalizeAgentInteraction({
    interactionId: "ia_3",
    capabilityId: "netcatty.host.notes.set",
    summary: { method: "netcatty/host.notes.set", hostId: "host_9" },
    deadlineMs: -5,
  });
  assert.deepEqual(expired, {
    interactionId: "ia_3",
    capabilityId: "netcatty.host.notes.set",
    description: undefined,
    summary: { method: "netcatty/host.notes.set", hostId: "host_9" },
    deadlineMs: undefined,
  });
  assert.equal(normalizeAgentInteraction(null), null);
  assert.equal(normalizeAgentInteraction("ia_1"), null);
  assert.equal(normalizeAgentInteraction({ capabilityId: "netcatty.exec" }), null);
  assert.equal(normalizeAgentInteraction({ interactionId: "ia_1" }), null);
  const badSummary = normalizeAgentInteraction({ interactionId: "ia_4", capabilityId: "x", summary: ["bad"] });
  assert.equal(badSummary?.summary, undefined);
});

test("agentInteractionExpiryDelayMs clamps past deadlines and skips unknown ones", () => {
  assert.equal(agentInteractionExpiryDelayMs(1500, 1000), 500);
  assert.equal(agentInteractionExpiryDelayMs(500, 1000), 0);
  assert.equal(agentInteractionExpiryDelayMs(undefined, 1000), null);
});

test("respondAgentInteraction forwards decisions and swallows typed already-resolved errors", async () => {
  const previousClient = getActiveRuntimeClient();
  const calls: Array<[string, boolean]> = [];
  try {
    setActiveRuntimeClient({
      transitionBridge: {
        agentRespondInteraction: async (interactionId: string, approved: boolean) => {
          calls.push([interactionId, approved]);
          if (interactionId === "ia_gone") throw new Error('interaction "ia_gone" was already resolved');
        },
      } as NetcattyBridge,
    } as RuntimeClient);
    assert.equal(await respondAgentInteraction("ia_1", true), true);
    assert.equal(await respondAgentInteraction("ia_gone", false), false);
    assert.deepEqual(calls, [["ia_1", true], ["ia_gone", false]]);
    setActiveRuntimeClient(undefined);
    assert.equal(await respondAgentInteraction("ia_2", false), false);
  } finally {
    setActiveRuntimeClient(previousClient);
  }
});

test("approval cards render the capability, its command and the localized title", () => {
  const markup = renderWithProviders(
    React.createElement(AgentInteractionApprovalCards, {
      requests: [
        {
          interactionId: "ia-1",
          capabilityId: "netcatty.exec",
          description: "Run a shell command on PROD",
          summary: { method: "netcatty/exec", command: "shutdown -h now", sessionId: "sess-1" },
          deadlineMs: 4102444800000,
        },
        {
          interactionId: "ia-2",
          capabilityId: "netcatty.host.notes.set",
          summary: { method: "netcatty/host.notes.set", hostId: "host_9" },
        },
      ],
      onRespond: () => {},
    }),
  );

  assert.match(markup, /data-testid="agent-interaction-approvals-host"/);
  assert.match(markup, /Agent approvals/);
  assert.match(markup, /shutdown -h now/);
  assert.match(markup, /netcatty\.host\.notes\.set/);
  assert.match(markup, /title="Run a shell command on PROD"/);
  assert.match(markup, /border-yellow-500\/30/);
});

test("approval cards render nothing without pending requests", () => {
  assert.equal(
    renderWithProviders(React.createElement(AgentInteractionApprovalCards, { requests: [], onRespond: () => {} })),
    "",
  );
});

test("the always-mounted host renders nothing before events arrive", () => {
  assert.equal(renderWithProviders(React.createElement(AgentInteractionApprovalsHost)), "");
});

test("the host subscribes, replays, expires and responds through the bridge", () => {
  const source = readFileSync(new URL("./AgentInteractionApprovalsHost.tsx", import.meta.url), "utf8");

  // Mount-time subscription plus pending replay for the settings window.
  assert.match(source, /bridge\?\.onAgentInteraction\?\.\(/);
  assert.match(source, /agentPendingInteractions\?\.\(\)/);
  // One-shot expiry timer per interaction, never re-armed by replays.
  assert.match(source, /expiryTimers\.current\.has\(request\.interactionId\)/);
  assert.match(source, /setTimeout\(\(\) => removeInteraction\(request\.interactionId\), delay\)/);
  assert.match(source, /clearTimeout\(timer\)/);
  // Decisions ride the shared respond helper; the title key must be wired.
  assert.match(source, /respondAgentInteraction\(interactionId, approved\)/);
  assert.match(source, /ai\.agentApproval\.title/);
});
