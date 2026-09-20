import test from "node:test";
import assert from "node:assert/strict";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { CopilotCliCard } from "./CopilotCliCard";

function buttonFor(markup: string, label: string): string {
  const buttons = markup.match(/<button\b[\s\S]*?<\/button>/g) ?? [];
  return buttons.find((button) => button.includes(label)) ?? "";
}

const noop = () => {};

test("Cursor check button stays enabled without a custom path", () => {
  const markup = renderToStaticMarkup(
    <CopilotCliCard
      pathInfo={{ path: null, version: null, available: false }}
      isResolvingPath={false}
      customPath=""
      onCustomPathChange={noop}
      onSelectDirectory={noop}
      onRecheckPath={noop}
      i18nPrefix="ai.cursor"
      allowEmptyCheck
    />,
  );

  assert.equal(buttonFor(markup, "ai.cursor.check").includes("disabled=\"\""), false);
  assert.ok(markup.indexOf("ai.agent.directory") < markup.indexOf("ai.cursor.check"));
});

test("Copilot check button still requires a custom path", () => {
  const markup = renderToStaticMarkup(
    <CopilotCliCard
      pathInfo={{ path: null, version: null, available: false }}
      isResolvingPath={false}
      customPath=""
      onCustomPathChange={noop}
      onSelectDirectory={noop}
      onRecheckPath={noop}
    />,
  );

  assert.equal(buttonFor(markup, "ai.copilot.check").includes("disabled=\"\""), true);
});

test("Grok card surfaces ACP runtime toggle when detected", () => {
  const markup = renderToStaticMarkup(
    <CopilotCliCard
      pathInfo={{ path: "/usr/bin/grok", version: "0.2.118", available: true }}
      isResolvingPath={false}
      customPath=""
      onCustomPathChange={noop}
      onSelectDirectory={noop}
      onRecheckPath={noop}
      i18nPrefix="ai.grok"
      grokRuntime="acp"
      onGrokRuntimeChange={noop}
    />,
  );

  assert.match(markup, /ai\.grok\.runtime\.acp\.title|Use Grok ACP/);
  assert.match(markup, /role="switch"/);
});

test("Grok card hides ACP toggle without runtime change handler", () => {
  const markup = renderToStaticMarkup(
    <CopilotCliCard
      pathInfo={{ path: "/usr/bin/grok", version: "0.2.118", available: true }}
      isResolvingPath={false}
      customPath=""
      onCustomPathChange={noop}
      onSelectDirectory={noop}
      onRecheckPath={noop}
      i18nPrefix="ai.grok"
      grokRuntime="acp"
    />,
  );

  assert.doesNotMatch(markup, /role="switch"/);
  assert.doesNotMatch(markup, /ai\.grok\.runtime\.acp\.title|Use Grok ACP \(agent stdio\)/);
});
