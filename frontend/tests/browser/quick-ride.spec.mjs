import { expect, test } from "@playwright/test";

async function selectOption(page, label, option) {
  await page.getByRole("combobox", { name: label, exact: true }).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}

test("Hanoi bike quote → booking → driver → completed with the real backend", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/playground");
  await page.getByRole("button", { name: "Dùng Sandbox để test tài xế", exact: true }).click();
  await selectOption(page, "Loại xe test", "Xe máy · Green Motorcycle");
  const responsePromise = page.waitForResponse(
    (response) =>
      response.url().includes("/ride/estimate/order") && response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "So sánh giá ô tô / xe máy", exact: true }).click();
  const response = await responsePromise;
  expect(response.status()).toBe(200);
  const data = (await response.json()).data;
  const bike = data.service_estimates.find((item) => item.service.id === 2180).estimate_info;
  const car = data.service_estimates.find((item) => item.service.id === 2179).estimate_info;
  expect(bike.estimate.total_pay).toBeLessThan(car.estimate.total_pay);
  expect(bike.pricing.simulated).toBe(true);
  expect(bike.pricing.items.reduce((sum, item) => sum + item.amount, 0)).toBe(
    bike.estimate.total_pay,
  );
  expect(bike.trip.geometry.type).toBe("LineString");
  await expect(page.getByRole("heading", { name: /^Giá xe máy:/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "Điểm đón:", exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: "Điểm đến:", exact: false })).toBeVisible();
  await expect(page.locator(".route-map .leaflet-overlay-pane path")).toHaveCount(1);
  await page.screenshot({ path: "test-results/quick-ride-pricing.png", fullPage: true });
  await page.getByRole("button", { name: "Đặt chuyến đã chọn", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Chuyến test · FINDING", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Bắt đầu chuyến", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Gán tài xế", exact: true }).click();
  await expect(page.getByText(/Simulated Driver · VinFast Feliz S/)).toBeVisible();
  await page.getByRole("button", { name: "Bắt đầu chuyến", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Chuyến test · IN PROCESS", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Hủy chuyến test", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Hoàn thành chuyến", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Chuyến test · COMPLETED", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Gán tài xế", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Tải lại chuyến", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Chuyến test · COMPLETED", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("map selection, swap, explicit address search, failures and mobile layout", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/playground");
  await expect(page.locator(".route-map.leaflet-container")).toBeVisible();
  const pickup = page.locator(".selected-location").first();
  const dropoff = page.locator(".selected-location").last();
  const originalPickup = await pickup.innerText();
  await page.getByRole("button", { name: "Chọn điểm đến", exact: true }).click();
  await page.locator(".route-map").click({ position: { x: 150, y: 180 } });
  await expect(dropoff).not.toContainText("Lotte Center");
  await expect(pickup).toHaveText(originalPickup, { useInnerText: true });
  await page.getByRole("button", { name: "Đảo chiều", exact: true }).click();
  await expect(dropoff).toContainText("Hồ Hoàn Kiếm");
  let calls = 0;
  await page.route("**/api/backend/v1/place/geocode?**", async (route) => {
    calls += 1;
    await route.fulfill({
      json: {
        data: [
          { place_id: "W123", label: "Văn Miếu, Hà Nội", latitude: 21.0281, longitude: 105.8355 },
        ],
      },
    });
  });
  await page.getByLabel("Tìm địa chỉ tại điểm đang chọn", { exact: true }).fill("Văn Miếu, Hà Nội");
  expect(calls).toBe(0);
  await page.getByRole("button", { name: "Tìm địa chỉ", exact: true }).click();
  await page.getByRole("button", { name: "Văn Miếu, Hà Nội", exact: true }).click();
  await expect(dropoff).toContainText("Văn Miếu, Hà Nội");
  await page.route("**/api/backend/v1/ride/estimate/order", (route) =>
    route.fulfill({ status: 502, json: { message: "Provider test unavailable" } }),
  );
  await page.getByRole("button", { name: "So sánh giá ô tô / xe máy", exact: true }).click();
  await expect(
    page.getByText("HTTP 502: Provider test unavailable", { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Đặt chuyến đã chọn", exact: true })).toHaveCount(
    0,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: "test-results/quick-ride-mobile.png", fullPage: true });
});
