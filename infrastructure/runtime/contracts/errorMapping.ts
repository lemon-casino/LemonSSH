// Cross-language error/cancel mapping (P1-03). Maps runtime failures onto the
// stable contracts ErrorCode union so frontend callers can branch on codes
// with the same semantics the Go side enforces via contracts.AsError.

import type { ServiceError } from "./generated/contracts";

const SERVICE_ERROR_CODES = new Set([
  "netcatty.cancelled",
  "netcatty.conflict",
  "netcatty.deadline_exceeded",
  "netcatty.internal",
  "netcatty.invalid_request",
  "netcatty.not_found",
  "netcatty.unavailable",
  "netcatty.unknown",
] as const);

export type ServiceErrorCode = typeof SERVICE_ERROR_CODES extends Set<infer TCode> ? TCode : never;

export function isServiceErrorCode(value: string): value is ServiceErrorCode {
  return SERVICE_ERROR_CODES.has(value as ServiceErrorCode);
}

export function toServiceError(error: unknown): ServiceError {
  if (error instanceof Error) {
    if (error.name === "BridgeUnavailableError") {
      return { code: "netcatty.unavailable", message: error.message };
    }
    if (error.name === "AbortError" || error.name === "TimeoutError") {
      return {
        code: error.name === "AbortError" ? "netcatty.cancelled" : "netcatty.deadline_exceeded",
        message: error.message,
        retryable: error.name === "TimeoutError",
      };
    }
    return { code: "netcatty.internal", message: error.message };
  }
  return { code: "netcatty.unknown", message: String(error) };
}
