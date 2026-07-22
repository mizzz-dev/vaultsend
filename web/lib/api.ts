import type {
  AccessInspectResponse,
  AccessVerifyResponse,
  ApiErrorPayload,
  AuthResponse,
  CompleteUploadResponse,
  CreateShipmentResponse,
  CreateUploadResponse,
  DownloadURLResponse,
  ShipmentDetail,
  ShipmentListResponse,
} from "@/lib/types";

const API_PREFIX = "/api/v1";

export class ApiClientError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;
  readonly upgradeRequired: boolean;
  readonly upgradeUrl?: string;

  constructor(status: number, payload: ApiErrorPayload) {
    super(payload.message ?? "APIリクエストに失敗しました");
    this.name = "ApiClientError";
    this.status = status;
    this.code = payload.code ?? payload.error ?? "unknown_error";
    this.requestId = payload.request_id;
    this.upgradeRequired = payload.upgrade_required ?? false;
    this.upgradeUrl = payload.upgrade_url;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${API_PREFIX}${path}`, {
    ...init,
    headers,
    credentials: "include",
    cache: "no-store",
  });

  if (response.status === 204) {
    return undefined as T;
  }

  const body = (await response.json().catch(() => null)) as T | ApiErrorPayload | null;
  if (!response.ok) {
    throw new ApiClientError(response.status, (body ?? {}) as ApiErrorPayload);
  }

  return body as T;
}

export const api = {
  register(input: { email: string; password: string; display_name?: string }) {
    return request<AuthResponse>("/auth/register", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  login(input: { email: string; password: string }) {
    return request<AuthResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  logout() {
    return request<void>("/auth/logout", { method: "POST" });
  },
  me() {
    return request<AuthResponse>("/auth/me");
  },
  createUpload(input: {
    shipment_id?: string;
    file_name: string;
    file_size: number;
    content_type: string;
    checksum_sha256: string;
  }) {
    return request<CreateUploadResponse>("/uploads", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  completeUpload(
    uploadSessionId: string,
    parts: Array<{ part_number: number; etag: string }>,
  ) {
    return request<CompleteUploadResponse>(
      `/uploads/${encodeURIComponent(uploadSessionId)}/complete`,
      {
        method: "POST",
        body: JSON.stringify({ parts }),
      },
    );
  },
  createShipment(input: {
    shipment_id: string;
    file_ids: string[];
    subject: string;
    message?: string;
    share_mode: "url_shared" | "recipient_restricted";
    recipients: Array<{ email: string }>;
    expires_at: string;
    max_download_count: number;
    password?: string;
  }) {
    return request<CreateShipmentResponse>("/shipments", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  inspectAccess(token: string) {
    return request<AccessInspectResponse>(`/access/${encodeURIComponent(token)}`);
  },
  verifyAccess(token: string, password?: string) {
    return request<AccessVerifyResponse>(`/access/${encodeURIComponent(token)}/verify`, {
      method: "POST",
      body: JSON.stringify({ password: password || undefined }),
    });
  },
  getDownloadURL(token: string, fileId: string) {
    const query = new URLSearchParams({ access_token: token });
    return request<DownloadURLResponse>(
      `/files/${encodeURIComponent(fileId)}/download-url?${query.toString()}`,
    );
  },
  listShipments(limit = 10, offset = 0) {
    const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    return request<ShipmentListResponse>(`/shipments?${query.toString()}`);
  },
  getShipment(id: string) {
    return request<ShipmentDetail>(`/shipments/${encodeURIComponent(id)}`);
  },
  deleteShipment(id: string) {
    return request<{ status: string }>(`/shipments/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
  },
  resendShipment(id: string) {
    return request<unknown>(`/shipments/${encodeURIComponent(id)}/resend`, {
      method: "POST",
      body: JSON.stringify({ recipient_ids: [] }),
    });
  },
};
