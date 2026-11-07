// Tests focus lease conflicts, rapid selection changes, and retired publisher clearing.

import { it } from "node:test";
import { expect } from "@tests/expect";
import type { UpdateVoiceFocusRequest, UpdateVoiceFocusResponse } from "@sdk/types.gen";
import { createVoiceFocusPublisher } from "./voiceFocus";

it("claims a server lease before publishing a selection", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  const publisher = createVoiceFocusPublisher(async (request) => {
    requests.push(request);
    return { version: 7, applied: request.version === 7 };
  }, true);
  await publisher.publish("conversation", "node", () => true);
  expect(requests).toEqual([
    { "openai/session": "conversation", nodeId: "node", version: 0, browserSelection: true },
    { "openai/session": "conversation", nodeId: "node", version: 7, browserSelection: true },
  ]);
  await publisher.publish("conversation", null, () => true);
  expect(requests.at(-1)).toEqual({
    "openai/session": "conversation",
    nodeId: "0",
    version: 7,
    browserSelection: true,
  });
});

it("serializes rapid selections and does not retry the retired selection", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  let release!: (response: UpdateVoiceFocusResponse) => void;
  const publisher = createVoiceFocusPublisher(async (request) => {
    requests.push(request);
    if (requests.length === 1)
      return new Promise((resolve) => {
        release = resolve;
      });
    return { version: 9, applied: true };
  }, true);
  const first = publisher.publish("conversation", "old", () => true);
  await Promise.resolve();
  const second = publisher.publish("conversation", "new", () => true);
  release({ version: 8, applied: false });
  await Promise.all([first, second]);
  expect(requests.map((request) => request.nodeId)).toEqual(["old", "new"]);
  expect(requests[1]!.version).toBe(8);
});

it("clears a retired owner with its last lease without retrying conflicts", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  const publisher = createVoiceFocusPublisher(async (request) => {
    requests.push(request);
    return { version: 12, applied: true };
  }, true);
  await publisher.publish("conversation", "old", () => true);
  publisher.dispose();
  await Promise.resolve();
  await Promise.resolve();
  expect(requests.at(-1)).toEqual({ "openai/session": "conversation", nodeId: "0", version: 12, release: true });
  await publisher.publish("conversation", "retired", () => true);
  expect(requests).toHaveLength(2);
});

it("bounds conflicts and suppresses canceled publications", async () => {
  let count = 0;
  const publisher = createVoiceFocusPublisher(async () => ({ version: ++count, applied: false }), true);
  await expect(publisher.publish("conversation", "node", () => true)).rejects.toThrow("changed concurrently");
  expect(count).toBe(4);
  await publisher.publish("conversation", "canceled", () => false);
  expect(count).toBe(4);
});

it("never adopts another owner's conflict version for its retired clear", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  let release!: (value: UpdateVoiceFocusResponse) => void;
  const publisher = createVoiceFocusPublisher(async (request) => {
    requests.push(request);
    if (requests.length === 2)
      return new Promise((resolve) => {
        release = resolve;
      });
    return { version: 1, applied: true };
  }, true);
  await publisher.publish("conversation", "first", () => true);
  const pending = publisher.publish("conversation", "pending", () => true);
  await Promise.resolve();
  publisher.dispose();
  release({ version: 99, applied: false });
  await pending;
  await Promise.resolve();
  expect(requests.at(-1)).toEqual({ "openai/session": "conversation", nodeId: "0", version: 1, release: true });
});

it("does not publish a retired clear for an initial claim that never selected a node", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  let release!: (value: UpdateVoiceFocusResponse) => void;
  const publisher = createVoiceFocusPublisher(async (request) => {
    requests.push(request);
    return new Promise((resolve) => {
      release = resolve;
    });
  }, true);
  const pending = publisher.publish("conversation", "pending", () => true);
  await Promise.resolve();
  publisher.dispose();
  release({ version: 1, applied: false });
  await pending;
  await Promise.resolve();
  expect(requests).toHaveLength(1);
});
