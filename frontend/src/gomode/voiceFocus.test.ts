// Tests host focus CAS claims and version-bound lease cleanup after shared-owner cancellation.

import { it } from "node:test";
import { expect } from "@tests/expect";
import type { UpdateVoiceFocusRequest, UpdateVoiceFocusResponse } from "@sdk/types.gen";
import { createVoiceFocusPublisher } from "./voiceFocus";

const runRelease = (request: (signal: AbortSignal) => Promise<unknown>) => {
  void request(new AbortController().signal);
};

it("claims a server lease before publishing a selection and preserves null selection semantics", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  const publisher = createVoiceFocusPublisher(
    async (request) => {
      requests.push(request);
      return { version: 7, applied: request.version === 7 };
    },
    "conversation",
    true,
    runRelease,
  );
  await publisher.publish("node", () => true, new AbortController().signal);
  expect(requests).toEqual([
    { "openai/session": "conversation", nodeId: "node", version: 0, browserSelection: true },
    { "openai/session": "conversation", nodeId: "node", version: 7, browserSelection: true },
  ]);
  await publisher.publish(null, () => true, new AbortController().signal);
  expect(requests.at(-1)).toEqual({
    "openai/session": "conversation",
    nodeId: "0",
    version: 7,
    browserSelection: true,
  });
});

it("releases only a canceled response's committed version without replacing a newer CAS version", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  let release!: (response: UpdateVoiceFocusResponse) => void;
  const publisher = createVoiceFocusPublisher(
    async (request) => {
      requests.push(request);
      if (requests.length === 1)
        return new Promise((resolve) => {
          release = resolve;
        });
      return { version: 12, applied: true };
    },
    "conversation",
    true,
    runRelease,
  );
  const old = new AbortController();
  const pending = publisher.publish("old", () => true, old.signal);
  old.abort();
  await publisher.publish("new", () => true, new AbortController().signal);
  release({ version: 11, applied: true });
  await pending;
  expect(requests.at(-1)).toEqual({ "openai/session": "conversation", nodeId: "0", version: 11, release: true });
  await publisher.publish("newer", () => true, new AbortController().signal);
  expect(requests.at(-1)?.version).toBe(12);
  publisher.dispose();
});

it("clears the last committed lease once without retrying another owner's conflict", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  const publisher = createVoiceFocusPublisher(
    async (request) => {
      requests.push(request);
      return request.release ? { version: 99, applied: false } : { version: 12, applied: true };
    },
    "conversation",
    true,
    runRelease,
  );
  await publisher.publish("old", () => true, new AbortController().signal);
  publisher.dispose();
  await Promise.resolve();
  expect(requests).toEqual([
    { "openai/session": "conversation", nodeId: "old", version: 0, browserSelection: true },
    { "openai/session": "conversation", nodeId: "0", version: 12, release: true },
  ]);
});

it("bounds CAS conflicts and never sends an already canceled publication", async () => {
  let count = 0;
  const publisher = createVoiceFocusPublisher(
    async () => ({ version: ++count, applied: false }),
    "conversation",
    true,
    runRelease,
  );
  await expect(publisher.publish("node", () => true, new AbortController().signal)).rejects.toThrow(
    "changed concurrently",
  );
  expect(count).toBe(4);
  const canceled = new AbortController();
  canceled.abort();
  await publisher.publish("canceled", () => true, canceled.signal);
  await publisher.publish("retired", () => false, new AbortController().signal);
  expect(count).toBe(4);
});

it("never adopts another owner's conflict version when its publication retires", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  let release!: (response: UpdateVoiceFocusResponse) => void;
  const publisher = createVoiceFocusPublisher(
    async (request) => {
      requests.push(request);
      if (requests.length === 2)
        return new Promise((resolve) => {
          release = resolve;
        });
      return { version: 1, applied: true };
    },
    "conversation",
    true,
    runRelease,
  );
  await publisher.publish("first", () => true, new AbortController().signal);
  let current = true;
  const pending = publisher.publish("pending", () => current, new AbortController().signal);
  current = false;
  publisher.dispose();
  release({ version: 99, applied: false });
  await pending;
  expect(requests.at(-1)).toEqual({ "openai/session": "conversation", nodeId: "0", version: 1, release: true });
});

it("does not release an initial CAS claim that never committed a selection", async () => {
  const requests: UpdateVoiceFocusRequest[] = [];
  let release!: (response: UpdateVoiceFocusResponse) => void;
  const publisher = createVoiceFocusPublisher(
    async (request) => {
      requests.push(request);
      return new Promise((resolve) => {
        release = resolve;
      });
    },
    "conversation",
    true,
    runRelease,
  );
  const canceled = new AbortController();
  const pending = publisher.publish("pending", () => true, canceled.signal);
  canceled.abort();
  publisher.dispose();
  release({ version: 1, applied: false });
  await pending;
  expect(requests).toHaveLength(1);
});
