// Drives the production asynchronous native frontend-tool envelope in built-browser host journeys.

import type { APIRequestContext, Page } from "@playwright/test";
import { expect, installAuthBridge } from "./helpers";

export interface NativeInvocation {
  document: string;
  token: string;
  scope: string;
  invocation: string;
  sessionID?: string;
}
export interface NativeResult extends NativeInvocation {
  result: Record<string, unknown>;
}
interface NativeReadiness {
  document: string;
  token: string;
  scope: string;
  ready: boolean;
  tools: Array<{ name: string }>;
  voiceSessionVersion?: number;
  voiceSessionPrepared?: string | null;
}
interface NativeFixture {
  readiness: NativeReadiness[];
  results: NativeResult[];
  sessionID: string | null;
}
declare global {
  interface Window {
    nativeToolFixture: NativeFixture;
  }
}

export async function installNativeTools(page: Page, request: APIRequestContext, sessionID?: string) {
  const manifest = await request.get("/.well-known/gomode.json");
  expect(manifest.ok()).toBe(true);
  expect(await manifest.json()).toMatchObject({ apiVersion: 1, webShell: { bridgeVersion: 3 } });
  await installAuthBridge(page, {
    document: "focus-document",
    token: "identity-",
    appendRequest: true,
    voiceConnected: true,
    voiceSessionVersion: sessionID === undefined ? undefined : 1,
  });
  await page.addInitScript((initialSessionID) => {
    const fixture: NativeFixture = { readiness: [], results: [], sessionID: initialSessionID ?? null };
    const readiness: { onmessage?: ((event: { data: string }) => void) | null; postMessage(message: string): void } = {
      postMessage(message) {
        const value: NativeReadiness = JSON.parse(message);
        fixture.readiness.push(value);
        if (value.ready && value.voiceSessionVersion === 1)
          queueMicrotask(() =>
            readiness.onmessage?.({
              data: JSON.stringify({ ...value, sessionID: fixture.sessionID }),
            }),
          );
      },
    };
    Object.assign(window, {
      nativeToolFixture: fixture,
      gomodeFrontendReadiness: readiness,
      gomodeFrontendToolResult: { postMessage: (message: string) => fixture.results.push(JSON.parse(message)) },
    });
  }, sessionID);
}

export async function changeNativeVoiceSession(page: Page, sessionID: string | null) {
  await page.evaluate((nextSessionID) => {
    const fixture = window.nativeToolFixture;
    fixture.sessionID = nextSessionID;
    const bound = fixture.readiness.at(-1);
    const bridge = (window as unknown as { gomodeFrontendReadiness: { onmessage(event: { data: string }): void } })
      .gomodeFrontendReadiness;
    bridge.onmessage({ data: JSON.stringify({ ...bound, sessionID: nextSessionID }) });
  }, sessionID);
}

export async function waitNativeReady(page: Page) {
  await expect
    .poll(() => page.evaluate(() => window.nativeToolFixture.readiness.slice(-1)[0]))
    .toMatchObject({ ready: true, tools: [{ name: "focus_node" }] });
}

export async function invokeNativeFocus(
  page: Page,
  nodeId: unknown,
  invocation: string,
  sessionID: string | null,
): Promise<NativeInvocation> {
  return page.evaluate(
    ({ id, call, conversation }) => {
      const ready = window.nativeToolFixture.readiness.slice(-1)[0];
      if (!ready?.ready) throw new Error("Frontend preparation is incomplete");
      const envelope = {
        document: ready.document,
        token: ready.token,
        scope: ready.scope,
        invocation: call,
        ...(conversation === null ? {} : { sessionID: conversation }),
      };
      const host = window as unknown as {
        dispatchGoModeVoiceTool(request: NativeInvocation & { name: string; args: Record<string, unknown> }): void;
      };
      host.dispatchGoModeVoiceTool({ ...envelope, name: "focus_node", args: { nodeId: id } });
      return envelope;
    },
    { id: nodeId, call: invocation, conversation: sessionID },
  );
}

export async function nativeResult(page: Page, invocation: string): Promise<Record<string, unknown>> {
  await expect
    .poll(() =>
      page.evaluate((call) => window.nativeToolFixture.results.find((entry) => entry.invocation === call), invocation),
    )
    .toBeDefined();
  return page.evaluate(
    (call) => window.nativeToolFixture.results.find((entry) => entry.invocation === call)!.result,
    invocation,
  );
}
export async function cancelNativeFocus(page: Page, envelope: NativeInvocation) {
  await page.evaluate((request) => {
    (window as unknown as { cancelGoModeVoiceTool(request: NativeInvocation): void }).cancelGoModeVoiceTool(request);
  }, envelope);
}

