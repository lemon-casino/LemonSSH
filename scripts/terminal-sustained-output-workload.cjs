"use strict";

const TERMINAL_SUSTAINED_OUTPUT_WORKLOAD = Object.freeze({
  id: "xterm-keyword-highlight-sustained-output",
  version: 1,
  encoding: "utf8",
  canonicalBenchmarkSource: "scripts/xterm-keyword-highlight-throughput.live.test.cjs",
  generatorSource: "scripts/terminal-sustained-output-workload.cjs",
  defaultChunkCount: 1600,
  parameters: Object.freeze({
    date: "2026-08-13",
    infoText: "INFO",
    warnText: "WARN",
    errorText: "ERROR",
    failureText: "failed",
    linesPerChunk: 64,
    workerModulo: 32,
    ipPrefix: "10.2",
    ipOctetModulo: 255,
    secondIpOctetChunkMultiplier: 7,
    payloadCharacter: "x",
    payloadLength: 24,
    lineEnding: "\r\n",
  }),
});

function makeTerminalSustainedOutputChunk(index) {
  const parameters = TERMINAL_SUSTAINED_OUTPUT_WORKLOAD.parameters;
  let chunk = "";
  for (let line = 0; line < parameters.linesPerChunk; line += 1) {
    chunk += parameters.date + " " + parameters.infoText + " worker="
      + (line % parameters.workerModulo) + " " + parameters.warnText + " "
      + parameters.errorText + " " + parameters.failureText + " from "
      + parameters.ipPrefix + "." + ((index + line) % parameters.ipOctetModulo)
      + "." + ((index * parameters.secondIpOctetChunkMultiplier + line)
        % parameters.ipOctetModulo)
      + " payload=" + parameters.payloadCharacter.repeat(parameters.payloadLength)
      + parameters.lineEnding;
  }
  return chunk;
}

function makeTerminalSustainedOutputChunks(
  chunkCount = TERMINAL_SUSTAINED_OUTPUT_WORKLOAD.defaultChunkCount,
) {
  return Array.from(
    { length: chunkCount },
    (_, index) => makeTerminalSustainedOutputChunk(index),
  );
}

module.exports = {
  TERMINAL_SUSTAINED_OUTPUT_WORKLOAD,
  makeTerminalSustainedOutputChunk,
  makeTerminalSustainedOutputChunks,
};
