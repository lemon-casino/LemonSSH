// ProfileClient (P2-02): the frontend adapter for the host-owned
// transactional profile store. Raw values are opaque bytes; the Wails wire
// transports them base64-encoded, so the base64 form is this client's wire
// currency and text/JSON helpers encode and decode around it.

import * as bindings from "../wails/bindings/github.com/binaricat/netcatty/cmd/netcatty/profileservice.js";

export interface ProfileMutation {
  domain: string;
  key: string;
  /** base64-encoded value; ignored when delete is true */
  valueBase64?: string;
  delete?: boolean;
}

export interface ProfileWriteResult {
  revision: number;
}

export interface ProfileClient {
  revision(): Promise<number>;
  /** Returns the raw value base64-encoded, or undefined when absent. */
  getRawBase64(domain: string, key: string): Promise<string | undefined>;
  setRawBase64(domain: string, key: string, valueBase64: string): Promise<void>;
  deleteRaw(domain: string, key: string): Promise<void>;
  write(expectedRevision: number, mutations: ProfileMutation[]): Promise<ProfileWriteResult>;
  domains(): Promise<string[]>;
  domainKeys?(domain: string): Promise<string[]>;
}

function toWireMutations(mutations: ProfileMutation[]): Array<{ Domain: string; Key: string; Value: string | null; Delete: boolean }> {
  return mutations.map((mutation) => ({
    Domain: mutation.domain,
    Key: mutation.key,
    Value: mutation.delete ? null : (mutation.valueBase64 ?? ""),
    Delete: mutation.delete ?? false,
  }));
}

export function createProfileClient(): ProfileClient {
  return {
    revision: async () => Number(await bindings.Revision()),
    getRawBase64: async (domain, key) => {
      try {
        return await bindings.GetRaw(domain, key);
      } catch (error) {
        if (error instanceof Error && error.message.includes("profile key not found")) return undefined;
        throw error;
      }
    },
    setRawBase64: (domain, key, valueBase64) => bindings.SetRaw(domain, key, valueBase64),
    deleteRaw: (domain, key) => bindings.DeleteRaw(domain, key),
    write: async (expectedRevision, mutations) => {
      const result = await bindings.Write(expectedRevision, toWireMutations(mutations));
      return { revision: Number(result.Revision) };
    },
    domains: () => bindings.Domains(),
    domainKeys: async (domain) => {
      const listed = await (bindings as { DomainKeys?: (domain: string) => Promise<string[]> }).DomainKeys?.(domain);
      return listed ?? [];
    },
  };
}

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

function toBase64(bytes: Uint8Array): string {
  if (typeof Buffer !== "undefined") return Buffer.from(bytes).toString("base64");
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function fromBase64(value: string): Uint8Array {
  if (typeof Buffer !== "undefined") return new Uint8Array(Buffer.from(value, "base64"));
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

/** localStorage-style text payload helper. */
export async function getRawText(client: ProfileClient, domain: string, key: string): Promise<string | undefined> {
  let encoded: string | undefined;
  try {
    encoded = await client.getRawBase64(domain, key);
  } catch (error) {
    if (error instanceof Error && error.message.includes("profile key not found")) return undefined;
    throw error;
  }
  if (encoded === undefined) return undefined;
  return textDecoder.decode(fromBase64(encoded));
}

export function setRawText(client: ProfileClient, domain: string, key: string, value: string): Promise<void> {
  return client.setRawBase64(domain, key, toBase64(textEncoder.encode(value)));
}
