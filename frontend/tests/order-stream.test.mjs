import assert from "node:assert/strict";
import { test } from "node:test";
import { followOrder, readOrderEvents } from "#lib/order-stream.mjs";

function eventFrame(status) {
  return `event: order\ndata: ${JSON.stringify({ data: { id: "trip", status, name: "Tài xế" } })}\n\n`;
}

function eventResponse(text) {
  return new Response(text, { headers: { "content-type": "text/event-stream" } });
}

test("SSE parses fragmented UTF-8 and CRLF frames, ignores heartbeats and closes at terminal", async () => {
  const encoded = new TextEncoder().encode(
    (": heartbeat\n\n" + eventFrame("ASSIGNED") + eventFrame("COMPLETED")).replaceAll("\n", "\r\n"),
  );
  let cancelled = false;
  const body = new ReadableStream({
    start(controller) {
      for (const byte of encoded) controller.enqueue(new Uint8Array([byte]));
    },
    cancel() {
      cancelled = true;
    },
  });
  const orders = [];
  assert.equal(await readOrderEvents(new Response(body), (order) => orders.push(order)), true);
  assert.deepEqual(
    orders.map((order) => order.status),
    ["ASSIGNED", "COMPLETED"],
  );
  assert.equal(orders[0].name, "Tài xế");
  assert.equal(cancelled, true);
});

test("SSE reports malformed data and identifies a disconnected active stream", async () => {
  for (const text of ["event: order\ndata: invalid\n\n", "event: order\ndata: {}\n\n"]) {
    await assert.rejects(readOrderEvents(eventResponse(text), () => {}));
  }
  assert.equal(await readOrderEvents(eventResponse(eventFrame("ASSIGNED")), () => {}), false);
});

test("SSE reconnects after disconnect, preserving bearer auth without token in URL", async (t) => {
  const controller = new AbortController();
  let calls = 0;
  t.mock.method(globalThis, "fetch", async (url, options) => {
    calls += 1;
    assert.equal(url, "/api/backend/v1/sandbox/ride/orders/trip/events");
    assert.equal(options.headers.Authorization, "Bearer secret-token");
    assert.equal(options.headers.Accept, "text/event-stream");
    return eventResponse(eventFrame(calls === 1 ? "ASSIGNED" : "COMPLETED"));
  });
  const orders = [];
  const errors = [];
  await followOrder({
    id: "trip",
    token: "secret-token",
    signal: controller.signal,
    onOrder: (order) => orders.push(order.status),
    onError: (error) => errors.push(error),
  });
  assert.equal(calls, 2);
  assert.deepEqual(orders, ["ASSIGNED", "COMPLETED"]);
  assert.equal(errors.length, 1);
});

test("SSE stops on authorization/not-found errors and aborts reconnect on unmount", async (t) => {
  for (const status of [401, 403, 404]) {
    let calls = 0;
    const mock = t.mock.method(globalThis, "fetch", async () => {
      calls += 1;
      return new Response(null, { status });
    });
    const errors = [];
    await followOrder({
      id: "trip",
      token: "expired",
      signal: new AbortController().signal,
      onOrder: () => assert.fail("unexpected event"),
      onError: (error) => errors.push(error),
    });
    assert.equal(calls, 1);
    assert.match(errors[0], new RegExp(String(status)));
    mock.mock.restore();
  }
  const controller = new AbortController();
  let calls = 0;
  t.mock.method(globalThis, "fetch", async () => {
    calls += 1;
    throw new TypeError("network unavailable");
  });
  await followOrder({
    id: "trip",
    token: "token",
    signal: controller.signal,
    onOrder: () => assert.fail("unexpected event"),
    onError: () => controller.abort(),
  });
  assert.equal(calls, 1);
});
