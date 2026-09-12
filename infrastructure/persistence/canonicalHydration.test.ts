import assert from "node:assert/strict";
import { test } from "node:test";

import { hydrateCanonicalProfile, diffCanonicalSources } from "./canonicalHydration";

// SYNC-01 canonical cutover harness: proves the hydration state machine
// (hydrate / promote / heal / skip) is lossless and that AI-managed keys
// never cross the localStorage <-> Go profile store boundary.

type Sources = {
  local: Map<string, string | null>;
  profile: Map<string, string>;
  promoted: Array<{ domain: string; key: string; value: string }>;
};

function makeSources(localInitial: Record<string, string> = {}, profileInitial: Record<string, string> = {}): Sources {
  const local = new Map<string, string | null>(Object.entries(localInitial));
  const profile = new Map<string, string>(Object.entries(profileInitial));
  const promoted: Array<{ domain: string; key: string; value: string }> = [];
  return {
    local,
    profile,
    promoted,
  };
}

function readerOf(sources: Sources) {
  return {
    getRawText: async (_domain: string, key: string) => sources.profile.get(key),
    domainKeys: async () => Array.from(sources.profile.keys()),
  };
}

function writerOf(sources: Sources) {
  return {
    setRawText: async (domain: string, key: string, value: string) => {
      sources.profile.set(key, value);
      sources.promoted.push({ domain, key, value });
    },
  };
}

function localOf(sources: Sources) {
  return {
    readString: (key: string) => sources.local.get(key) ?? null,
    writeString: (key: string, value: string) => {
      sources.local.set(key, value);
      return true;
    },
  };
}

test("canonical hydrate fills empty local keys from the profile store losslessly", async () => {
  const sources = makeSources({}, { theme: "dark", "netcatty_hosts_v1": "[{\"id\":\"h1\"}]" });
  const outcome = await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "settings", [
    "theme",
    "netcatty_hosts_v1",
    "missing-both",
  ]);
  assert.equal(sources.local.get("theme"), "dark");
  assert.equal(sources.local.get("netcatty_hosts_v1"), "[{\"id\":\"h1\"}]");
  assert.deepEqual(outcome.hydratedFromProfile.sort(), ["netcatty_hosts_v1", "theme"]);
  assert.deepEqual(outcome.promotedToProfile, []);
  assert.deepEqual(outcome.healedToProfile, []);
  assert.deepEqual(outcome.divergences, []);
  assert.equal(sources.promoted.length, 0);
});

test("canonical hydrate promotes legacy local-only values into the profile store", async () => {
  const sources = makeSources({ "netcatty_hosts_v1": "[legacy]" });
  const outcome = await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "vault", [
    "netcatty_hosts_v1",
  ]);
  assert.equal(sources.profile.get("netcatty_hosts_v1"), "[legacy]");
  assert.deepEqual(outcome.promotedToProfile, ["netcatty_hosts_v1"]);
  assert.deepEqual(outcome.healedToProfile, ["netcatty_hosts_v1"]);
  assert.deepEqual(outcome.divergences, [{ domain: "vault", key: "netcatty_hosts_v1", conflict: false }]);
  // Legacy bytes survive the promotion unchanged.
  assert.equal(sources.promoted[0]?.value, "[legacy]");
});

test("canonical hydrate heals conflicts toward the local value and reports them", async () => {
  const sources = makeSources({ theme: "local-newer" }, { theme: "stale-mirror" });
  const outcome = await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "settings", [
    "theme",
  ]);
  assert.equal(sources.local.get("theme"), "local-newer");
  assert.equal(sources.profile.get("theme"), "local-newer");
  assert.deepEqual(outcome.healedToProfile, ["theme"]);
  assert.deepEqual(outcome.divergences, [{ domain: "settings", key: "theme", conflict: true }]);
  // A conflict is not a promotion: the local value already existed in a prior
  // session, the profile heal only restores convergence.
  assert.deepEqual(outcome.promotedToProfile, []);
});

