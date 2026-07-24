import { expect, test, type Page, type Route } from "@playwright/test";

const API_PATH = "/api/v1/shipments";

test("送信履歴を検索・絞り込みし、詳細を確認できる", async ({ page }) => {
  const diagnostics = collectDiagnostics(page);
  await installShipmentRoutes(page);

  await page.goto("/shipments");

  await expect(page).toHaveURL(/\/shipments$/);
  await expect(page.getByRole("heading", { name: "送信履歴" })).toBeVisible();
  await expect(page.getByText("E2E User", { exact: true })).toBeVisible();

  const summary = page.getByLabel("送信状況サマリー");
  const detailPanel = page.getByLabel("送信詳細");
  await expect(summary.getByText("3", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: /契約書一式/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /製品紹介資料/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /旧見積書/ })).toBeVisible();

  await expect(
    detailPanel.getByRole("heading", { name: "契約書一式", level: 2 }),
  ).toBeVisible();
  await expect(detailPanel.getByText("client@example.com", { exact: true })).toBeVisible();
  await expect(detailPanel.getByText("通知 1回", { exact: true })).toBeVisible();
  await expect(detailPanel.getByText("受領済み", { exact: true })).toBeVisible();
  await expect(detailPanel.getByRole("button", { name: "通知を再送" })).toBeVisible();

  const search = page.getByLabel("送信履歴を検索");
  await search.fill("shipment-history-2");
  await expect(page.getByRole("button", { name: /製品紹介資料/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /契約書一式/ })).toHaveCount(0);

  await page.getByRole("button", { name: /製品紹介資料/ }).click();
  await expect(
    detailPanel.getByRole("heading", { name: "製品紹介資料", level: 2 }),
  ).toBeVisible();
  await expect(detailPanel.getByText("URL共有", { exact: true })).toBeVisible();
  await expect(detailPanel.getByRole("button", { name: "通知を再送" })).toHaveCount(0);

  await search.clear();
  await page.getByLabel("状態で絞り込み").selectOption("accessed");
  await expect(page.getByRole("button", { name: /製品紹介資料/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /契約書一式/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /旧見積書/ })).toHaveCount(0);

  await page.getByLabel("状態で絞り込み").selectOption("all");
  await search.fill("一致しない検索語");
  await expect(page.getByText("条件に一致する送信履歴はありません。", { exact: true })).toBeVisible();

  expect(diagnostics.pageErrors).toEqual([]);
  expect(diagnostics.consoleErrors).toEqual([]);
});

test("受信者限定送信の通知を再送し、論理削除できる", async ({ page }) => {
  const diagnostics = collectDiagnostics(page);
  await installShipmentRoutes(page);

  await page.goto("/shipments");
  const detailPanel = page.getByLabel("送信詳細");
  await expect(
    detailPanel.getByRole("heading", { name: "契約書一式", level: 2 }),
  ).toBeVisible();

  const resendResponsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith(`${API_PATH}/shipment-history-1/resend`) &&
      response.request().method() === "POST",
  );
  await detailPanel.getByRole("button", { name: "通知を再送" }).click();
  expect((await resendResponsePromise).status()).toBe(202);

  await expect(
    page.getByText("受信者への通知を再送キューに登録しました。", { exact: true }),
  ).toBeVisible();
  await expect(detailPanel.getByText("通知 2回", { exact: true })).toBeVisible();

  page.once("dialog", async (dialog) => {
    expect(dialog.type()).toBe("confirm");
    expect(dialog.message()).toContain("契約書一式");
    expect(dialog.message()).toContain("受信リンクは直ちに無効になります");
    await dialog.accept();
  });

  const deleteResponsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith(`${API_PATH}/shipment-history-1`) &&
      response.request().method() === "DELETE",
  );
  await detailPanel.getByRole("button", { name: "送信を削除" }).click();
  expect((await deleteResponsePromise).status()).toBe(200);

  await expect(
    page.getByText("shipmentを論理削除し、受信リンクを無効化しました。", { exact: true }),
  ).toBeVisible();
  await expect(detailPanel.getByText("一覧から送信を選択してください。", { exact: true })).toBeVisible();

  const deletedRow = page.getByRole("button", { name: /契約書一式/ });
  await expect(deletedRow.getByText("削除済み", { exact: true })).toBeVisible();

  await deletedRow.click();
  await expect(
    detailPanel.getByRole("heading", { name: "契約書一式", level: 2 }),
  ).toBeVisible();
  await expect(detailPanel.getByText("削除済み", { exact: true })).toBeVisible();
  await expect(detailPanel.getByRole("button", { name: "通知を再送" })).toHaveCount(0);
  await expect(detailPanel.getByRole("button", { name: "送信を削除" })).toHaveCount(0);

  expect(diagnostics.pageErrors).toEqual([]);
  expect(diagnostics.consoleErrors).toEqual([]);
});

