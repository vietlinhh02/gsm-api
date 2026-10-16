import assert from "node:assert/strict";
import { test } from "node:test";
import { endpoints, points } from "#lib/catalog.mjs";
import {
  initialState,
  changeEnvironment,
  chooseEndpoint,
  bookingFromEstimate,
  parseRequestBody,
  recordResult,
  validatePath,
} from "#lib/playground.mjs";

test("switching directly to a sandbox-only endpoint clears production token and quote", () => {
  const state = {
    ...initialState(),
    token: "production-token",
    orderId: "production-order",
    booking: { fee_id: "production-fee" },
    drafts: { create: "{}" },
  };
  const next = chooseEndpoint(state, "status");
  assert.equal(next.environment, "sandbox");
  assert.equal(next.token, "");
  assert.equal(next.orderId, "");
  assert.equal(next.booking, null);
  assert.deepEqual(next.drafts, {});
  assert.equal(next.path, "/v1/sandbox/ride/update-status");
});

test("changing from sandbox-only status to production selects a supported endpoint", () => {
  const next = changeEnvironment(initialState("status"), "production");
  assert.equal(next.endpointId, "services");
  assert.equal(next.path, "/v1/ride/list-available-services");
});

test("trip booking binds normalized time, route and the selected service add-ons", () => {
  const request = {
    order_time: 0,
    payment_method: "partner_pay",
    points,
    service_ids: [{ id: 2179, addons: [{ id: 10, apply: true }] }],
  };
  const response = {
    data: {
      order_time: "1770350401",
      service_estimates: [
        { fee_unavailable: true },
        { service: { id: 2179 }, estimate_info: { fee_id: "trip-fee" } },
      ],
    },
  };
  const result = bookingFromEstimate("trip", request, response);
  assert.equal(result.order_time, 1770350401);
  assert.equal(result.service_type, "RIDE-TRIP");
  assert.equal(result.fee_id, "trip-fee");
  assert.deepEqual(result.points, points);
  assert.deepEqual(result.addons, [{ id: 10, apply: true }]);
});

test("hourly booking preserves the original zero order_time", () => {
  const request = endpoints.find((endpoint) => endpoint.id === "hour").sample;
  const response = {
    data: {
      service_estimates: [
        {
          service: { id: 1903 },
          estimate_infos: [{ fee_id: "hour-fee" }],
        },
      ],
    },
  };
  const result = bookingFromEstimate("hour", request, response);
  assert.equal(result.order_time, 0);
  assert.equal(result.service_type, "RIDE-HOUR");
  assert.equal(result.service_id, 1903);
});

test("unavailable and malformed estimates never become a booking", () => {
  for (const response of [{}, { data: { service_estimates: [{ fee_unavailable: true }] } }]) {
    assert.equal(bookingFromEstimate("trip", {}, response), null);
  }
  assert.equal(
    bookingFromEstimate(
      "hour",
      {},
      {
        data: {
          service_estimates: [{ service: { id: 1903 }, estimate_infos: [{ fee_id: "id" }] }],
        },
      },
    ),
    null,
  );
});

test("new order ID fills detail, cancel and sandbox update drafts", () => {
  const result = { status: 200, data: { data: { id: "created-order" } } };
  const state = recordResult(initialState("create"), result, {});
  assert.equal(
    chooseEndpoint(state, "details").path,
    "/v1/ride/orders/created-order?extra_components=driver,path",
  );
  assert.equal(JSON.parse(chooseEndpoint(state, "cancel").body).order_id, "created-order");
});

test("invalid JSON and missing IDs fail before sending", () => {
  for (const input of ["{", "null", "[]", "1"]) {
    assert.throws(() => parseRequestBody("POST", input));
  }
  assert.deepEqual(parseRequestBody("POST", "{}"), {});
  assert.equal(parseRequestBody("GET", "{"), undefined);
  assert.throws(() => validatePath("/v1/ride/orders/{id}"), /Điền request URL/);
  assert.throws(() => validatePath("https://other.example/v1/ride"), /Điền request URL/);
  assert.doesNotThrow(() => validatePath("/v1/ride/list-user-orders"));
});
