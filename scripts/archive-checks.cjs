"use strict";

const path = require("node:path");

function parseSums(text) {
  const sums = new Map();
  for (const line of text.split(/\r?\n/)) {
    const match = line.match(/^([0-9a-f]{64})\s+\*?\s*(\S+)\s*$/i);
    if (match) sums.set(match[2], match[1].toLowerCase());
  }
  return sums;
}

function assertSafeTarEntry(entry) {
  if (!entry || entry.includes("\\") || entry.startsWith("/") || /^[A-Za-z]:/.test(entry)) {
    throw new Error(`unsafe tar path entry: ${entry}`);
  }
  const parts = entry.split("/").filter(Boolean);
  if (parts.length === 0 || parts.some((part) => part === ".." || part === ".")) {
    throw new Error(`unsafe tar path entry: ${entry}`);
  }
}

function resolveTarArchiveInvocation(archivePath, platform = process.platform) {
  const pathApi = platform === "win32" ? path.win32 : path;
  return { cwd: pathApi.dirname(archivePath), archive: pathApi.basename(archivePath) };
}

function validateTarEntries(entries) {
  if (entries.length === 0) throw new Error("tarball is empty");
  for (const entry of entries) assertSafeTarEntry(entry);
}

module.exports = { parseSums, resolveTarArchiveInvocation, validateTarEntries };
