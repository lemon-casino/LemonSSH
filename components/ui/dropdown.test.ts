import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(new URL("./dropdown.tsx", import.meta.url), "utf8");

test("dropdown portals cancel the frameless title-drag region", () => {
  assert.match(
    source,
    /fixed z-\[999999\] rounded-md border border-border\/60 bg-popover p-1 text-popover-foreground shadow-md app-no-drag pointer-events-auto/,
  );
});