test("canonical hydrate treats equal values as converged no-ops", async () => {
  const sources = makeSources({ theme: "same" }, { theme: "same" });
  const outcome = await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "settings", [
    "theme",
  ]);
  assert.deepEqual(outcome.hydratedFromProfile, []);
  assert.deepEqual(outcome.promotedToProfile, []);
  assert.deepEqual(outcome.healedToProfile, []);
  assert.deepEqual(outcome.divergences, []);
  assert.equal(sources.promoted.length, 0);
});

test("AI-managed keys never cross the boundary in either direction", async () => {
  const sources = makeSources(
    { "netcatty_ai_providers_v1": "[local-ai]", "netcatty.aiDebug.hide": "x" },
    { "netcatty_ai_providers_v1": "[profile-ai]", "netcatty.aiDebug.profile": "y" },
  );
  const outcome = await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "settings", [
    "netcatty_ai_providers_v1",
    "netcatty.aiDebug.hide",
    "netcatty.aiDebug.profile",
  ]);
  // Local AI values stay untouched and are never promoted into the store.
  assert.equal(sources.local.get("netcatty_ai_providers_v1"), "[local-ai]");
  assert.equal(sources.local.get("netcatty.aiDebug.hide"), "x");
  // Profile AI values are never hydrated down into the local cache.
  assert.equal(sources.local.has("netcatty.aiDebug.profile"), false);
  assert.deepEqual(outcome.hydratedFromProfile, []);
  assert.deepEqual(outcome.promotedToProfile, []);
  assert.deepEqual(outcome.healedToProfile, []);
  assert.equal(sources.promoted.length, 0);
});

test("diffCanonicalSources reports coverage and mismatches without writing", async () => {
  const sources = makeSources(
    { theme: "a", orphan: "b" },
    { theme: "a", stale: "c", remoteOnly: "d" },
  );
  const diff = await diffCanonicalSources(readerOf(sources), localOf(sources), "settings", [
    "theme",
    "orphan",
    "stale",
    "remoteOnly",
    "netcatty_ai_sessions_v1",
  ]);
  const byKey = new Map(diff.map((entry) => [entry.key, entry]));
  assert.deepEqual(byKey.get("theme"), { domain: "settings", key: "theme", inLocal: true, inProfile: true, mismatch: false });
  assert.deepEqual(byKey.get("orphan"), { domain: "settings", key: "orphan", inLocal: true, inProfile: false, mismatch: false });
  assert.deepEqual(byKey.get("stale"), { domain: "settings", key: "stale", inLocal: false, inProfile: true, mismatch: false });
  assert.deepEqual(byKey.get("remoteOnly"), { domain: "settings", key: "remoteOnly", inLocal: false, inProfile: true, mismatch: false });
  // AI keys are excluded from the differential entirely.
  assert.equal(byKey.has("netcatty_ai_sessions_v1"), false);
});

test("canonical hydrate state machine: full cutover boot sequence converges both stores", async () => {
  // Simulate the three boot phases: legacy profile -> first run -> restart.
  // Phase 1: legacy data only in localStorage; first run promotes it.
  const sources = makeSources({ "netcatty_hosts_v1": "[hosts]", "theme": "dark" });
  await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "vault", ["netcatty_hosts_v1"]);
  await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "settings", ["theme"]);
  assert.equal(sources.profile.get("netcatty_hosts_v1"), "[hosts]");
  assert.equal(sources.profile.get("theme"), "dark");

  // Phase 2: wipe the local cache (profile reset / new machine sharing the
  // Go store); the restart hydrates everything back from the profile store.
  sources.local.clear();
  await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "vault", ["netcatty_hosts_v1"]);
  await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "settings", ["theme"]);
  assert.equal(sources.local.get("netcatty_hosts_v1"), "[hosts]");
  assert.equal(sources.local.get("theme"), "dark");

  // Phase 3: steady state - converged sources produce no writes at all.
  const before = sources.promoted.length;
  await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "vault", ["netcatty_hosts_v1"]);
  await hydrateCanonicalProfile(readerOf(sources), writerOf(sources), localOf(sources), "settings", ["theme"]);
  assert.equal(sources.promoted.length, before);
});
