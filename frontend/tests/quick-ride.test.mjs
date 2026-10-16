import assert from "node:assert/strict";
import { test } from "node:test";
import {
  presetPoints,
  selectedPoint,
  swapPoints,
  tripRequest,
  selectedBooking,
  simulationActions,
  quoteGeometry,
} from "#lib/quick-ride.mjs";

test("preset, swap and map selections preserve complete passenger points", () => {
  const points = presetPoints("center");
  assert.equal(points[0].type, "PICK UP");
  assert.equal(points[1].type, "DROP OFF");
  assert.deepEqual(swapPoints(swapPoints(points)), points);
  const selected = selectedPoint(
    { ...points[0], place_id: "old" },
    {
      latitude: 21.03,
      longitude: 105.82,
    },
  );
  assert.equal(selected.name, points[0].name);
  assert.equal(selected.type, "PICK UP");
  assert.equal(selected.address, "21.030000, 105.820000");
  assert.equal(selected.place_id, undefined);
  assert.equal(points[0].latitude, 21.0287);
  const found = selectedPoint(selected, {
    latitude: 21.04,
    longitude: 105.84,
    label: "x".repeat(700),
    place_id: "new",
  });
  assert.equal(found.address.length, 512);
  assert.equal(found.place_id, "new");
});

test("comparison request binds contacts and payment to both service IDs", () => {
  const form = {
    points: presetPoints("center"),
    name: " Test ",
    phone: "+84900000000",
    payment: "partner_pay",
  };
  const originalName = form.points[0].name;
  const request = tripRequest(form, [{ id: 2179 }, { id: 2180 }]);
  assert.deepEqual(request.service_ids, [{ id: 2179 }, { id: 2180 }]);
  assert.equal(request.points[0].name, "Test");
  assert.equal(request.payment_method, "partner_pay");
  assert.equal(form.points[0].name, originalName);
  for (const name of ["", " ", "a".repeat(257)]) {
    assert.throws(() => tripRequest({ ...form, name }, [{ id: 2179 }]), /tên khách/);
  }
  for (const phone of ["", "0900000000", "+1", "+84900000000x"]) {
    assert.throws(() => tripRequest({ ...form, phone }, [{ id: 2179 }]), /điện thoại/);
  }
  assert.throws(() => tripRequest(form, []), /dịch vụ/);
});

function quoteResponse() {
  return {
    data: {
      order_time: "1791993600",
      service_estimates: [
        {
          service: { id: 2179 },
          fee_unavailable: false,
          estimate_info: { fee_id: "car", expires_at: Date.now() / 1000 + 600 },
        },
        {
          service: { id: 2180 },
          fee_unavailable: false,
          estimate_info: { fee_id: "bike", expires_at: Date.now() / 1000 + 600 },
        },
      ],
    },
  };
}

test("booking chooses the exact bike quote and rejects expired or unavailable quotes", () => {
  const request = { payment_method: "cash", points: presetPoints("center") };
  const response = quoteResponse();
  const booking = selectedBooking(request, response, 2180);
  assert.equal(booking.fee_id, "bike");
  assert.equal(booking.service_id, 2180);
  assert.deepEqual(booking.points, request.points);
  response.data.service_estimates[1].estimate_info.expires_at = 1;
  assert.throws(() => selectedBooking(request, response, 2180), /hết hạn/);
  for (const invalid of [null, {}, { data: { service_estimates: [] } }]) {
    assert.throws(() => selectedBooking(request, invalid, 2179), /báo giá hợp lệ/);
  }
  response.data.service_estimates[0].fee_unavailable = true;
  assert.throws(() => selectedBooking(request, response, 2179), /báo giá hợp lệ/);
  response.data.service_estimates[0].fee_unavailable = false;
  response.data.order_time = "bad";
  assert.throws(() => selectedBooking(request, response, 2179), /báo giá hợp lệ/);
});

test("quick controls guide the sandbox lifecycle and disable terminal actions", () => {
  assert.equal(simulationActions("FINDING").assign, true);
  assert.equal(simulationActions("ASSIGNED").start, true);
  assert.equal(simulationActions("IN PROCESS").complete, true);
  assert.equal(simulationActions("IN PROCESS").cancel, false);
  for (const status of ["COMPLETED", "CANCELLED"]) {
    assert.deepEqual(simulationActions(status), {
      assign: false,
      start: false,
      complete: false,
      cancel: false,
    });
  }
  assert.equal(quoteGeometry(null), undefined);
});
