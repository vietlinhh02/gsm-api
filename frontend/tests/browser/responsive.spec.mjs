import { expect, test } from "@playwright/test";

test("mobile menu opens, navigates and closes with Escape", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/playground", { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Mở menu", exact: true }).click({ timeout: 5000 });
  const drawer = page.getByRole("dialog");
  await expect(drawer).toBeVisible();
  await drawer.getByRole("link", { name: /^Ước tính chuyến/ }).click();
  await expect(page).toHaveURL(/\/docs\/trip$/);
  await expect(drawer).not.toBeVisible();
  await page.getByRole("button", { name: "Mở menu", exact: true }).click();
  await page.keyboard.press("Escape");
  await expect(drawer).not.toBeVisible();
});

test("desktop typography and playground panels remain readable", async ({ page }) => {
  await page.goto("/playground", { waitUntil: "domcontentloaded" });
  const tabs = page.getByRole("tablist", { name: "Công cụ chuyến xe", exact: true });
  await expect(tabs).toBeVisible({ timeout: 5000 });
  await page.getByRole("tab", { name: "Xác thực", exact: true }).click();
  await expect(page.getByLabel("Client ID", { exact: true })).toBeVisible();
  await page.getByRole("tab", { name: "API & JSON", exact: true }).click();
  await expect(page.getByLabel("Request body (JSON)", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Client ID", { exact: true })).not.toBeVisible();
  const bodyStyle = await page
    .getByRole("heading", { name: "API playground", exact: true })
    .evaluate((el) => {
      const style = getComputedStyle(el);
      return { family: style.fontFamily, size: Number.parseFloat(style.fontSize) };
    });
  expect(bodyStyle.family).toContain("Be Vietnam Pro");
  expect(bodyStyle.size).toBeGreaterThanOrEqual(18);
});

test("geolocation selects only the chosen point and tabs preserve drafts", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["geolocation"]);
  await context.setGeolocation({ latitude: 21.015, longitude: 105.8, accuracy: 25 });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/playground", { waitUntil: "domcontentloaded" });
  const pickup = page.locator(".selected-location").first();
  const dropoff = page.locator(".selected-location").last();
  await expect(pickup).toContainText("Hồ Hoàn Kiếm");
  await page.getByRole("button", { name: "Chọn điểm đến", exact: true }).click();
  await page.getByRole("button", { name: "Lấy vị trí của tôi", exact: true }).click();
  await expect(dropoff).toContainText("21.015000, 105.800000");
  await expect(pickup).toContainText("Hồ Hoàn Kiếm");
  await expect(page.getByText(/sai số khoảng 25 m/)).toBeVisible();
  await page.getByLabel("Nền bản đồ", { exact: true }).click();
  await page.getByRole("option", { name: "Ban đêm", exact: true }).click();
  await expect(page.locator(".route-map")).toHaveClass(/map-style-dark/);
  await page.getByRole("tab", { name: "API & JSON", exact: true }).click();
  await page.getByLabel("Request body (JSON)", { exact: true }).fill('{"draft":true}');
  await page.getByRole("tab", { name: "Đặt chuyến", exact: true }).click();
  await expect(dropoff).toContainText("21.015000, 105.800000");
  await expect(page.locator(".route-map")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: "test-results/mobile-location.png", fullPage: true });
  await page.getByRole("tab", { name: "API & JSON", exact: true }).click();
  await expect(page.getByLabel("Request body (JSON)", { exact: true })).toHaveValue(
    '{"draft":true}',
  );
});

test("denied location permission shows recovery without changing the route", async ({
  page,
  context,
}) => {
  await context.clearPermissions();
  await page.addInitScript(() => {
    navigator.geolocation.getCurrentPosition = (_success, failure) => failure({ code: 1 });
  });
  await page.goto("/playground", { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Lấy vị trí của tôi", exact: true }).click();
  await expect(page.getByText(/Bạn đã từ chối quyền vị trí/)).toBeVisible();
  await expect(page.locator(".selected-location").first()).toContainText("Hồ Hoàn Kiếm");
  await expect(page.getByRole("button", { name: "Lấy vị trí của tôi", exact: true })).toBeEnabled();
});
