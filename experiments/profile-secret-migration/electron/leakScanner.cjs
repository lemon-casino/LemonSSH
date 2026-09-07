"use strict";

const fs = require("node:fs");
const path = require("node:path");

const MAX_SCAN_FILE_BYTES = 8 * 1024 * 1024;
const MAX_SCAN_TOTAL_BYTES = 64 * 1024 * 1024;

function canaryEncodings(value) {
  const utf8 = Buffer.from(value, "utf8");
  return [
    utf8,
    Buffer.from(value, "utf16le"),
    Buffer.from(utf8.toString("base64"), "ascii"),
    Buffer.from(utf8.toString("hex"), "ascii"),
  ];
}

function scanBuffers(buffers, canaries) {
  const needles = canaries.flatMap(canaryEncodings);
  for (const candidate of buffers) {
    const haystack = Buffer.isBuffer(candidate) ? candidate : Buffer.from(String(candidate), "utf8");
    for (const needle of needles) {
      if (needle.length > 0 && haystack.indexOf(needle) !== -1) return false;
    }
  }
  return true;
}

function scanTree(root, canaries) {
  const buffers = [];
  let total = 0;
  let crashDump = false;
  const visit = (entry) => {
    const stat = fs.lstatSync(entry);
    if (stat.isSymbolicLink()) throw new Error("scan tree contains symbolic link");
    if (stat.isDirectory()) {
      for (const name of fs.readdirSync(entry)) visit(path.join(entry, name));
      return;
    }
    if (!stat.isFile()) return;
    if (/\.(?:dmp|mdmp|core)$/i.test(entry) || /crashpad.*\.db$/i.test(entry)) crashDump = true;
    if (stat.size > MAX_SCAN_FILE_BYTES || total + stat.size > MAX_SCAN_TOTAL_BYTES) {
      throw new Error("leak scan bounds exceeded");
    }
    const data = fs.readFileSync(entry);
    total += data.length;
    buffers.push(data);
  };
  if (fs.existsSync(root)) visit(root);
  return { passed: !crashDump && scanBuffers(buffers, canaries), crashDump, bytesScanned: total };
}

function runLeakScan({ canaries, capturedStdout, capturedStderr, marker, receipt, root, argv, envValues }) {
  if (!Array.isArray(canaries) || canaries.length === 0) throw new Error("missing canaries");
  const memoryPassed = scanBuffers([
    ...(capturedStdout || []),
    ...(capturedStderr || []),
    JSON.stringify(marker || {}),
    JSON.stringify(receipt || {}),
    ...(argv || []),
    ...(envValues || []),
  ], canaries);
  const tree = scanTree(root, canaries);
  return memoryPassed && tree.passed && !tree.crashDump;
}

module.exports = { canaryEncodings, runLeakScan, scanBuffers, scanTree };
