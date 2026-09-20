import assert from "node:assert/strict";
import test from "node:test";
import { executeWebSearchProvider, type WebSearchKeyDecrypt } from "./webSearchProviders";
import type { NetcattyBridge } from "../cattyAgent/executor";
import type { WebSearchConfig } from "../types";

const PLACEHOLDER = "__WEB_SEARCH_KEY__";

interface FetchCall {
  url: string;
  method: string;
  headers: Record<string, string>;
  body?: string;
}

interface FakeBridgeOptions {
  allowlistError?: Error;
}

function createFakeBridge(options: FakeBridgeOptions = {}): {
  bridge: NetcattyBridge;
  fetchCalls: FetchCall[];
  allowlistCalls: string[];
} {
  const fetchCalls: FetchCall[] = [];
  const allowlistCalls: string[] = [];
  const bridge = {
    aiExec: async () => ({ ok: true, stdout: "", stderr: "" }),
    aiFetch: async (
      url: string,
      method: string,
      headers: Record<string, string>,
      body?: string,
    ) => {
      fetchCalls.push({ url, method, headers, body });
      return {
        ok: true,
        status: 200,
        data: JSON.stringify({
          results: [{ title: "t", url: "https://example.com", content: "c" }],
        }),
      };
    },
    aiAllowlistAddHost: async (baseURL: string) => {
      if (options.allowlistError) throw options.allowlistError;
      allowlistCalls.push(baseURL);
      return { ok: true };
    },
  };
  return { bridge, fetchCalls, allowlistCalls };
}

function buildConfig(overrides: Partial<WebSearchConfig> = {}): WebSearchConfig {
  return {
    providerId: "tavily",
    apiKey: "enc:v1:sealed-key",
    apiHost: "https://search.example.com",
    enabled: true,
    maxResults: 5,
    ...overrides,
  };
}

test("decrypt success sends the real key header and seeds the allowlist once", async () => {
  const { bridge, fetchCalls, allowlistCalls } = createFakeBridge();
  const decrypt: WebSearchKeyDecrypt = async (value) =>
    value === "enc:v1:sealed-key" ? "real-plaintext-key" : undefined;

  const results = await executeWebSearchProvider(
    bridge,
    buildConfig(),
    "netcatty test",
    5,
    decrypt,
  );

  assert.equal(fetchCalls.length, 1);
  const call = fetchCalls[0];
  assert.ok(call, "expected one aiFetch call");
  assert.equal(call.url, "https://search.example.com/search");
  assert.equal(call.method, "POST");
  assert.equal(call.headers["Authorization"], "Bearer real-plaintext-key");
  assert.ok(!JSON.stringify(call.headers).includes(PLACEHOLDER));
  assert.deepEqual(allowlistCalls, ["https://search.example.com"]);
  assert.equal(results.length, 1);
  assert.equal(results[0]?.title, "t");
});

test("decrypt failure keeps the placeholder and still sends the request", async () => {
  // decryptField contract: empty/undefined result or a thrown error both mean
  // "could not decrypt".
  const decryptEmpty: WebSearchKeyDecrypt = async () => "";
  const decryptUndefined: WebSearchKeyDecrypt = async () => undefined;
  const decryptThrows: WebSearchKeyDecrypt = async () => {
    throw new Error("seal open failed");
  };
  const decrypts = [
    ["empty string", decryptEmpty],
    ["undefined", decryptUndefined],
    ["thrown", decryptThrows],
  ] as const;

  for (const [label, decrypt] of decrypts) {
    const { bridge, fetchCalls, allowlistCalls } = createFakeBridge();
    const results = await executeWebSearchProvider(
      bridge,
      buildConfig(),
      "netcatty test",
      5,
      decrypt,
    );
    assert.equal(fetchCalls.length, 1, `fetch must still run when decrypt is ${label}`);
    assert.equal(
      fetchCalls[0]?.headers["Authorization"],
      `Bearer ${PLACEHOLDER}`,
      `placeholder must be kept when decrypt is ${label}`,
    );
    assert.deepEqual(allowlistCalls, ["https://search.example.com"]);
    assert.equal(results.length, 1);
  }
});

test("decrypt passthrough keeps the placeholder: ciphertext must not go out", async () => {
  // secureFieldAdapter.decryptField returns the sealed value unchanged when
  // the shell has no credentialsDecrypt bridge; that passthrough is not a
  // decryption and must never be sent as the API key.
  const decryptPassthrough: WebSearchKeyDecrypt = async (value) => value ?? null;
  const { bridge, fetchCalls, allowlistCalls } = createFakeBridge();
  const results = await executeWebSearchProvider(
    bridge,
    buildConfig(),
    "netcatty test",
    5,
    decryptPassthrough,
  );
  assert.equal(fetchCalls.length, 1);
  assert.equal(
    fetchCalls[0]?.headers["Authorization"],
    `Bearer ${PLACEHOLDER}`,
    "passthrough result must keep the placeholder",
  );
  assert.deepEqual(allowlistCalls, ["https://search.example.com"]);
  assert.equal(results.length, 1);
});

test("no apiKey behaves as before: placeholder is sent", async () => {
  const { bridge, fetchCalls, allowlistCalls } = createFakeBridge();
  const seenValues: Array<string | undefined> = [];
  const decrypt: WebSearchKeyDecrypt = async (value) => {
    seenValues.push(value);
    return value;
  };

  await executeWebSearchProvider(
    bridge,
    buildConfig({ apiKey: undefined }),
    "netcatty test",
    5,
    decrypt,
  );

  assert.deepEqual(seenValues, [undefined]);
  assert.equal(fetchCalls.length, 1);
  assert.equal(fetchCalls[0]?.headers["Authorization"], `Bearer ${PLACEHOLDER}`);
  assert.deepEqual(allowlistCalls, ["https://search.example.com"]);
});

test("allowlist call throwing does not affect the fetch", async () => {
  const { bridge, fetchCalls, allowlistCalls } = createFakeBridge({
    allowlistError: new Error("allowlist rejected"),
  });
  const decrypt: WebSearchKeyDecrypt = async (value) => value;

  const results = await executeWebSearchProvider(
    bridge,
    buildConfig(),
    "netcatty test",
    5,
    decrypt,
  );

  assert.equal(allowlistCalls.length, 0);
  assert.equal(fetchCalls.length, 1);
  assert.equal(fetchCalls[0]?.url, "https://search.example.com/search");
  assert.equal(results.length, 1);
});

test("no apiHost skips the allowlist call entirely", async () => {
  const { bridge, fetchCalls, allowlistCalls } = createFakeBridge();
  const decrypt: WebSearchKeyDecrypt = async (value) => value;

  await executeWebSearchProvider(
    bridge,
    buildConfig({ apiHost: undefined }),
    "netcatty test",
    5,
    decrypt,
  );

  assert.deepEqual(allowlistCalls, []);
  assert.equal(fetchCalls.length, 1);
  // Preset default host is used for the URL even without a custom apiHost.
  assert.equal(fetchCalls[0]?.url, "https://api.tavily.com/search");
});
