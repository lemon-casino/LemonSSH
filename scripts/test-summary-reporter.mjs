// Minimal node:test reporter: prints the final pass/fail/skip counts and one
// line per failing test. The built-in spec reporter emits one line per test,
// which pushes `npm test` stdout past CI log caps (650 KB+ for ~7.4k tests);
// this keeps a full run at a few hundred bytes while still surfacing every
// failure with its error on red runs.
import { Transform } from "node:stream";

const MAX_ERROR_CHARS = 1000;

const formatError = (error) => {
  if (!error) return "";
  if (typeof error === "string") return error;
  if (error instanceof Error) return `${error.name}: ${error.message}`;
  const message = error.message ?? error.errorMessage ?? error;
  return typeof message === "string" ? message : JSON.stringify(message);
};

const truncate = (text) => (text.length > MAX_ERROR_CHARS
  ? `${text.slice(0, MAX_ERROR_CHARS)}… [truncated ${text.length - MAX_ERROR_CHARS} chars]`
  : text);

export default class TestSummaryReporter extends Transform {
  #failures = [];
  #seenFailures = new Set();

  constructor() {
    super({ readable: true, writable: true, objectMode: true });
  }

  _transform(chunk, _encoding, callback) {
    if (chunk?.type === "test:fail") {
      const data = chunk.data ?? {};
      const name = data.name ?? "unknown test";
      const errorText = truncate(formatError(data.details?.error ?? data.details?.failureType));
      const key = `${name}::${errorText.slice(0, 120)}`;
      if (!this.#seenFailures.has(key)) {
        this.#seenFailures.add(key);
        this.#failures.push({ name, errorText });
      }
    }
    // The last summary (no `file`) aggregates the whole run.
    if (chunk?.type === "test:summary" && chunk.data && !chunk.data.file) {
      this.#emitSummary(chunk.data);
    }
    callback();
  }

  #emitSummary(summary) {
    const counts = summary.counts ?? {};
    const lines = [
      `# tests ${counts.tests ?? 0}`,
      `# suites ${counts.suites ?? 0}`,
      `# pass ${counts.passed ?? 0}`,
      `# fail ${counts.failed ?? 0}`,
      `# cancelled ${counts.cancelled ?? 0}`,
      `# skipped ${counts.skipped ?? 0}`,
      `# todo ${counts.todo ?? 0}`,
      `# duration_ms ${summary.duration_ms ?? 0}`,
    ];
    for (const { name, errorText } of this.#failures) {
      lines.push(`✖ ${name}${errorText ? `\n    ${errorText.replaceAll("\n", "\n    ")}` : ""}`);
    }
    lines.push(summary.success ? "TEST RUN OK" : "TEST RUN FAILED");
    this.push(`${lines.join("\n")}\n`);
  }

  _flush(callback) {
    callback();
  }
}