export function gate() {
  let resolve!: () => void;
  const promise = new Promise<void>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

interface BrowserToolFixture {
  sent: Array<Record<string, unknown>>;
  receive(frame: Record<string, unknown>): void;
  peerCount: number;
  closed: boolean;
  tracks: MediaStreamTrack[];
}
declare global {
  interface Window {
    browserToolFixture: BrowserToolFixture;
  }
}

/** Replace only the WebRTC peer boundary; speech capture and the Go Mode runtime remain real. */
export async function installBrowserTools(page: Page) {
  await page.addInitScript(() => {
    const fixture: BrowserToolFixture = {
      sent: [],
      receive() {
        throw new Error("Voice transport is not connected");
      },
      peerCount: 0,
      closed: false,
      tracks: [],
    };
    window.browserToolFixture = fixture;
    class Channel {
      readyState = "connecting";
      onopen: (() => void) | null = null;
      onclose: (() => void) | null = null;
      onmessage: ((event: MessageEvent<string>) => void) | null = null;
      send(raw: string) {
        const frame = JSON.parse(raw) as Record<string, unknown>;
        fixture.sent.push(frame);
        if (frame.kind === "session.setup") queueMicrotask(() => this.receive({ kind: "session.ready" }));
        // Complete the fixture's greeting turn, just as a gateway with no
        // generated audio does, so passive context can flush when idle.
        if (frame.kind === "user.message") queueMicrotask(() => this.receive({ kind: "turn.status", state: "idle" }));
      }
      receive(frame: Record<string, unknown>) {
        this.onmessage?.(new MessageEvent("message", { data: JSON.stringify(frame) }));
      }
      close() {
        this.readyState = "closed";
        queueMicrotask(() => this.onclose?.());
      }
    }
    class Peer extends EventTarget {
      channel = new Channel();
      iceGatheringState = "complete";
      connectionState = "connected";
      iceConnectionState = "connected";
      localDescription: RTCSessionDescriptionInit | null = null;
      constructor() {
        super();
        fixture.peerCount++;
        fixture.closed = false;
        fixture.receive = (frame) => this.channel.receive(frame);
      }
      addTrack(track: MediaStreamTrack) {
        fixture.tracks.push(track);
      }
      getSenders() {
        return [];
      }
      createDataChannel() {
        return this.channel;
      }
      async createOffer() {
        return {
          type: "offer",
          sdp: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=candidate:1 1 udp 2130706431 192.0.2.2 50000 typ host\r\n",
        };
      }
      async setLocalDescription(offer: RTCSessionDescriptionInit) {
        this.localDescription = offer;
      }
      async setRemoteDescription() {
        queueMicrotask(() => {
          this.channel.readyState = "open";
          this.channel.onopen?.();
        });
      }
      close() {
        fixture.closed = true;
        this.connectionState = "closed";
        this.iceConnectionState = "closed";
      }
    }
    Object.defineProperty(window, "RTCPeerConnection", { value: Peer, configurable: true });
    localStorage.setItem("mddb_pwa_dismissed", new Date().toISOString());
  });
  await page.route("**/.well-known/gomode.json", async (route) => {
    const response = await route.fetch();
    const manifest = await response.json();
    manifest.webShell.voiceGateway = { url: "/" };
    await route.fulfill({ response, json: manifest });
  });
  await page.route("**/api/voicegateway/v2/**", async (route) => {
    const url = route.request().url();
    if (url.endsWith("/voice/choices"))
      await route.fulfill({
        json: {
          choices: [{ id: "fixture", label: "Fixture voice", transports: ["webrtc"] }],
          defaults: [{ transport: "webrtc", id: "fixture" }],
        },
      });
    else if (url.endsWith("/voice/rtc/offer"))
      await route.fulfill({ json: { sdp: "fixture-answer", sessionID: "fixture-session" } });
    else if (url.endsWith("/voice/rtc/fixture-session")) await route.fulfill({ json: { status: "closed" } });
    else throw new Error(`Unexpected voice transport request ${url}`);
  });
}

export async function browserFocus(page: Page, nodeId: string, id: string) {
  await page.evaluate((frame) => window.browserToolFixture.receive(frame), {
    kind: "tool.call",
    id,
    name: "focus_node",
    args: { nodeId },
  });
}
export async function browserResult(page: Page, id: string): Promise<Record<string, unknown>> {
  await expect
    .poll(() =>
      page.evaluate(
        (call) => window.browserToolFixture.sent.find((entry) => entry.kind === "tool.result" && entry.id === call),
        id,
      ),
    )
    .toBeDefined();
  return page.evaluate(
    (call) =>
      window.browserToolFixture.sent.find((entry) => entry.kind === "tool.result" && entry.id === call)!
        .result as Record<string, unknown>,
    id,
  );
}
