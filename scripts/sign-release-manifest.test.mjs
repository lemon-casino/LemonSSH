import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { realpath } from "node:fs/promises";

// macOS /var is a system symlink; tests use the physical temp directory so
// output paths behave consistently across platforms.
const tempRoot = await realpath(tmpdir());

import {
  checksumArtifactFor,
  generateKeyPair,
  goJSONString,
  goMarshalManifest,
  keyPairFromSeed,
  parseArgs,
  publicKeyFromPrivateHex,
  signManifestHex,
  signReleaseManifest,
  verifyManifestHex,
} from "./sign-release-manifest.mjs";

// RFC 8032 ed25519 TEST 1 vectors: a public test key, never a release key.
const RFC_SEED = "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60";
const RFC_PUBLIC = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a";

// Manifest inputs shared with internal/app/updateuse/manifest_verify_test.go.
const VECTOR_MANIFEST = {
  version: "1.2.3",
  publishedMs: 1699999999000,
  artifacts: {
    "windows-amd64": "a".repeat(64),
    "linux-amd64": "b".repeat(64),
    "darwin-arm64": "c".repeat(64),
  },
  notes: 'LemonSSH 1.2.3 <b>&safe</b> "quoted"  line',
};

// The manifest body pinned byte-for-byte on BOTH sides: this file compares the
// Node signer's output against it, and internal/app/updateuse/
// manifest_verify_test.go feeds the same bytes to the Go verifier. (The vector
// itself was emitted by the Node signer for the RFC seed — see the Go-side
// constant comment.) Any byte of drift between the two implementations makes
// the cryptographic assertions below fail.
const GO_SIGNED_MANIFEST = String.raw`{"version":"1.2.3","publishedMs":1699999999000,"artifacts":{"darwin-arm64":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","linux-amd64":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","windows-amd64":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"notes":"LemonSSH 1.2.3 \u003cb\u003e\u0026safe\u003c/b\u003e \"quoted\"  line","signature":"xqasm1+4dQqRkBjafjLOR0knkd0ydOIUHDrLUMZMeKDU6FuOCjkJVfOSzrdsT+kIYVkaZKKtA9lzT+2iZidYCA=="}`;

test("goJSONString escapes exactly like encoding/json with HTML escaping", () => {
  assert.equal(goJSONString("plain"), '"plain"');
  assert.equal(goJSONString('say "hi"'), '"say \\"hi\\""');
  assert.equal(goJSONString("back\\slash"), '"back\\\\slash"');
  assert.equal(goJSONString("<b>&</b>"), '"\\u003cb\\u003e\\u0026\\u003c/b\\u003e"');
  assert.equal(goJSONString("line\nbreak\ttab\rret"), '"line\\nbreak\\ttab\\rret"');
  assert.equal(goJSONString("\u0000\u001f"), '"\\u0000\\u001f"');
  assert.equal(goJSONString("\u2028\u2029"), '"\\u2028\\u2029"');
  // Untouched characters: slash and plus stay literal (Go does not escape them).
  assert.equal(goJSONString("a/b+c"), '"a/b+c"');
});

test("goMarshalManifest reproduces the Go struct order, sorted artifact keys and omitempty notes", () => {
  const signingBody = goMarshalManifest({ ...VECTOR_MANIFEST, signature: "" });
  const parsed = JSON.parse(signingBody);
  assert.deepEqual(Object.keys(parsed), ["version", "publishedMs", "artifacts", "notes", "signature"]);
  assert.deepEqual(Object.keys(parsed.artifacts), ["darwin-arm64", "linux-amd64", "windows-amd64"]);
  // The signed body must be exactly the Go manifest minus the signature
  // value; re-setting Go's signature must yield byte-identical JSON.
  const goSignature = JSON.parse(GO_SIGNED_MANIFEST).signature;
  assert.equal(goMarshalManifest({ ...VECTOR_MANIFEST, signature: goSignature }), GO_SIGNED_MANIFEST);
  // omitempty: an empty notes field disappears entirely, signature stays.
  const noNotes = JSON.parse(goMarshalManifest({ ...VECTOR_MANIFEST, notes: "", signature: "" }));
  assert.equal("notes" in noNotes, false);
  assert.equal(noNotes.signature, "");
});

