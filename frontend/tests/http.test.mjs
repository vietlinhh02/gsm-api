import assert from "node:assert/strict";
import { test } from "node:test";
import { apiErrorMessage, requestApi } from "#lib/http.mjs";

test("booking errors explain how to recover", () => {
  const cases = [
    ["fee_id is invalid or expired", /so sánh giá lại/],
    ["fee_id already booked with different fields; obtain a new estimate", /báo giá mới/],
    ["no available vehicle within 10 km; retry later", /xe rảnh/],
    ["payment_failed requires payment_method partner_pay", /Đối tác thanh toán/],
  ];
  for (const [message, expected] of cases) {
    assert.match(apiErrorMessage({ status: 400, data: { message } }), expected);
  }
});

test("network, timeout and malformed JSON errors remain actionable", async (t) => {
  const fetch = t.mock.method(globalThis, "fetch");
  fetch.mock.mockImplementation(async () => {
    throw new TypeError("Failed to fetch");
  });
  await assert.rejects(requestApi({ path: "/healthz" }), /Kiểm tra kết nối/);
  fetch.mock.mockImplementation(async () => {
    throw new DOMException("timeout", "TimeoutError");
  });
  await assert.rejects(requestApi({ path: "/healthz" }), /không tạo chuyến trùng/);
  fetch.mock.mockImplementation(
    async () => new Response("{", { headers: { "content-type": "application/json" } }),
  );
  await assert.rejects(requestApi({ path: "/healthz" }), /dữ liệu không hợp lệ/);
});

test("requests preserve backend status, bearer token and JSON error", async (t) => {
  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url, "/api/backend/v1/sandbox/ride/create-order");
    assert.equal(options.headers.Authorization, "Bearer token");
    assert.equal(options.method, "POST");
    return Response.json({ message: "already booked" }, { status: 409 });
  });
  const result = await requestApi({
    path: "/v1/sandbox/ride/create-order",
    method: "POST",
    token: "Bearer token",
    body: "{}",
  });
  assert.equal(result.status, 409);
  assert.equal(result.data.message, "already booked");
});
