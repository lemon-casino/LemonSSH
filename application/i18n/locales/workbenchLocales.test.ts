import assert from "node:assert/strict";
import test from "node:test";

import en from "./en.ts";
import zhCN from "./zh-CN.ts";
import zhTW from "./zh-TW.ts";
import ru from "./ru.ts";
import es from "./es.ts";

const WORKBENCH_TREE_KEYS = [
  "workbench.tree.newSession",
  "workbench.tree.empty",
  "workbench.tree.section.ungrouped",
  "workbench.tree.section.local",
  "workbench.tree.section.workspaces",
  "workbench.tree.section.logs",
  "workbench.tree.section.editors",
  "workbench.tree.section.others",
] as const;

const LOCALES = [
  { name: "en", messages: en },
  { name: "zh-CN", messages: zhCN },
  { name: "zh-TW", messages: zhTW },
  { name: "ru", messages: ru },
  { name: "es", messages: es },
];

test("all locales carry the workbench session tree labels", () => {
  for (const locale of LOCALES) {
    const missing = WORKBENCH_TREE_KEYS.filter((key) => !locale.messages[key]);
    assert.deepEqual(
      missing,
      [],
      `${locale.name} is missing workbench tree labels`,
    );
  }
});
