import assert from "node:assert/strict";
import test from "node:test";
import { COMMON_FIG_SPECS, normalizeCommandName } from "./autocomplete/figSpecLoader";

test("preload common specs includes dnf alongside yum and apt", () => {
  assert.ok(COMMON_FIG_SPECS.includes("apt"));
  assert.ok(COMMON_FIG_SPECS.includes("yum"));
  assert.ok(COMMON_FIG_SPECS.includes("dnf"));
});

test("normalizeCommandName strips path and extension", () => {
  assert.equal(normalizeCommandName("/usr/bin/dnf"), "dnf");
  assert.equal(normalizeCommandName("DNF"), "dnf");
});

/**
 * Locks the fig spec loader contract for first-load behavior:
 * - the available-spec listing is fetched once and cached,
 * - spec loads deduplicate concurrent requests and cache resolved specs,
 * - preloadCommonSpecs never loads on the synchronous path (nonblocking),
 *   and its deferred batches eventually populate the shared cache.
 * getBridge reads the bridge at call time, so window.netcatty can be mocked
 * after module import.
 */

type MockSpec = { name: string };

let listFigSpecsCalls = 0;
const loadFigSpecRequests: string[] = [];

Object.defineProperty(globalThis, "window", {
  value: {
    netcatty: {
      listFigSpecs: async () => {
        listFigSpecsCalls++;
        return ["story", "slow-spec"];
      },
      loadFigSpec: async (commandName: string): Promise<MockSpec | null> => {
        loadFigSpecRequests.push(commandName);
        if (commandName === "slow-spec") {
          await new Promise((resolve) => setTimeout(resolve, 60));
        }
        return commandName === "story" || commandName === "slow-spec"
          ? { name: commandName }
          : null;
      },
    },
  },
  configurable: true,
});

const { getAvailableSpecs, hasSpec, loadSpec, preloadCommonSpecs } = await import(
  "./autocomplete/figSpecLoader.ts"
);

test("available-spec listing and spec loads are cached, concurrent loads deduplicate", async () => {
  assert.deepEqual(await getAvailableSpecs(), ["story", "slow-spec"]);
  assert.deepEqual(await getAvailableSpecs(), ["story", "slow-spec"]);
  assert.equal(listFigSpecsCalls, 1, "available-specs listing must be cached after first load");

  const [first, second] = await Promise.all([loadSpec("slow-spec"), loadSpec("slow-spec")]);
  assert.deepEqual(first, { name: "slow-spec" });
  assert.deepEqual(second, { name: "slow-spec" });
  assert.equal(
    loadFigSpecRequests.filter((name) => name === "slow-spec").length,
    1,
    "concurrent loads of the same spec must share one bridge request",
  );

  const cached = await loadSpec("slow-spec");
  assert.deepEqual(cached, { name: "slow-spec" });
  assert.equal(
    loadFigSpecRequests.filter((name) => name === "slow-spec").length,
    1,
    "resolved specs must be served from cache on later queries",
  );

  assert.equal(await hasSpec("story"), true);
  assert.equal(await hasSpec("definitely-missing-spec"), false);
  assert.equal(listFigSpecsCalls, 1, "hasSpec must reuse the cached listing");
});

test("preloadCommonSpecs defers loading off the synchronous path", async () => {
  const before = loadFigSpecRequests.length;

  preloadCommonSpecs();
  assert.equal(
    loadFigSpecRequests.length,
    before,
    "preload must not perform any spec load synchronously",
  );

  // First deferred batch fires at 200ms; later batches follow on idle/timers.
  await new Promise((resolve) => setTimeout(resolve, 450));
  assert.ok(
    loadFigSpecRequests.length > before,
    "deferred batches must eventually load common specs",
  );
});
