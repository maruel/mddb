// Tests Vite API proxy Origin normalization through real HTTP forwarding.
import assert from "node:assert/strict";
import { once } from "node:events";
import { createServer as createHTTPServer } from "node:http";
import { test } from "node:test";
import { createServer } from "vite";

test("development API proxy normalizes only its own Origin", async (t) => {
  const backend = createHTTPServer((req, res) => {
    res.setHeader("Content-Type", "application/json");
    res.end(JSON.stringify({ origin: req.headers.origin, host: req.headers.host }));
  });
  backend.listen(0, "127.0.0.1");
  await once(backend, "listening");
  t.after(() => new Promise((resolve, reject) => backend.close((err) => (err ? reject(err) : resolve()))));
  const target = `http://127.0.0.1:${backend.address().port}`;
  const vite = await createServer({
    server: { host: "127.0.0.1", port: 0, proxy: { "/api": { target } } },
  });
  await vite.listen();
  t.after(() => vite.close());
  const origin = `http://127.0.0.1:${vite.httpServer.address().port}`;
  for (const supplied of [undefined, origin, "https://foreign.example", "null", `${origin}/bad`]) {
    const response = await fetch(`${origin}/api/proxy-origin`, {
      headers: supplied === undefined ? {} : { Origin: supplied },
    });
    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), {
      ...(supplied === undefined ? {} : { origin: supplied === origin ? target : supplied }),
      host: new URL(target).host,
    });
  }
});
