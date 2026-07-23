import { createServer } from "node:http";

const host = "127.0.0.1";
const port = Number(process.env.MOCK_API_PORT ?? 8081);
const appOrigin = "http://127.0.0.1:3000";
const sessions = new Map();
let uploadSequence = 0;

const server = createServer(async (request, response) => {
  const url = new URL(request.url ?? "/", `http://${host}:${port}`);

  if (request.method === "OPTIONS") {
    sendEmpty(response, 204, corsHeaders());
    return;
  }

  if (request.method === "GET" && url.pathname === "/health") {
    sendText(response, 200, "ok");
    return;
  }

  if (request.method === "GET" && url.pathname === "/v1/auth/me") {
    sendJSON(response, 200, {
      user: {
        id: "e2e-user-1",
        email: "e2e@example.com",
        display_name: "E2E User",
        status: "active",
        created_at: new Date().toISOString(),
      },
    });
    return;
  }

  if (request.method === "POST" && url.pathname === "/v1/uploads") {
    const body = await readJSON(request);
    if (
      !body ||
      typeof body.file_name !== "string" ||
      typeof body.file_size !== "number" ||
      body.file_size <= 0 ||
      typeof body.checksum_sha256 !== "string" ||
      body.checksum_sha256.length !== 64
    ) {
      sendAPIError(response, 400, "invalid_upload_request", "アップロード要求が不正です");
      return;
    }

    uploadSequence += 1;
    const uploadSessionId = `upload-session-${uploadSequence}`;
    const shipmentId = body.shipment_id || "shipment-e2e-1";
    const fileId = `file-${uploadSequence}`;
    const partSize = 4;
    const partCount = Math.ceil(body.file_size / partSize);

    sessions.set(uploadSessionId, {
      fileId,
      shipmentId,
      fileName: body.file_name,
      fileSize: body.file_size,
      partCount,
    });

    sendJSON(response, 201, {
      upload_session_id: uploadSessionId,
      shipment_id: shipmentId,
      object_key: `e2e/${uploadSessionId}/${body.file_name}`,
      s3_upload_id: `s3-${uploadSessionId}`,
      part_size: partSize,
      parts: Array.from({ length: partCount }, (_, index) => ({
        part_number: index + 1,
        presigned_url: `http://${host}:${port}/mock-s3/${uploadSessionId}/${index + 1}`,
      })),
      expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
    });
    return;
  }

  const completeMatch = url.pathname.match(/^\/v1\/uploads\/([^/]+)\/complete$/);
  if (request.method === "POST" && completeMatch) {
    const uploadSessionId = decodeURIComponent(completeMatch[1]);
    const session = sessions.get(uploadSessionId);
    const body = await readJSON(request);
    if (!session) {
      sendAPIError(response, 404, "upload_session_not_found", "upload sessionが見つかりません");
      return;
    }
    if (
      !body ||
      !Array.isArray(body.parts) ||
      body.parts.length !== session.partCount ||
      body.parts.some(
        (part) =>
          typeof part.part_number !== "number" ||
          typeof part.etag !== "string" ||
          part.etag.length === 0,
      )
    ) {
      sendAPIError(response, 400, "invalid_parts", "完了パート情報が不正です");
      return;
    }

    sendJSON(response, 200, {
      upload_session_id: uploadSessionId,
      file_id: session.fileId,
      shipment_id: session.shipmentId,
      status: "completed",
    });
    return;
  }

  const s3Match = url.pathname.match(/^\/mock-s3\/([^/]+)\/(\d+)$/);
  if (request.method === "PUT" && s3Match) {
    await consumeBody(request);
    const partNumber = Number(s3Match[2]);
    sendEmpty(response, 200, {
      ...corsHeaders(),
      ETag: `"e2e-etag-${partNumber}"`,
    });
    return;
  }

  if (request.method === "POST" && url.pathname === "/v1/shipments") {
    const body = await readJSON(request);
    if (
      !body ||
      typeof body.shipment_id !== "string" ||
      !Array.isArray(body.file_ids) ||
      body.file_ids.length === 0 ||
      typeof body.subject !== "string" ||
      body.subject.trim() === "" ||
      !["url_shared", "recipient_restricted"].includes(body.share_mode)
    ) {
      sendAPIError(response, 400, "invalid_shipment_request", "送信確定要求が不正です");
      return;
    }

    const recipients = Array.isArray(body.recipients)
      ? body.recipients.map((recipient, index) => ({
          id: `recipient-${index + 1}`,
          email: recipient.email,
          status: "pending",
        }))
      : [];

    sendJSON(response, 201, {
      id: body.shipment_id,
      status: "sent",
      share_mode: body.share_mode,
      expires_at: body.expires_at,
      max_download_count: body.max_download_count,
      access_url:
        body.share_mode === "url_shared"
          ? `${appOrigin}/r/e2e-token`
          : undefined,
      recipients,
      files: body.file_ids.map((fileId) => ({
        id: fileId,
        original_name: "proposal.txt",
        size_bytes: 17,
      })),
    });
    return;
  }

  const accessMatch = url.pathname.match(/^\/v1\/access\/([^/]+)$/);
  if (request.method === "GET" && accessMatch) {
    const token = decodeURIComponent(accessMatch[1]);
    if (token !== "e2e-token") {
      sendAPIError(response, 404, "token_not_found", "受信リンクが見つかりません");
      return;
    }

    sendJSON(response, 200, {
      requires_password: true,
      verified: hasAccessGrant(request),
      shipment: {
        id: "shipment-e2e-1",
        share_mode: "url_shared",
        subject: "E2E受信テスト",
        message: "Playwrightで受信フローを確認します。",
        expires_at: new Date(Date.now() + 24 * 60 * 60_000).toISOString(),
        max_download_count: 5,
      },
      files: [
        {
          id: "recipient-file-1",
          original_name: "contract.pdf",
          size_bytes: 1024,
        },
      ],
    });
    return;
  }

  const verifyMatch = url.pathname.match(/^\/v1\/access\/([^/]+)\/verify$/);
  if (request.method === "POST" && verifyMatch) {
    const token = decodeURIComponent(verifyMatch[1]);
    const body = await readJSON(request);
    if (token !== "e2e-token") {
      sendAPIError(response, 404, "token_not_found", "受信リンクが見つかりません");
      return;
    }
    if (body?.password !== "correct-password") {
      sendAPIError(response, 401, "invalid_password", "パスワードが一致しません");
      return;
    }

    sendJSON(
      response,
      200,
      {
        granted: true,
        expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
      },
      {
        "Set-Cookie": "e2e_access_grant=granted; HttpOnly; SameSite=Lax; Path=/; Max-Age=600",
      },
    );
    return;
  }

  const downloadURLMatch = url.pathname.match(/^\/v1\/files\/([^/]+)\/download-url$/);
  if (request.method === "GET" && downloadURLMatch) {
    const fileId = decodeURIComponent(downloadURLMatch[1]);
    if (url.searchParams.get("access_token") !== "e2e-token" || !hasAccessGrant(request)) {
      sendAPIError(
        response,
        401,
        "access_verification_required",
        "パスワード確認が必要です",
      );
      return;
    }

    sendJSON(response, 200, {
      url: `http://${host}:${port}/mock-download/${encodeURIComponent(fileId)}`,
      expires_at: new Date(Date.now() + 60_000).toISOString(),
    });
    return;
  }

  const downloadMatch = url.pathname.match(/^\/mock-download\/([^/]+)$/);
  if (request.method === "GET" && downloadMatch) {
    sendText(
      response,
      200,
      `mock download: ${decodeURIComponent(downloadMatch[1])}`,
      { "Content-Disposition": "attachment; filename=contract.pdf" },
    );
    return;
  }

  sendAPIError(response, 404, "not_found", `未定義のmock endpointです: ${request.method} ${url.pathname}`);
});