function collectDiagnostics(page: Page) {
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];

  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  page.on("pageerror", (error) => pageErrors.push(error.message));

  return { consoleErrors, pageErrors };
}

async function installShipmentRoutes(page: Page) {
  const fixtures = createShipmentFixtures();

  await page.route("**/api/v1/auth/me", async (route) => {
    await fulfillJSON(route, 200, {
      user: {
        id: "e2e-user-1",
        email: "e2e@example.com",
        display_name: "E2E User",
        status: "active",
        created_at: "2026-07-01T00:00:00Z",
      },
    });
  });

  await page.route("**/api/v1/shipments**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const pathname = url.pathname;
    const method = request.method();

    if (method === "GET" && pathname === API_PATH) {
      const limit = parseNonNegativeInteger(url.searchParams.get("limit"), 10);
      const offset = parseNonNegativeInteger(url.searchParams.get("offset"), 0);
      const items = fixtures.order.map((id) => fixtures.byId.get(id)!.listItem);
      await fulfillJSON(route, 200, {
        items: items.slice(offset, offset + limit),
        limit,
        offset,
        total: items.length,
      });
      return;
    }

    const resendMatch = pathname.match(/^\/api\/v1\/shipments\/([^/]+)\/resend$/);
    if (method === "POST" && resendMatch) {
      const id = decodeURIComponent(resendMatch[1]);
      const fixture = fixtures.byId.get(id);
      if (!fixture) {
        await fulfillAPIError(route, 404, "shipment_not_found", "shipmentが見つかりません");
        return;
      }

      const body = request.postDataJSON() as { recipient_ids?: unknown } | null;
      if (!body || !Array.isArray(body.recipient_ids)) {
        await fulfillAPIError(route, 400, "invalid_resend_request", "再送要求が不正です");
        return;
      }
      if (
        fixture.detail.share_mode !== "recipient_restricted" ||
        ["expired", "deleted", "revoked"].includes(fixture.detail.status)
      ) {
        await fulfillAPIError(route, 409, "shipment_not_resendable", "このshipmentは再送できません");
        return;
      }

      fixture.detail.notification_summary.total_notifications += 1;
      fixture.detail.notification_summary.queued_count += 1;
      fixture.detail.notification_summary.last_notification_at = "2026-07-24T03:30:00Z";
      fixture.detail.recipient_summaries = fixture.detail.recipient_summaries.map((recipient) => ({
        ...recipient,
        notification_count: recipient.notification_count + 1,
        last_notification_status: "queued",
        last_notification_type: "resend",
        last_notified_at: "2026-07-24T03:30:00Z",
      }));

      await fulfillJSON(route, 202, { queued_count: 1 });
      return;
    }

    const detailMatch = pathname.match(/^\/api\/v1\/shipments\/([^/]+)$/);
    if (detailMatch) {
      const id = decodeURIComponent(detailMatch[1]);
      const fixture = fixtures.byId.get(id);
      if (!fixture) {
        await fulfillAPIError(route, 404, "shipment_not_found", "shipmentが見つかりません");
        return;
      }

      if (method === "GET") {
        await fulfillJSON(route, 200, fixture.detail);
        return;
      }

      if (method === "DELETE") {
        if (!["deleted", "revoked"].includes(fixture.detail.status)) {
          fixture.detail.status = "deleted";
          fixture.listItem.status = "deleted";
        }
        await fulfillJSON(route, 200, { status: "deleted" });
        return;
      }
    }

    await fulfillAPIError(
      route,
      404,
      "e2e_route_not_found",
      `未定義のE2E routeです: ${method} ${pathname}`,
    );
  });
}

