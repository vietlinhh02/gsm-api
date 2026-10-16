import { expect, test } from "@playwright/test";

test("sandbox receives SSE status changes without polling", async ({ page }) => {
  let reads = 0;
  let streams = 0;
  let manualUpdates = 0;
  await page.route("**/api/backend/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    expect(path).toMatch(/^\/api\/backend\/(healthz$|v1\/sandbox\/)/);
    let data;
    if (path.endsWith("/token")) {
      data = { access_token: "sandbox-test-token" };
    } else if (path.endsWith("/list-available-services")) {
      data = { data: [{ service: { id: 2179, travel_mode: "Car" } }] };
    } else if (path.endsWith("/estimate/order")) {
      data = {
        data: {
          order_time: String(Math.floor(Date.now() / 1000)),
          service_estimates: [
            {
              service: { id: 2179, travel_mode: "Car" },
              estimate_info: {
                fee_id: "test-fee",
                expires_at: Date.now() / 1000 + 600,
                estimate: { total_pay: 50000 },
                pricing: { items: [] },
                trip: { distance_estimate: 4, duration_estimate: 600 },
              },
            },
          ],
        },
      };
    } else if (path.endsWith("/create-order")) {
      data = { data: { id: "automatic-trip", status: "FINDING" } };
    } else if (path.endsWith("/events")) {
      streams += 1;
      expect(route.request().headers().authorization).toBe("Bearer sandbox-test-token");
      const body = ["ASSIGNED", "IN PROCESS", "COMPLETED"]
        .map(
          (status) =>
            `event: order\ndata: ${JSON.stringify({
              data: {
                id: "automatic-trip",
                status,
                driver: { id: "local-driver" },
                vehicle: { model: "VF5" },
              },
            })}\n\n`,
        )
        .join("");
      await route.fulfill({ contentType: "text/event-stream", body });
      return;
    } else if (path.includes("/orders/")) {
      reads += 1;
      data = {
        data: {
          id: "automatic-trip",
          status: "FINDING",
          driver: {},
          vehicle: { model: "VF5" },
        },
      };
    } else {
      if (path.endsWith("/update-status")) manualUpdates += 1;
      data = { data: {} };
    }
    await route.fulfill({ json: data });
  });
  await page.goto("/playground");
  await expect(page.getByRole("combobox", { name: "Môi trường", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Dùng Sandbox", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "So sánh giá", exact: true }).click();
  await page.getByRole("button", { name: "Đặt xe", exact: true }).click();
  await expect(page.getByText("Mã chuyến: automatic-trip", { exact: true })).toBeVisible();
  await expect.poll(() => streams).toBe(1);
  await expect(page.getByRole("heading", { name: "Đã hoàn thành", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Gán tài xế", exact: true })).toBeDisabled();
  await page.waitForTimeout(2500);
  expect(reads).toBe(0);
  expect(streams).toBe(1);
  expect(manualUpdates).toBe(0);
  await expect(page.getByRole("button", { name: "Gán tài xế", exact: true })).toBeDisabled();
});

test("real backend pushes sandbox assignment through the Next.js SSE proxy", async ({ page }) => {
  const prefix = "/api/backend/v1/sandbox";
  const auth = await page.request.post(`${prefix}/token`, {
    form: {
      client_id: process.env.CLIENT_ID ?? "local-client",
      client_secret: process.env.CLIENT_SECRET ?? "local-secret",
      grant_type: "client_credentials",
      scope: "sandbox.ride.trips",
    },
  });
  expect(auth.status()).toBe(200);
  const { access_token: token } = await auth.json();
  const headers = { Authorization: `Bearer ${token}` };
  const points = [
    {
      type: "PICK UP",
      latitude: 21.0287,
      longitude: 105.8523,
      address: "SSE integration test",
      short_address: "SSE test",
      name: "SSE Test",
      phone_number: "+84900000000",
    },
  ];
  const estimate = await page.request.post(`${prefix}/ride/estimate/hour-order`, {
    headers,
    data: {
      data: [
        { service_id: 1903, hour: 1, order_time: 0, discount: 0, payment_method: "cash", points },
      ],
    },
  });
  expect(estimate.status()).toBe(200);
  const quote = (await estimate.json()).data.service_estimates[0].estimate_infos[0];
  const created = await page.request.post(`${prefix}/ride/create-order`, {
    headers,
    data: {
      service_id: 1903,
      service_type: "RIDE-HOUR",
      order_time: 0,
      fee_id: quote.fee_id,
      payment_method: "cash",
      points,
    },
  });
  expect(created.status()).toBe(200);
  const { id } = (await created.json()).data;
  try {
    await page.goto("/playground");
    const statuses = await page.evaluate(
      async ({ url, token }) => {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), 12000);
        const response = await fetch(url, {
          headers: { Authorization: `Bearer ${token}`, Accept: "text/event-stream" },
          signal: controller.signal,
        });
        if (!response.ok) throw new Error(`SSE returned ${response.status}`);
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let pending = "";
        const statuses = [];
        try {
          while (statuses.length < 2) {
            const { value, done } = await reader.read();
            if (done) break;
            pending += decoder.decode(value, { stream: true });
            const frames = pending.split("\n\n");
            pending = frames.pop();
            for (const frame of frames) {
              if (frame.startsWith("event: order\n")) {
                statuses.push(JSON.parse(frame.split("\ndata: ")[1]).data.status);
              }
            }
          }
          return statuses;
        } finally {
          clearTimeout(timeout);
          controller.abort();
        }
      },
      { url: `${prefix}/ride/orders/${id}/events`, token },
    );
    expect(statuses).toEqual(["FINDING", "ASSIGNED"]);
  } finally {
    const cancelled = await page.request.post(`${prefix}/ride/cancel-order`, {
      headers,
      data: { order_id: id, cancel_code: 9000, cancel_message: "SSE test finished" },
    });
    expect(cancelled.status()).toBe(200);
  }
});
