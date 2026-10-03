#!/usr/bin/env node
// Signed release-manifest production side (pairs with the Go verification
// side in internal/platform/updater + internal/app/updateuse). Builds
// release-manifest.json exactly the way Go's json.Marshal serialises
// updater.ReleaseManifest, signs the blank-signature body with ed25519 and
// prints the public key for the -X main.updatePublicKey build stamp.
//
// Byte compatibility contract (unit tests on both sides pin it):
//   - struct field order: version, publishedMs, artifacts, notes (omitempty),
//     signature (always present, blank while signing);
//   - artifacts is a Go map: keys sorted byte-wise, values lowercase sha256;
//   - Go's default HTML escaping applies (\u003c \u003e \u0026, U+2028/2029);
//   - signatures are std base64 over the blank-signature body; keys are hex
//     (32-byte public, 64-byte private seed||public as ed25519.PrivateKey).
import { createPrivateKey, createPublicKey, randomBytes, sign as cryptoSign, verify as cryptoVerify } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";

const SHA256_PATTERN = /^[0-9a-f]{64}$/;
const PRIVATE_KEY_PATTERN = /^[0-9a-fA-F]{128}$/;
// PKCS#8 DER prefix for an ed25519 private key (RFC 8410): lets Node import
// the raw 32-byte seed that Go's hex private key starts with.
const ED25519_PKCS8_PREFIX = Buffer.from("302e020100300506032b657004220420", "hex");
// SPKI DER prefix for an ed25519 public key (RFC 8410): wraps the raw
// 32-byte key into the DER Node's createPublicKey expects.
const ED25519_SPKI_PREFIX = Buffer.from("302a300506032b6570032100", "hex");

// goJSONString encodes one string exactly like Go's encoding/json with the
// default HTML escaping (json.Marshal).
export function goJSONString(value) {
  let out = '"';
  for (const char of String(value)) {
    const code = char.codePointAt(0);
    if (char === '"') out += '\\"';
    else if (char === "\\") out += "\\\\";
    else if (char === "\n") out += "\\n";
    else if (char === "\r") out += "\\r";
    else if (char === "\t") out += "\\t";
    else if (code < 0x20) out += `\\u${code.toString(16).padStart(4, "0")}`;
    else if (char === "<") out += "\\u003c";
    else if (char === ">") out += "\\u003e";
    else if (char === "&") out += "\\u0026";
    else if (code === 0x2028) out += "\\u2028";
    else if (code === 0x2029) out += "\\u2029";
    else out += char;
  }
  return `${out}"`;
}

function compareGoString(a, b) {
  // Go sort.Strings orders map keys byte-wise; Buffer.compare matches for
  // the ASCII goos-goarch keys and stays correct for non-ASCII too, unlike
  // the default UTF-16 code-unit sort.
  return Buffer.compare(Buffer.from(a, "utf8"), Buffer.from(b, "utf8"));
}

// goMarshalManifest serialises a manifest object the way Go marshals
// updater.ReleaseManifest: fixed field order, byte-wise sorted artifact keys,
// omitempty notes, blank signature kept. This is the exact signing payload.
export function goMarshalManifest({ version, publishedMs, artifacts, notes = "", signature = "" }) {
  const entries = Object.keys(artifacts)
    .sort(compareGoString)
    .map((key) => `${goJSONString(key)}:${goJSONString(artifacts[key])}`);
  const fields = [
    `"version":${goJSONString(version)}`,
    `"publishedMs":${Number(publishedMs)}`,
    `"artifacts":{${entries.join(",")}}`,
  ];
  if (notes !== "") fields.push(`"notes":${goJSONString(notes)}`);
  fields.push(`"signature":${goJSONString(signature)}`);
  return `{${fields.join(",")}}`;
}

