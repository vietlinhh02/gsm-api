import assert from "node:assert/strict";
import { test } from "node:test";
import { forwardRequest } from "#lib/proxy.mjs";

test("generated receiver links accept public POSTs and preserve webhook headers", async () => {
  const path = ["webhook-inbox", "a".repeat(64)];
  const response = await forwardRequest(
    new Request("http://docs/api/backend/receiver", {
      method: "POST",
      body: '{"event_id":"event"}',
      headers: {
        "content-type": "application/json",
        "x-vsf-signature": "signature",
        "x-environment": "sandbox",
        "x-debug-source": "custom-sender",
      },
    }),
    path,
    {
      fetchImpl: async (url, options) => {
        assert.equal(url, `http://localhost:8081/${path.join("/")}`);
        assert.equal(options.headers.get("x-vsf-signature"), "signature");
        assert.equal(options.headers.get("x-environment"), "sandbox");
        assert.equal(options.headers.get("x-debug-source"), "custom-sender");
        assert.equal(options.headers.has("authorization"), false);
        return Response.json({ message: "received" });
      },
    },
  );
  assert.equal(response.status, 200);
});

test("inbox management routes preserve authentication and reject wrong methods", async () => {
  for (const [path, method] of [
    [["v1", "sandbox", "webhook-inboxes"], "POST"],
    [["v1", "sandbox", "webhook-inboxes", "inbox-id"], "GET"],
    [["v1", "sandbox", "webhook-inboxes", "inbox-id", "receipts", "42"], "GET"],
  ]) {
    const response = await forwardRequest(
      new Request("http://docs/api/backend/inbox", {
        method,
        headers: { authorization: "Bearer test-token" },
      }),
      path,
      {
        fetchImpl: async (_url, options) => {
          assert.equal(options.headers.get("authorization"), "Bearer test-token");
          return Response.json({ data: {} });
        },
      },
    );
    assert.equal(response.status, 200);
  }
  const invalid = await forwardRequest(new Request("http://docs/api/backend/inbox"), [
    "webhook-inbox",
    "a".repeat(64),
  ]);
  assert.equal(invalid.status, 405);
});

test("proxy preserves token, query, status and JSON response without caching", async () => {
  const request = new Request(
    "http://docs/api/backend/v1/sandbox/ride/orders/id?extra_components=path",
    {
      headers: { authorization: "Bearer test-token" },
    },
  );
  const response = await forwardRequest(request, ["v1", "sandbox", "ride", "orders", "id"], {
    origin: "http://api:8081/",
    fetchImpl: async (url, options) => {
      assert.equal(url, "http://api:8081/v1/sandbox/ride/orders/id?extra_components=path");
      assert.equal(options.headers.get("authorization"), "Bearer test-token");
      assert.equal(options.cache, "no-store");
      assert.equal(options.body, undefined);
      return Response.json({ code: 401, message: "token expired" }, { status: 401 });
    },
  });
  assert.equal(response.status, 401);
  assert.equal(response.headers.get("cache-control"), "no-store");
  assert.deepEqual(await response.json(), { code: 401, message: "token expired" });
});

test("form token exchange is forwarded unchanged", async () => {
  const body = "client_id=local-client&grant_type=client_credentials";
  const request = new Request("http://docs/api/backend/v1/sandbox/token", {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded" },
    body,
  });
  const response = await forwardRequest(request, ["v1", "sandbox", "token"], {
    fetchImpl: async (_url, options) => {
      assert.equal(new TextDecoder().decode(options.body), body);
      assert.equal(options.headers.get("content-type"), "application/x-www-form-urlencoded");
      return Response.json({ token_type: "Bearer" });
    },
  });
  assert.equal(response.status, 200);
});

test("network failure and timeout become actionable JSON", async () => {
  for (const [name, status] of [
    ["TypeError", 502],
    ["TimeoutError", 504],
  ]) {
    const error = new Error("failure");
    error.name = name;
    const response = await forwardRequest(
      new Request("http://docs/api/backend/healthz"),
      ["healthz"],
      {
        fetchImpl: async () => {
          throw error;
        },
      },
    );
    assert.equal(response.status, status);
    assert.equal((await response.json()).code, status);
  }
});

