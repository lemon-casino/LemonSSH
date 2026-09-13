import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

test("syncing while locked opens the unlock dialog instead of toasting Vault is locked", () => {
  const source = readFileSync(new URL("./CloudSyncSettings.tsx", import.meta.url), "utf8");
  const handleSyncIndex = source.indexOf("const handleSync = async");
  const nextHandlerIndex = source.indexOf("const handleResolveConflict", handleSyncIndex);
  assert.notEqual(handleSyncIndex, -1);
  assert.notEqual(nextHandlerIndex, -1);
  const helperIndex = source.indexOf("const promptUnlockInsteadOfError");
  assert.notEqual(helperIndex, -1);
  const body = source.slice(handleSyncIndex, nextHandlerIndex);
  assert.match(body, /promptUnlockInsteadOfError/);
  assert.match(source.slice(helperIndex, handleSyncIndex), /setShowUnlockDialog\(true\)/);
  assert.equal(
    /toast\.error\([\s\S]*Vault is locked/.test(body),
    false,
    "locked sync must prompt for the master key, not toast Vault is locked",
  );
});

test("connecting one OAuth provider does not disconnect another ready provider", () => {
  const source = readFileSync(new URL("./CloudSyncSettings.tsx", import.meta.url), "utf8");
  const fnIndex = source.indexOf("const disconnectOtherProviders = async");
  const fnEnd = source.indexOf("// GitHub Device Flow state", fnIndex);
  assert.notEqual(fnIndex, -1);
  assert.notEqual(fnEnd, -1);
  const body = source.slice(fnIndex, fnEnd);
  assert.equal(
    body.includes("disconnectProvider"),
    false,
    "connecting Google/Drive must not kick GitHub (or any other ready provider) offline",
  );
  assert.match(body, /cancelOAuthConnect/);
});

test("USE_LOCAL conflict resolution forces upload-local without decrypting remote", () => {
  const source = readFileSync(new URL("./CloudSyncSettings.tsx", import.meta.url), "utf8");
  const useLocalIndex = source.indexOf("} else if (resolution === 'USE_LOCAL') {");
  const syncNowIndex = source.indexOf("results = await sync.syncNow(localPayload, {", useLocalIndex);
  const overrideShrinkIndex = source.indexOf("overrideShrink: true,", syncNowIndex);
  const uploadLocalIndex = source.indexOf("conflictActionOverride: 'upload-local',", syncNowIndex);
  const nextBranchIndex = source.indexOf("toast.success(t('cloudSync.resolve.uploaded'));", syncNowIndex);

  assert.notEqual(useLocalIndex, -1);
  assert.notEqual(syncNowIndex, -1);
  assert.notEqual(overrideShrinkIndex, -1);
  assert.notEqual(uploadLocalIndex, -1);
  assert.notEqual(nextBranchIndex, -1);
  assert.ok(
    useLocalIndex < syncNowIndex
      && syncNowIndex < overrideShrinkIndex
      && overrideShrinkIndex < uploadLocalIndex
      && uploadLocalIndex < nextBranchIndex,
    "keep-local must re-sync with conflictActionOverride upload-local so a new master password can overwrite an undecryptable cloud backup",
  );
});