// keyPairFromSeed rebuilds the Go-shaped key material from a 32-byte seed:
// hex public key (64 chars) plus the 64-byte ed25519.PrivateKey form
// (seed||public, 128 hex chars) that Go's decodePrivateKey expects.
export function keyPairFromSeed(seed) {
  const rawSeed = Buffer.from(String(seed), "hex");
  if (rawSeed.length !== 32) throw new Error("ed25519 seed must be 32 bytes of hex");
  const privateKeyObject = createPrivateKey({ key: Buffer.concat([ED25519_PKCS8_PREFIX, rawSeed]), format: "der", type: "pkcs8" });
  const publicRaw = Buffer.from(createPublicKey(privateKeyObject).export({ format: "jwk" }).x, "base64url");
  return {
    publicKeyHex: publicRaw.toString("hex"),
    privateKeyHex: Buffer.concat([rawSeed, publicRaw]).toString("hex"),
    privateKeyObject,
  };
}

// generateKeyPair mints a fresh key pair in the same hex shapes.
export function generateKeyPair() {
  return keyPairFromSeed(randomBytes(32).toString("hex"));
}

function privateKeyObjectFromHex(privateKeyHex) {
  const clean = String(privateKeyHex).trim();
  if (!PRIVATE_KEY_PATTERN.test(clean)) throw new Error("private key must be 128 hex chars (ed25519.PrivateKey = seed||public)");
  return createPrivateKey({ key: Buffer.concat([ED25519_PKCS8_PREFIX, Buffer.from(clean.slice(0, 64), "hex")]), format: "der", type: "pkcs8" });
}

// publicKeyFromPrivateHex derives the injectable 64-hex-char public key.
export function publicKeyFromPrivateHex(privateKeyHex) {
  const clean = String(privateKeyHex).trim();
  if (!PRIVATE_KEY_PATTERN.test(clean)) throw new Error("private key must be 128 hex chars (ed25519.PrivateKey = seed||public)");
  const { publicKeyHex } = keyPairFromSeed(clean.slice(0, 64));
  return publicKeyHex;
}

// signManifestHex mirrors updater.SignManifest: sign the blank-signature
// body, then return the manifest bytes with the std-base64 signature set.
export function signManifestHex(manifest, privateKeyHex) {
  const privateKeyObject = privateKeyObjectFromHex(privateKeyHex);
  const signingBytes = Buffer.from(goMarshalManifest({ ...manifest, signature: "" }), "utf8");
  const signature = cryptoSign(null, signingBytes, privateKeyObject);
  return {
    signingBytesHex: signingBytes.toString("hex"),
    manifestJSON: goMarshalManifest({ ...manifest, signature: signature.toString("base64") }),
  };
}

// verifyManifestHex mirrors updater.VerifyManifest's signature half (the
// timestamp envelope stays a Go-side concern): re-serialise with the parsed
// signature blanked and check the ed25519 signature.
export function verifyManifestHex(manifestJSON, publicKeyHex) {
  const manifest = JSON.parse(manifestJSON);
  const signingBytes = Buffer.from(goMarshalManifest({ ...manifest, signature: "" }), "utf8");
  const signature = Buffer.from(manifest.signature ?? "", "base64");
  const publicRaw = Buffer.from(String(publicKeyHex).trim(), "hex");
  if (publicRaw.length !== 32) throw new Error("public key must be 64 hex chars");
  const ok = cryptoVerify(null, signingBytes, createPublicKey({ key: Buffer.concat([ED25519_SPKI_PREFIX, publicRaw]), format: "der", type: "spki" }), signature);
  return { ok, manifest };
}

// checksumArtifactFor reads checksums.txt lines and returns the sha256 for
// the platform artifact LemonSSH-<version>-<goos>-<goarch>[.exe]; callers
// key it as "<goos>-<goarch>" which is what internal/app/updateuse looks up
// (manifest.Artifacts[goos+"-"+goarch]). Digests are normalized to the
// lowercase form Go's manifest contract stores, so producers emitting
// uppercase hex (e.g. certutil) still validate against the same bytes.
export function checksumArtifactFor(checksumsContent, version, goos, goarch) {
  const executable = `LemonSSH-${version}-${goos}-${goarch}${goos === "windows" ? ".exe" : ""}`;
  for (const line of String(checksumsContent).split("\n")) {
    const fields = line.trim().split(/\s+/);
    if (fields.length === 2 && fields[1].replace(/^\*/, "") === executable) {
      const digest = fields[0].toLowerCase();
      if (!SHA256_PATTERN.test(digest)) throw new Error(`checksums.txt digest malformed for ${executable}`);
      return digest;
    }
  }
  throw new Error(`checksums.txt has no entry for ${executable}`);
}