test("signManifestHex signs deterministically and reproduces the Go bytes from the shared inputs", () => {
  const privateKeyHex = `${RFC_SEED}${RFC_PUBLIC}`;
  const first = signManifestHex(VECTOR_MANIFEST, privateKeyHex);
  const second = signManifestHex(VECTOR_MANIFEST, privateKeyHex);
  assert.equal(first.manifestJSON, second.manifestJSON, "signing must be deterministic");
  assert.equal(first.manifestJSON, GO_SIGNED_MANIFEST, "node-signed bytes must equal the Go verifier-side bytes");
  // signingBytesHex is the blank-signature body the Go side re-derives.
  assert.match(first.signingBytesHex, /^[0-9a-f]+$/);
});

test("verifyManifestHex accepts the shared pinned bytes and rejects tampering", () => {
  assert.equal(verifyManifestHex(GO_SIGNED_MANIFEST, RFC_PUBLIC).ok, true);
  const parsed = JSON.parse(GO_SIGNED_MANIFEST);
  const tampered = goMarshalManifest({ ...parsed, version: "9.9.9" });
  assert.equal(verifyManifestHex(tampered, RFC_PUBLIC).ok, false);
  const digestSwapped = goMarshalManifest({
    ...parsed,
    artifacts: { ...parsed.artifacts, "windows-amd64": "d".repeat(64) },
  });
  assert.equal(verifyManifestHex(digestSwapped, RFC_PUBLIC).ok, false);
  assert.throws(() => verifyManifestHex(GO_SIGNED_MANIFEST, "short"), /64 hex chars/);
});

test("keyPairFromSeed and generateKeyPair produce the Go hex shapes", () => {
  const { publicKeyHex, privateKeyHex } = keyPairFromSeed(RFC_SEED);
  assert.equal(publicKeyHex, RFC_PUBLIC);
  assert.equal(privateKeyHex, `${RFC_SEED}${RFC_PUBLIC}`, "private key is seed||public like ed25519.PrivateKey");
  assert.throws(() => keyPairFromSeed("abcd"), /32 bytes/);
  const fresh = generateKeyPair();
  assert.match(fresh.publicKeyHex, /^[0-9a-f]{64}$/);
  assert.match(fresh.privateKeyHex, /^[0-9a-f]{128}$/);
  assert.equal(fresh.privateKeyHex.slice(64), fresh.publicKeyHex);
  assert.equal(publicKeyFromPrivateHex(fresh.privateKeyHex), fresh.publicKeyHex);
  assert.throws(() => publicKeyFromPrivateHex("nothex"), /128 hex chars/);
});

test("checksumArtifactFor maps checksums.txt entries onto platform keys", () => {
  const checksums = [
    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  LemonSSH-1.2.3-windows-amd64.exe",
    "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210  resources/mosh/win32-x64/mosh-client.exe",
    `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  LemonSSH-1.2.3-linux-amd64`,
    "truncated-line",
    "",
  ].join("\n");
  assert.equal(
    checksumArtifactFor(checksums, "1.2.3", "windows", "amd64"),
    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  );
  assert.equal(
    checksumArtifactFor(checksums, "1.2.3", "linux", "amd64"),
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  );
  assert.throws(() => checksumArtifactFor(checksums, "1.2.3", "darwin", "arm64"), /no entry/);
  assert.throws(
    () => checksumArtifactFor("zz  LemonSSH-1.2.3-windows-amd64.exe", "1.2.3", "windows", "amd64"),
    /malformed/,
  );
});

test("checksumArtifactFor normalizes uppercase hex digests to the manifest form", () => {
  const uppercase = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF";
  assert.equal(
    checksumArtifactFor(`${uppercase}  LemonSSH-1.2.3-windows-amd64.exe`, "1.2.3", "windows", "amd64"),
    uppercase.toLowerCase(),
    "uppercase producers must reach the same lowercase manifest digest",
  );
  // An uppercase digest must not be silently accepted as malformed either:
  // mixed-case beyond the hex alphabet still throws.
  assert.throws(
    () => checksumArtifactFor("ZZ  LemonSSH-1.2.3-windows-amd64.exe", "1.2.3", "windows", "amd64"),
    /malformed/,
  );
});

