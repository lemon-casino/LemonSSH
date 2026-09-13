import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { HostTagChips, toggleSelectedTag, visibleHostTags } from "./HostTagChips.tsx";

test("visibleHostTags keeps the first two tags and reports overflow", () => {
  assert.deepEqual(visibleHostTags(["edge", "prod", "web"]), {
    shown: ["edge", "prod"],
    extra: 1,
  });
  assert.deepEqual(visibleHostTags(["  ", "edge", ""]), {
    shown: ["edge"],
    extra: 0,
  });
  assert.deepEqual(visibleHostTags(undefined), { shown: [], extra: 0 });
});

test("toggleSelectedTag adds and removes a tag without mutating the source", () => {
  const selected = ["edge"];
  assert.deepEqual(toggleSelectedTag(selected, "prod"), ["edge", "prod"]);
  assert.deepEqual(toggleSelectedTag(selected, "edge"), []);
  assert.deepEqual(selected, ["edge"]);
});

test("HostTagChips paints tags without requiring hover", () => {
  const markup = renderToStaticMarkup(
    React.createElement(HostTagChips, { tags: ["edge", "prod", "web"] }),
  );
  assert.match(markup, /data-host-tag="edge"/);
  assert.match(markup, /data-host-tag="prod"/);
  assert.match(markup, /data-host-tag-extra=""/);
  assert.match(markup, /\+1/);
  assert.doesNotMatch(markup, /<button/);
});

test("HostTagChips becomes a toggle control when a handler is provided", () => {
  const markup = renderToStaticMarkup(
    React.createElement(HostTagChips, {
      tags: ["edge"],
      selectedTags: ["edge"],
      onToggleTag: () => undefined,
    }),
  );
  assert.match(markup, /<button[^>]*data-host-tag="edge"/);
  assert.match(markup, /ring-1 ring-primary\/30/);
});
