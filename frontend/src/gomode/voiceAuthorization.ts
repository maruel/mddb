// Validates voice endpoint URLs and exchanges mddb login credentials for scoped gateway authorization.

import type { ServiceAuthorization } from "@maruel/gomode/sdk/voicegateway/types.gen.ts";

/** Gateway and issuer URLs are secure origins, allowing HTTP for loopback development. */
export function voiceOrigin(value: string, origin: string): URL {
  if (value !== "/" && !/^https?:\/\/[^/?#@\\]+\/?$/.test(value)) {
    throw new Error("Voice gateway and issuer must be secure origins");
  }
  const url = new URL(value, origin);
  const host = url.hostname;
  const loopback = host === "localhost" || host === "[::1]" || /^127\./.test(host);
  if (
    (url.protocol !== "https:" && !(url.protocol === "http:" && loopback)) ||
    url.username ||
    url.password ||
    url.pathname !== "/" ||
    url.search ||
    url.hash ||
    value.includes("?") ||
    value.includes("#")
  )
    throw new Error("Voice gateway and issuer must be secure origins");
  return url;
}

export function mddbEndpoint(value: string, origin: string): string {
  const url = new URL(value, origin);
  if (
    url.origin !== origin ||
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    value.includes("#")
  ) {
    throw new Error("Voice token and MCP endpoints must be same-origin");
  }
  return url.href;
}

/** Errors intentionally exclude response bodies, which may contain credentials. */
export async function voiceAuthorization(
  endpoint: string,
  failureMessage: string,
  expectedSession?: string,
): Promise<ServiceAuthorization> {
  let response: Response;
  try {
    response = await fetch(endpoint, {
      method: "POST",
      ...(expectedSession ? { headers: { "X-MDDB-Session": expectedSession } } : {}),
      cache: "no-store",
      credentials: "same-origin",
      redirect: "error",
    });
  } catch {
    throw new Error(failureMessage);
  }
  if (!response.ok) throw new Error(failureMessage);
  let value: unknown;
  try {
    value = await response.json();
  } catch {
    throw new Error(failureMessage);
  }
  if (
    typeof value !== "object" ||
    value === null ||
    !("kind" in value) ||
    value.kind !== "mddb" ||
    !("instanceID" in value) ||
    typeof value.instanceID !== "string" ||
    !value.instanceID.trim() ||
    !("baseURL" in value) ||
    typeof value.baseURL !== "string" ||
    !value.baseURL ||
    !("token" in value) ||
    typeof value.token !== "string" ||
    !value.token.trim()
  )
    throw new Error(failureMessage);
  try {
    voiceOrigin(value.baseURL, value.baseURL);
  } catch {
    throw new Error(failureMessage);
  }
  return { kind: value.kind, instanceID: value.instanceID, baseURL: value.baseURL, token: value.token };
}
