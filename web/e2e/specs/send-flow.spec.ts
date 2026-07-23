import { expect, test } from "@playwright/test";

test("ファイルをmultipart uploadしてURL共有shipmentを確定できる", async ({ page }) => {
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  page.on("pageerror", (error) => pageErrors.push(error.message));

  await page.goto("/send");

  await expect(page).toHaveURL(/\/send$/);
  await expect(page.getByRole("heading", { name: "新しいファイル送信" })).toBeVisible();
  await expect(page.getByText("ファイルをここへドロップ")).toBeVisible();

  await page.locator('input[type="file"]').setInputFiles({
    name: "proposal.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("Playwright E2E upload test"),
  });

  await expect(page.getByText("proposal.txt")).toBeVisible();
  await expect(page.getByText("1ファイル", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "アップロードを開始" }).click();

  await expect(page.getByRole("heading", { name: "送信内容" })).toBeVisible({
    timeout: 15_000,
  });
  await expect(
    page.getByText("すべてのファイルをアップロードしました。送信条件を設定してください。"),
  ).toBeVisible();

  await page.getByLabel("件名").fill("E2E送信テスト");
  await page.getByLabel("メッセージ").fill("Playwrightから送信条件を設定しました。");
  await page.getByRole("radio", { name: /URL共有/ }).check();
  await page.getByLabel("最大DL回数").fill("3");

  await page.getByRole("button", { name: "この内容で送信" }).click();

  await expect(page.getByRole("heading", { name: "送信を受け付けました" })).toBeVisible();
  await expect(page.getByText("共有URLを発行しました。必要な相手へ安全な方法でお知らせください。"))
    .toBeVisible();
  await expect(page.getByLabel("共有URL")).toHaveValue(
    "http://127.0.0.1:3000/r/e2e-token",
  );
  await expect(page.getByText("shipment-e2e-1")).toBeVisible();
  await expect(page.getByText("3回")).toBeVisible();

  expect(pageErrors).toEqual([]);
  expect(consoleErrors).toEqual([]);
});
