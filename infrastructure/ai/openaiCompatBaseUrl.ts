/**
 * OpenAI-compatible SDKs append paths such as /chat/completions or /responses
 * to the configured base URL. Most gateways expose those routes below /v1,
 * while settings are often entered as a bare origin.
 */
export function normalizeOpenAICompatSdkBaseURL(baseURL: string): string {
  const trimmed = baseURL.trim().replace(/\/+$/, "");
  if (!trimmed) return trimmed;

  try {
    const parsed = new URL(trimmed);
    if ((!parsed.pathname || parsed.pathname === "/") && !parsed.search && !parsed.hash) {
      return `${trimmed}/v1`;
    }
  } catch {
    // Leave malformed or relative values unchanged so normal validation can
    // report them at the request boundary.
  }

  return trimmed;
}