test("parseArgs accepts the documented flags", () => {
  const args = parseArgs([
    "--version", "1.2.3",
    "--notes", "hello",
    "--published-ms", "123",
    "--artifact", "windows-amd64=" + "a".repeat(64),
    "--from-checksums", "checksums.txt", "linux", "amd64",
    "--key", "ff",
    "--key-file", "key.hex",
    "--out", "out.json",
    "--verify", "release-manifest.json",
    "--generate-key",
  ]);
  assert.deepEqual(args, {
    version: "1.2.3",
    notes: "hello",
    publishedMs: 123,
    artifacts: ["windows-amd64=" + "a".repeat(64)],
    checksums: [{ file: "checksums.txt", goos: "linux", goarch: "amd64" }],
    key: "ff",
    keyFile: "key.hex",
    out: "out.json",
    verify: "release-manifest.json",
    generateKey: true,
  });
  assert.throws(() => parseArgs(["--nonsense"]), /unknown argument/);
});

test("signReleaseManifest signs from checksums.txt and emits the injectable public key", async () => {
  const dir = await mkdtemp(path.join(tempRoot, "sign-manifest-"));
  try {
    const digest = "a".repeat(64);
    const checksumsPath = path.join(dir, "checksums.txt");
    await writeFile(checksumsPath, `${digest}  LemonSSH-2.0.0-windows-amd64.exe\n${"b".repeat(64)}  helper.txt\n`);
    const outPath = path.join(dir, "nested", "release-manifest.json");
    const { publicKeyHex, manifestJSON } = await signReleaseManifest([
      "--version", "2.0.0",
      "--notes", "LemonSSH 2.0.0 <release>",
      "--published-ms", "1699999999000",
      "--from-checksums", checksumsPath, "windows", "amd64",
      "--key", `${RFC_SEED}${RFC_PUBLIC}`,
      "--out", outPath,
    ]);
    assert.equal(publicKeyHex, RFC_PUBLIC);
    const written = (await readFile(outPath, "utf8")).replace(/\n$/, "");
    assert.equal(written, manifestJSON);
    assert.equal(verifyManifestHex(written, RFC_PUBLIC).ok, true);
    const parsed = JSON.parse(written);
    assert.deepEqual(parsed.artifacts, { "windows-amd64": digest });
    // The stored file keeps Go's HTML escaping; parsing restores the notes.
    assert.ok(written.includes("LemonSSH 2.0.0 \\u003crelease\\u003e"), "HTML escaping must reach the stored file");
    assert.equal(parsed.notes, "LemonSSH 2.0.0 <release>");
    assert.equal(verifyManifestHex(written.replace("2.0.0", "3.0.0"), RFC_PUBLIC).ok, false);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("signReleaseManifest verifies manifests and rejects bad keys and missing args", async () => {
  const dir = await mkdtemp(path.join(tempRoot, "sign-verify-"));
  try {
    const manifestPath = path.join(dir, "release-manifest.json");
    await writeFile(manifestPath, GO_SIGNED_MANIFEST);
    const { verified } = await signReleaseManifest(["--verify", manifestPath, "--key", `${RFC_SEED}${RFC_PUBLIC}`]);
    assert.equal(verified, true);
    await assert.rejects(
      () => signReleaseManifest(["--verify", manifestPath, "--key", `${"cd".repeat(64)}`]),
      /signature invalid/,
    );
    await assert.rejects(() => signReleaseManifest(["--version", "1.0.0"]), /signing key is required/);
    await assert.rejects(
      () => signReleaseManifest(["--version", "1.0.0", "--key", `${RFC_SEED}${RFC_PUBLIC}`]),
      /no artifacts to sign/,
    );
    await assert.rejects(
      () => signReleaseManifest(["--version", "1.0.0", "--key", `${RFC_SEED}${RFC_PUBLIC}`, "--artifact", "bogus"]),
      /expects <goos-goarch>=<sha256>/,
    );
    await assert.rejects(
      () => signReleaseManifest(["--version", "1.0.0", "--key", `${RFC_SEED}${RFC_PUBLIC}`, "--artifact", `linux-amd64=nothex`]),
      /digest malformed/,
    );
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("signReleaseManifest --generate-key prints a fresh key pair", async () => {
  const logs = [];
  const original = console.log;
  console.log = (line) => logs.push(String(line));
  try {
    const { publicKeyHex, privateKeyHex } = await signReleaseManifest(["--generate-key"]);
    assert.match(publicKeyHex, /^[0-9a-f]{64}$/);
    assert.equal(privateKeyHex.slice(64), publicKeyHex);
    assert.ok(logs.some((line) => line.includes(publicKeyHex)), "public key must be printed for build injection");
  } finally {
    console.log = original;
  }
});