test("unknown paths, traversal and wrong HTTP methods never contact the backend", async () => {
  const scenarios = [
    [["v1", "ride", "update-status"], "POST", 404],
    [["v1", "public", "token"], "POST", 404],
    [["v1", "ride", "list-user-orders"], "GET", 404],
    [["admin", "delete"], "GET", 404],
    [["v1", "..", "token"], "POST", 400],
    [["v1", "ride/orders/id"], "GET", 400],
    [["v1", "sandbox", "ride", "list-user-orders"], "POST", 405],
  ];
  for (const [path, method, status] of scenarios) {
    const response = await forwardRequest(
      new Request("http://docs/api/backend/test", { method }),
      path,
      {
        fetchImpl: () => {
          throw new Error("unexpected backend call");
        },
      },
    );
    assert.equal(response.status, status);
  }
});

test("SSE proxy streams immediately, survives header timeout and propagates cancellation", async () => {
  const controller = new AbortController();
  const request = new Request("http://docs/api/backend/v1/sandbox/ride/orders/trip/events", {
    headers: { authorization: "Bearer test-token", accept: "text/event-stream" },
    signal: controller.signal,
  });
  let upstreamSignal;
  let writer;
  let cancelled = false;
  const response = await forwardRequest(
    request,
    ["v1", "sandbox", "ride", "orders", "trip", "events"],
    {
      timeoutMs: 10,
      fetchImpl: async (_url, options) => {
        upstreamSignal = options.signal;
        assert.equal(options.headers.get("accept"), "text/event-stream");
        return new Response(
          new ReadableStream({
            start(stream) {
              writer = stream;
            },
            cancel() {
              cancelled = true;
            },
          }),
          { headers: { "content-type": "text/event-stream" } },
        );
      },
    },
  );
  assert.equal(response.headers.get("x-accel-buffering"), "no");
  assert.equal(response.headers.get("content-type"), "text/event-stream");
  await new Promise((resolve) => setTimeout(resolve, 25));
  assert.equal(upstreamSignal.aborted, false);
  const reader = response.body.getReader();
  writer.enqueue(new TextEncoder().encode(": heartbeat\n\n"));
  assert.equal(new TextDecoder().decode((await reader.read()).value), ": heartbeat\n\n");
  controller.abort();
  assert.equal(upstreamSignal.aborted, true);
  await reader.cancel();
  assert.equal(cancelled, true);
});

test("SSE proxy times out if upstream never returns response headers", async () => {
  const response = await forwardRequest(
    new Request("http://docs/api/backend/v1/sandbox/ride/orders/trip/events"),
    ["v1", "sandbox", "ride", "orders", "trip", "events"],
    {
      timeoutMs: 10,
      fetchImpl: (_url, { signal }) =>
        new Promise((_resolve, reject) => {
          signal.addEventListener("abort", () => reject(signal.reason), { once: true });
        }),
    },
  );
  assert.equal(response.status, 504);
});

test("empty credentials use server configuration without replacing explicit credentials", async () => {
  for (const explicit of [false, true]) {
    const body = new URLSearchParams({
      client_id: explicit ? "custom-id" : "",
      client_secret: explicit ? "custom-secret" : "",
      grant_type: "client_credentials",
      scope: "sandbox.ride.trips",
    });
    const response = await forwardRequest(
      new Request("https://docs/api/backend/v1/sandbox/token", {
        method: "POST",
        headers: { "content-type": "application/x-www-form-urlencoded" },
        body,
      }),
      ["v1", "sandbox", "token"],
      {
        clientId: "server-id",
        clientSecret: "server-secret",
        fetchImpl: async (_url, options) => {
          const form = new URLSearchParams(new TextDecoder().decode(options.body));
          assert.equal(form.get("client_id"), explicit ? "custom-id" : "server-id");
          assert.equal(form.get("client_secret"), explicit ? "custom-secret" : "server-secret");
          return Response.json({ access_token: "test-token" });
        },
      },
    );
    assert.equal(response.status, 200);
  }
});
