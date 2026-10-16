import { expect, test } from "@playwright/test";

async function selectOption(page, label, option) {
  await page.getByLabel(label, { exact: true }).click();
  await page.getByRole("option", { name: option, exact: true }).click();
}

test("hour estimate → booking → detail → cancel through the real API proxy", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/playground?endpoint=hour");
  await page.getByRole("tab", { name: "Xác thực", exact: true }).click();
  await page.getByRole("button", { name: "Lấy access token", exact: true }).click();
  await expect(page.getByLabel("Bearer token", { exact: true })).not.toHaveValue("");
  await page.getByRole("tab", { name: "API & JSON", exact: true }).click();
  await page.getByRole("button", { name: "Gửi request", exact: true }).click();
  await page.getByRole("button", { name: "Dùng báo giá để đặt", exact: true }).click();
  const booking = JSON.parse(
    await page.getByLabel("Request body (JSON)", { exact: true }).inputValue(),
  );
  expect(booking.service_type).toBe("RIDE-HOUR");
  expect(booking.order_time).toBe(0);
  expect(booking.fee_id).not.toBe("<fee_id>");
  await page.getByRole("button", { name: "Gửi request", exact: true }).click();
  await expect(page.getByText("Chuyến gần nhất:", { exact: false })).toBeVisible();
  await selectOption(page, "Endpoint", "GET · Chi tiết chuyến");
  await page.getByRole("button", { name: "Gửi request", exact: true }).click();
  await expect(page.getByText("HTTP 200", { exact: true })).toBeVisible();
  await selectOption(page, "Endpoint", "POST · Hủy chuyến");
  await page.getByRole("button", { name: "Gửi request", exact: true }).click();
  await expect(page.getByText("HTTP 200", { exact: true })).toBeVisible();
  expect(errors).toEqual([]);
});

test("invalid JSON is shown locally and Sandbox is the only environment", async ({ page }) => {
  await page.goto("/playground");
  await page.getByRole("tab", { name: "API & JSON", exact: true }).click();
  await page.getByLabel("Request body (JSON)", { exact: true }).fill("{");
  await page.getByRole("button", { name: "Gửi request", exact: true }).click();
  await expect(page.getByText(/JSON chưa hợp lệ/)).toBeVisible();
  await page.getByRole("tab", { name: "Xác thực", exact: true }).click();
  await page.getByLabel("Bearer token", { exact: true }).fill("sandbox-token");
  await expect(page.getByRole("combobox", { name: "Môi trường", exact: true })).toHaveCount(0);
  await expect(page.getByLabel("Bearer token", { exact: true })).toHaveValue("sandbox-token");
  await page.getByRole("tab", { name: "API & JSON", exact: true }).click();
  await selectOption(page, "Endpoint", "POST · Mô phỏng trạng thái");
  await expect(page.getByLabel("Request URL", { exact: true })).toHaveValue(
    "/v1/sandbox/ride/update-status",
  );
});

test("Astryx theme, searchable docs and mobile playground remain usable", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await page.getByRole("textbox", { name: "Tìm tài liệu", exact: true }).fill("uoc");
  await expect(page.getByRole("link", { name: /^Ước tính chuyến/ })).toBeVisible();
  await page.getByRole("button", { name: "Giao diện tối", exact: true }).click();
  await expect(page.getByRole("button", { name: "Giao diện sáng", exact: true })).toBeVisible();
  await page.screenshot({ path: "test-results/docs-dark.png" });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/playground");
  await page.getByRole("tab", { name: "API & JSON", exact: true }).click();
  await expect(page.getByRole("button", { name: "Gửi request", exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: "test-results/playground-mobile.png", fullPage: true });
  expect(errors).toEqual([]);
});
