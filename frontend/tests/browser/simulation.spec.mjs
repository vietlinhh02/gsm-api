import { expect, test } from "@playwright/test";

const scenarios = [
  ["no_drivers", "Hết xe · tìm lại rồi hủy", "NO_DRIVERS", "3 lần tìm"],
  [
    "payment_failed",
    "Thanh toán đối tác thất bại",
    "PAYMENT_FAILED",
    "Chuyến đã hủy trước khi tìm tài xế.",
  ],
];

function quoteResponse() {
  return {
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
}

async function mockScenario(page, scenario, reason) {
  await page.route("**/api/backend/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/healthz")) {
      await route.fulfill({ json: { status: "ok" } });
    } else if (path.endsWith("/token")) {
      await route.fulfill({ json: { access_token: "sandbox-token" } });
    } else if (path.endsWith("/list-available-services")) {
      await route.fulfill({
        json: {
          data: [{ service: { id: 2179, travel_mode: "Car", available_vehicles: 2 } }],
        },
      });
    } else if (path.endsWith("/estimate/order")) {
      await route.fulfill({ json: quoteResponse() });
    } else if (path.endsWith("/create-order")) {
      const body = route.request().postDataJSON();
      expect(body.sandbox_scenario).toBe(scenario);
      expect(body.payment_method).toBe(scenario === "payment_failed" ? "partner_pay" : "cash");
      await route.fulfill({ json: { data: { id: "scenario-trip", status: "FINDING" } } });
    } else if (path.endsWith("/events")) {
      const data = { id: "scenario-trip", status: "CANCELLED", simulation: { scenario, reason } };
      await route.fulfill({
        contentType: "text/event-stream",
        body: `event: order\ndata: ${JSON.stringify({ data })}\n\n`,
      });
    } else {
      throw new Error(`Unexpected API request: ${path}`);
    }
  });
}

for (const [scenario, label, reason, message] of scenarios) {
  test(`sandbox scenario ${scenario} sends the chosen scenario and explains cancellation`, async ({
    page,
  }) => {
    await mockScenario(page, scenario, reason);
    await page.goto("/playground");
    await page.getByRole("combobox", { name: "Kịch bản giả lập", exact: true }).click();
    await page.getByRole("option", { name: label, exact: true }).click();
    if (scenario === "payment_failed") {
      await expect(page.getByRole("combobox", { name: "Thanh toán", exact: true })).toBeDisabled();
    }
    await page.getByRole("button", { name: "So sánh giá", exact: true }).click();
    await expect(page.getByRole("columnheader", { name: "Xe rảnh lúc tra cứu" })).toBeVisible();
    await page.getByRole("button", { name: "Đặt xe", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Đã hủy", exact: true })).toBeVisible();
    await expect(page.getByText(message, { exact: false })).toBeVisible();
    await expect(page.getByRole("button", { name: "Gán tài xế", exact: true })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Hủy chuyến", exact: true })).toBeDisabled();
    await page.screenshot({ path: `test-results/simulation-${scenario}.png`, fullPage: true });
  });
}