export function parseArgs(argv) {
  const args = {
    version: undefined,
    notes: "",
    publishedMs: undefined,
    key: undefined,
    keyFile: undefined,
    out: "release-manifest.json",
    artifacts: [],
    checksums: [],
    verify: undefined,
    generateKey: false,
  };
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    if (arg === "--version") args.version = argv[++index];
    else if (arg === "--notes") args.notes = argv[++index];
    else if (arg === "--published-ms") args.publishedMs = Number(argv[++index]);
    else if (arg === "--artifact") args.artifacts.push(argv[++index]);
    else if (arg === "--from-checksums") args.checksums.push({ file: argv[++index], goos: argv[++index], goarch: argv[++index] });
    else if (arg === "--key") args.key = argv[++index];
    else if (arg === "--key-file") args.keyFile = argv[++index];
    else if (arg === "--out") args.out = argv[++index];
    else if (arg === "--verify") args.verify = argv[++index];
    else if (arg === "--generate-key") args.generateKey = true;
    else throw new Error(`unknown argument ${arg}`);
  }
  return args;
}

async function resolveKey(args) {
  if (args.keyFile) return (await readFile(args.keyFile, "utf8")).trim();
  if (args.key) return args.key;
  throw new Error("a signing key is required: pass --key <hex> or --key-file <path>");
}

export async function signReleaseManifest(argv = process.argv.slice(2)) {
  const args = parseArgs(argv);
  if (args.generateKey) {
    const { publicKeyHex, privateKeyHex } = generateKeyPair();
    console.log(`[sign-release-manifest] private key (hex, KEEP SECRET):\n${privateKeyHex}`);
    console.log(`[sign-release-manifest] public key (hex, inject at build time):\n${publicKeyHex}`);
    return { publicKeyHex, privateKeyHex };
  }
  const privateKeyHex = await resolveKey(args);
  const publicKeyHex = publicKeyFromPrivateHex(privateKeyHex);

  if (args.verify) {
    const manifestJSON = await readFile(args.verify, "utf8");
    const { ok } = verifyManifestHex(manifestJSON, publicKeyHex);
    if (!ok) throw new Error(`signature invalid for ${args.verify}`);
    console.log(`[sign-release-manifest] signature valid: ${args.verify}`);
    return { publicKeyHex, verified: true };
  }

  if (!args.version) throw new Error("--version is required");
  const artifacts = {};
  for (const entry of args.artifacts) {
    const separator = entry.indexOf("=");
    if (separator <= 0) throw new Error(`--artifact expects <goos-goarch>=<sha256>, got ${entry}`);
    const key = entry.slice(0, separator);
    const digest = entry.slice(separator + 1).toLowerCase();
    if (!SHA256_PATTERN.test(digest)) throw new Error(`artifact digest malformed for ${key}`);
    artifacts[key] = digest;
  }
  for (const source of args.checksums) {
    const content = await readFile(source.file, "utf8");
    artifacts[`${source.goos}-${source.goarch}`] = checksumArtifactFor(content, args.version, source.goos, source.goarch);
  }
  if (Object.keys(artifacts).length === 0) throw new Error("no artifacts to sign: pass --artifact or --from-checksums");

  const manifest = {
    version: args.version,
    publishedMs: args.publishedMs ?? Date.now(),
    artifacts,
    notes: args.notes,
  };
  const { manifestJSON } = signManifestHex(manifest, privateKeyHex);
  await mkdir(path.dirname(path.resolve(args.out)), { recursive: true });
  await writeFile(args.out, `${manifestJSON}\n`, "utf8");
  console.log(`[sign-release-manifest] signed ${Object.keys(artifacts).length} artifact(s) for version ${args.version} -> ${args.out}`);
  console.log(`[sign-release-manifest] public key (hex): ${publicKeyHex}`);
  console.log(`[sign-release-manifest] inject at build time: --update-public-key ${publicKeyHex}`);
  return { publicKeyHex, manifestJSON };
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  await signReleaseManifest();
}