function createShipmentFixtures() {
  const fixtures = [
    createFixture({
      id: "shipment-history-1",
      subject: "契約書一式",
      message: "署名済み契約書と添付資料です。",
      shareMode: "recipient_restricted",
      status: "sent",
      createdAt: "2026-07-23T03:00:00Z",
      expiresAt: "2026-08-01T03:00:00Z",
      downloadCount: 1,
      maxDownloadCount: 5,
      files: [
        { id: "history-file-1", file_name: "contract.pdf", size: 2048 },
        { id: "history-file-2", file_name: "terms.txt", size: 512 },
      ],
      recipientEmail: "client@example.com",
      recipientDownloaded: true,
      notificationCount: 1,
    }),
    createFixture({
      id: "shipment-history-2",
      subject: "製品紹介資料",
      message: "公開URLで共有する製品資料です。",
      shareMode: "url_shared",
      status: "accessed",
      createdAt: "2026-07-22T03:00:00Z",
      expiresAt: "2026-08-05T03:00:00Z",
      downloadCount: 4,
      maxDownloadCount: 10,
      files: [{ id: "history-file-3", file_name: "product-guide.pdf", size: 4096 }],
    }),
    createFixture({
      id: "shipment-history-3",
      subject: "旧見積書",
      message: "有効期限切れの過去見積です。",
      shareMode: "recipient_restricted",
      status: "expired",
      createdAt: "2026-07-10T03:00:00Z",
      expiresAt: "2026-07-17T03:00:00Z",
      downloadCount: 0,
      maxDownloadCount: 3,
      files: [{ id: "history-file-4", file_name: "estimate.xlsx", size: 8192 }],
      recipientEmail: "buyer@example.com",
      recipientDownloaded: false,
      notificationCount: 1,
    }),
  ];

  return {
    order: fixtures.map((fixture) => fixture.listItem.id),
    byId: new Map(fixtures.map((fixture) => [fixture.listItem.id, fixture])),
  };
}

type FixtureInput = {
  id: string;
  subject: string;
  message: string;
  shareMode: "url_shared" | "recipient_restricted";
  status: string;
  createdAt: string;
  expiresAt: string;
  downloadCount: number;
  maxDownloadCount: number;
  files: Array<{ id: string; file_name: string; size: number }>;
  recipientEmail?: string;
  recipientDownloaded?: boolean;
  notificationCount?: number;
};

function createFixture(input: FixtureInput) {
  const recipients = input.recipientEmail
    ? [{ id: `${input.id}-recipient`, email: input.recipientEmail, status: "notified" }]
    : [];
  const recipientSummaries = input.recipientEmail
    ? [
        {
          recipient_id: `${input.id}-recipient`,
          email: input.recipientEmail,
          recipient_status: "notified",
          notification_count: input.notificationCount ?? 0,
          last_notification_status: "sent",
          last_notification_type: "initial",
          last_notified_at: input.createdAt,
          first_download_at: input.recipientDownloaded ? "2026-07-23T05:00:00Z" : undefined,
          last_download_at: input.recipientDownloaded ? "2026-07-23T05:00:00Z" : undefined,
          download_count: input.recipientDownloaded ? 1 : 0,
          has_downloaded: Boolean(input.recipientDownloaded),
        },
      ]
    : [];

  return {
    listItem: {
      id: input.id,
      subject: input.subject,
      share_mode: input.shareMode,
      status: input.status,
      created_at: input.createdAt,
      expires_at: input.expiresAt,
      download_count: input.downloadCount,
      max_download_count: input.maxDownloadCount,
      file_count: input.files.length,
    },
    detail: {
      id: input.id,
      status: input.status,
      share_mode: input.shareMode,
      subject: input.subject,
      message: input.message,
      expires_at: input.expiresAt,
      max_download_count: input.maxDownloadCount,
      download_count: input.downloadCount,
      last_download_at: input.downloadCount > 0 ? "2026-07-23T05:00:00Z" : undefined,
      files: input.files,
      recipients,
      notification_summary: {
        total_notifications: recipients.length,
        queued_count: 0,
        sent_count: recipients.length,
        failed_count: 0,
        last_notification_at: recipients.length > 0 ? input.createdAt : undefined,
      },
      recipient_summaries: recipientSummaries,
    },
  };
}

function parseNonNegativeInteger(value: string | null, fallback: number) {
  if (value === null) return fallback;
  const parsed = Number.parseInt(value, 10);
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : fallback;
}

async function fulfillJSON(route: Route, status: number, body: unknown) {
  await route.fulfill({
    status,
    contentType: "application/json; charset=utf-8",
    body: JSON.stringify(body),
  });
}

async function fulfillAPIError(
  route: Route,
  status: number,
  code: string,
  message: string,
) {
  await fulfillJSON(route, status, {
    error: code,
    code,
    message,
    request_id: "e2e-shipment-history-request",
  });
}
