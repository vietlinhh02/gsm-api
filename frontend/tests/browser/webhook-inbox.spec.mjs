import { expect, test } from "@playwright/test";

async function openInbox(page) {
  await page.goto("/playground");
  await page.getByRole("tab", { name: "Webhook", exact: true }).click();
  const tokenResponse = page.waitForResponse(
    (response) => response.url().endsWith("/v1/sandbox/token") && response.status() === 200,
  );
  const createdResponse = page.waitForResponse(
    (response) => response.url().endsWith("/webhook-inboxes") && response.status() === 201,
  );
  await page.getByRole("button", { name: "Tạo link webhook", exact: true }).click();
  const { data: inbox } = await (await createdResponse).json();
  const { access_token: token } = await (await tokenResponse).json();
  const url = await page.getByRole("textbox", { name: "Link nhận webhook" }).inputValue();
  return { inbox, token, url };
}

test("generated URL receives callbacks and restores the inbox after reload", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const { url } = await openInbox(page);
  await page.getByRole("button", { name: "Sao chép link", exact: true }).click();
  await expect(page.getByText("Đã sao chép link webhook.", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(url);
  const received = await page.request.post(url, {
    data: { event_id: "external-callback", message: "Callback từ hệ thống ngoài" },
    headers: { "X-Debug-Source": "browser-test", Authorization: "Bearer private-test" },
  });
  expect(received.status()).toBe(200);
  await expect(page.getByRole("button", { name: /^Xem #/ })).toHaveCount(1, { timeout: 10000 });
  await page.getByRole("button", { name: /^Xem #/ }).click();
  await expect(page.getByRole("heading", { name: /^Request #/ })).toBeVisible();
  await expect(page.getByText("browser-test", { exact: false })).toBeVisible();
  await expect(page.getByText("private-test", { exact: false })).toHaveCount(0);
  await page.reload();
  await page.getByRole("tab", { name: "Webhook", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "Link nhận webhook" })).toHaveValue(url);
  await expect(page.getByRole("button", { name: /^Xem #/ })).toHaveCount(1, { timeout: 10000 });
  await page.getByRole("button", { name: "Gửi event thử", exact: true }).click();
  await expect(page.getByRole("button", { name: /^Xem #/ })).toHaveCount(2);
});

async function paymentFailureTrip(page, inbox, token) {
  const prefix = "/api/backend/v1/sandbox/ride";
  const headers = { Authorization: `Bearer ${token}` };
  const points = [
    {
      type: "PICK UP",
      latitude: 21.0287,
      longitude: 105.8524,
      name: "Webhook QA",
      phone_number: "+84900000000",
      address: "Webhook test",
      short_address: "Webhook test",
    },
  ];
  const estimate = await page.request.post(`${prefix}/estimate/hour-order`, {
    headers,
    data: {
      data: [
        {
          service_id: 1903,
          hour: 1,
          order_time: 0,
          discount: 0,
          payment_method: "partner_pay",
          points,
        },
      ],
    },
  });
  expect(estimate.status()).toBe(200);
  const quote = (await estimate.json()).data.service_estimates[0].estimate_infos[0];
  const created = await page.request.post(`${prefix}/create-order`, {
    headers,
    data: {
      service_id: 1903,
      service_type: "RIDE-HOUR",
      fee_id: quote.fee_id,
      order_time: 0,
      payment_method: "partner_pay",
      points,
      sandbox_scenario: "payment_failed",
      webhook_inbox_id: inbox.id,
    },
  });
  expect(created.status()).toBe(200);
}

test("sandbox worker delivers failure snapshots to the generated inbox", async ({ page }) => {
  const { inbox, token } = await openInbox(page);
  await paymentFailureTrip(page, inbox, token);
  await expect(page.getByRole("button", { name: /^Xem #/ })).toHaveCount(2, { timeout: 15000 });
  await page
    .getByRole("button", { name: /^Xem #/ })
    .first()
    .click();
  await expect(page.getByRole("heading", { name: /^Request #/ })).toBeVisible();
  const body = page.getByText('"reason": "PAYMENT_FAILED",', { exact: true });
  await expect(body).toBeVisible();
  await page.screenshot({ path: "test-results/webhook-inbox.png", fullPage: true });
});

async function mockBooking(page, inboxID) {
  await page.route("**/api/backend/v1/sandbox/ride/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/list-available-services")) {
      await route.fulfill({
        json: {
          data: [
            {
              service: {
                id: 2179,
                travel_mode: "Car",
                available_vehicles: 2,
              },
            },
          ],
        },
      });
    } else if (path.endsWith("/estimate/order")) {
      await route.fulfill({
        json: {
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
        },
      });
    } else if (path.endsWith("/create-order")) {
      expect(route.request().postDataJSON().webhook_inbox_id).toBe(inboxID);
      await route.fulfill({ json: { data: { id: "attached-trip", status: "COMPLETED" } } });
    } else {
      throw new Error(`Unexpected ride request: ${path}`);
    }
  });
}

test("new bookings automatically use the selected inbox", async ({ page }) => {
  const { inbox } = await openInbox(page);
  await mockBooking(page, inbox.id);
  await page.getByRole("tab", { name: "Đặt chuyến", exact: true }).click();
  await page.getByRole("button", { name: "So sánh giá", exact: true }).click();
  await page.getByRole("button", { name: "Đặt xe", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Đã hoàn thành", exact: true })).toBeVisible();
});