server.listen(port, host, () => {
  console.log(`Playwright mock API listening on http://${host}:${port}`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    server.close(() => process.exit(0));
  });
}

function hasAccessGrant(request) {
  return (request.headers.cookie ?? "")
    .split(";")
    .map((value) => value.trim())
    .includes("e2e_access_grant=granted");
}

function corsHeaders() {
  return {
    "Access-Control-Allow-Origin": appOrigin,
    "Access-Control-Allow-Methods": "PUT, OPTIONS",
    "Access-Control-Allow-Headers": "*",
    "Access-Control-Expose-Headers": "ETag",
  };
}

function sendAPIError(response, status, code, message) {
  sendJSON(response, status, {
    error: code,
    code,
    message,
    request_id: "e2e-request-id",
  });
}

function sendJSON(response, status, body, headers = {}) {
  response.writeHead(status, {
    "Content-Type": "application/json; charset=utf-8",
    ...headers,
  });
  response.end(JSON.stringify(body));
}

function sendText(response, status, body, headers = {}) {
  response.writeHead(status, {
    "Content-Type": "text/plain; charset=utf-8",
    ...headers,
  });
  response.end(body);
}

function sendEmpty(response, status, headers = {}) {
  response.writeHead(status, headers);
  response.end();
}

async function readJSON(request) {
  const chunks = [];
  for await (const chunk of request) {
    chunks.push(chunk);
  }
  if (chunks.length === 0) return null;
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    return null;
  }
}

async function consumeBody(request) {
  for await (const _chunk of request) {
    // mock S3 PUTでは内容を保持せず、受信完了だけを確認する。
  }
}
