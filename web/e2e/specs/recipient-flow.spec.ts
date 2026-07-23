import { expect, test } from "@playwright/test";

test("保護された受信リンクを確認してファイルをダウンロードできる", async ({ page }) => {
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  page.on("pageerror", (error) => pageErrors.push(error.message));

  await page.goto("/r/e2e-token");

  await expect(page).toHaveURL(/\/r\/e2e-token$/);
  await expect(page.getByRole("heading", { name: "E2E受信テスト" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "パスワードで保護されています" })).toBeVisible();

  const invalidPasswordResponse = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/access/e2e-token/verify") &&
      response.request().method() === "POST",
  );
  await page.getByLabel("パスワード").fill("invalid-value");
  await page.getByRole("button", { name: "ファイルを表示" }).click();

  expect((await invalidPasswordResponse).status()).toBe(401);
  await expect(
    page.getByText("パスワードが一致しません。", { exact: true }),
  ).toBeVisible();

  // 不正パスワードの401はこのシナリオで意図した失敗なので、以降の予期しないconsole errorだけを監視する。
  consoleErrors.length = 0;

  await page.getByLabel("パスワード").fill("correct-password");
  await page.getByRole("button", { name: "ファイルを表示" }).click();

  await expect(page.getByRole("heading", { name: "受信ファイル" })).toBeVisible();
  await expect(page.getByText("確認済み")).toBeVisible();
  await expect(page.getByText("contract.pdf")).toBeVisible();

  await page.reload();
  await expect(page.getByRole("heading", { name: "受信ファイル" })).toBeVisible();
  await expect(page.getByText("確認済み")).toBeVisible();

  const downloadURLResponse = page.waitForResponse(
    (response) =>
      response.url().includes("/api/v1/files/recipient-file-1/download-url") &&
      response.request().method() === "GET",
  );
  const downloadStarted = page.waitForEvent("download");

  await page.getByRole("button", { name: "contract.pdfをダウンロード" }).click();

  expect((await downloadURLResponse).status()).toBe(200);
  const download = await downloadStarted;
  expect(download.suggestedFilename()).toBe("contract.pdf");
  expect(await download.failure()).toBeNull();

  expect(pageErrors).toEqual([]);
  expect(consoleErrors).toEqual([]);
});
