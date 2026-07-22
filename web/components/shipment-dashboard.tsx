"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { api, ApiClientError } from "@/lib/api";
import type {
  AuthUser,
  ShipmentDetail,
  ShipmentListItem,
  ShipmentListResponse,
} from "@/lib/types";

const PAGE_SIZE = 10;

const dateFormatter = new Intl.DateTimeFormat("ja-JP", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

const numberFormatter = new Intl.NumberFormat("ja-JP");

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : dateFormatter.format(date);
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value < 0) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const digits = unitIndex === 0 || size >= 10 ? 0 : 1;
  return `${size.toFixed(digits)} ${units[unitIndex]}`;
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    draft: "下書き",
    uploading: "アップロード中",
    ready: "準備完了",
    sent: "送信済み",
    accessed: "アクセス済み",
    expired: "期限切れ",
    deleted: "削除済み",
    revoked: "取り消し済み",
  };
  return labels[status] ?? status;
}

function shareModeLabel(mode: string) {
  return mode === "recipient_restricted" ? "受信者限定" : "URL共有";
}

export function ShipmentDashboard() {
  const router = useRouter();
  const [user, setUser] = useState<AuthUser | null>(null);
  const [list, setList] = useState<ShipmentListResponse | null>(null);
  const [offset, setOffset] = useState(0);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<ShipmentDetail | null>(null);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const [isLoading, setIsLoading] = useState(true);
  const [isDetailLoading, setIsDetailLoading] = useState(false);
  const [isActionRunning, setIsActionRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const handleApiError = useCallback(
    (caught: unknown) => {
      if (caught instanceof ApiClientError && caught.status === 401) {
        router.replace("/auth");
        return;
      }
      setError(
        caught instanceof ApiClientError
          ? caught.message
          : "APIとの通信に失敗しました。サーバーの起動状態を確認してください。",
      );
    },
    [router],
  );

  const loadPage = useCallback(
    async (nextOffset: number) => {
      setIsLoading(true);
      setError(null);
      try {
        const [meResponse, shipmentResponse] = await Promise.all([
          api.me(),
          api.listShipments(PAGE_SIZE, nextOffset),
        ]);
        setUser(meResponse.user);
        setList(shipmentResponse);
        setOffset(nextOffset);

        const currentStillExists = shipmentResponse.items.some((item) => item.id === selectedId);
        if (!currentStillExists) {
          setSelectedId(shipmentResponse.items[0]?.id ?? null);
        }
      } catch (caught) {
        handleApiError(caught);
      } finally {
        setIsLoading(false);
      }
    },
    [handleApiError, selectedId],
  );

  const loadDetail = useCallback(
    async (shipmentId: string) => {
      setIsDetailLoading(true);
      setError(null);
      try {
        setDetail(await api.getShipment(shipmentId));
      } catch (caught) {
        setDetail(null);
        handleApiError(caught);
      } finally {
        setIsDetailLoading(false);
      }
    },
    [handleApiError],
  );

  useEffect(() => {
    void loadPage(0);
    // 初回のみ実行し、ページ変更は操作ハンドラから明示的に行う。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (selectedId) {
      void loadDetail(selectedId);
    } else {
      setDetail(null);
    }
  }, [loadDetail, selectedId]);

  const filteredItems = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return (list?.items ?? []).filter((item) => {
      const matchesQuery =
        normalized === "" ||
        item.subject.toLowerCase().includes(normalized) ||
        item.id.toLowerCase().includes(normalized);
      const matchesStatus = status === "all" || item.status === status;
      return matchesQuery && matchesStatus;
    });
  }, [list?.items, query, status]);

  const metrics = useMemo(() => {
    const items = list?.items ?? [];
    const active = items.filter(
      (item) => !["expired", "deleted", "revoked"].includes(item.status),
    ).length;
    const downloads = items.reduce((total, item) => total + item.download_count, 0);
    return { total: list?.total ?? 0, active, downloads };
  }, [list]);

  async function handleLogout() {
    setIsActionRunning(true);
    setError(null);
    try {
      await api.logout();
      router.replace("/auth");
      router.refresh();
    } catch (caught) {
      handleApiError(caught);
    } finally {
      setIsActionRunning(false);
    }
  }

  async function handleResend() {
    if (!detail) return;
    setIsActionRunning(true);
    setError(null);
    setNotice(null);
    try {
      await api.resendShipment(detail.id);
      setNotice("受信者への通知を再送キューに登録しました。");
      await loadDetail(detail.id);
    } catch (caught) {
      handleApiError(caught);
    } finally {
      setIsActionRunning(false);
    }
  }

  async function handleDelete() {
    if (!detail) return;
    const accepted = window.confirm(
      `「${detail.subject}」を削除します。受信リンクは直ちに無効になります。`,
    );
    if (!accepted) return;

    setIsActionRunning(true);
    setError(null);
    setNotice(null);
    try {
      await api.deleteShipment(detail.id);
      setNotice("shipmentを論理削除し、受信リンクを無効化しました。");
      setDetail(null);
      setSelectedId(null);
      await loadPage(offset);
    } catch (caught) {
      handleApiError(caught);
    } finally {
      setIsActionRunning(false);
    }
  }

  const pageStart = list && list.total > 0 ? offset + 1 : 0;
  const pageEnd = list ? Math.min(offset + list.items.length, list.total) : 0;
  const canGoPrevious = offset > 0;
  const canGoNext = list ? offset + PAGE_SIZE < list.total : false;

  return (
    <section className="shell page-section">
      <div className="page-heading">
        <div>
          <p className="eyebrow">Shipment workspace</p>
          <h1>送信履歴</h1>
          <p>通知、受領、ダウンロード回数を確認し、必要な相手へ安全に再送できます。</p>
        </div>
        <div className="inline-actions">
          {user && <span className="meta-text">{user.display_name || user.email}</span>}
          <button
            className="button button-secondary button-small"
            type="button"
            onClick={handleLogout}
            disabled={isActionRunning}
          >
            ログアウト
          </button>
        </div>
      </div>

      {error && <p className="alert alert-error" role="alert">{error}</p>}
      {notice && <p className="alert alert-success" role="status">{notice}</p>}

      <div className="metric-grid" aria-label="送信状況サマリー">
        <div className="metric">
          <span className="metric-label">全送信数</span>
          <strong className="metric-value">{numberFormatter.format(metrics.total)}</strong>
        </div>
        <div className="metric">
          <span className="metric-label">このページの有効な送信</span>
          <strong className="metric-value">{numberFormatter.format(metrics.active)}</strong>
        </div>
        <div className="metric">
          <span className="metric-label">このページのDL数</span>
          <strong className="metric-value">{numberFormatter.format(metrics.downloads)}</strong>
        </div>
      </div>

      <div className="dashboard-grid">
        <div className="panel">
          <div className="panel-header">
            <div>
              <strong>送信一覧</strong>
              <p>{pageStart}〜{pageEnd}件 / 全{list?.total ?? 0}件</p>
            </div>
          </div>

          <div className="toolbar">
            <input
              type="search"
              aria-label="送信履歴を検索"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="件名またはIDで検索"
            />
            <select
              aria-label="状態で絞り込み"
              value={status}
              onChange={(event) => setStatus(event.target.value)}
            >
              <option value="all">すべての状態</option>
              <option value="sent">送信済み</option>
              <option value="accessed">アクセス済み</option>
              <option value="ready">準備完了</option>
              <option value="expired">期限切れ</option>
              <option value="deleted">削除済み</option>
              <option value="revoked">取り消し済み</option>
            </select>
          </div>

          {isLoading ? (
            <p className="loading-state" role="status">送信履歴を読み込んでいます…</p>
          ) : filteredItems.length === 0 ? (
            <p className="empty-state">条件に一致する送信履歴はありません。</p>
          ) : (
            <div className="shipment-list">
              {filteredItems.map((item) => (
                <button
                  className="shipment-row"
                  type="button"
                  key={item.id}
                  aria-current={selectedId === item.id}
                  onClick={() => setSelectedId(item.id)}
                >
                  <span className="shipment-title">
                    <strong>{item.subject}</strong>
                    <span>{shareModeLabel(item.share_mode)}・{item.file_count}ファイル</span>
                  </span>
                  <span className="status-badge" data-status={item.status}>
                    {statusLabel(item.status)}
                  </span>
                  <span className="meta-text">期限 {formatDate(item.expires_at)}</span>
                  <strong>{item.download_count}/{item.max_download_count} DL</strong>
                </button>
              ))}
            </div>
          )}

          <div className="pagination">
            <button
              className="button button-secondary button-small"
              type="button"
              disabled={!canGoPrevious || isLoading}
              onClick={() => void loadPage(Math.max(0, offset - PAGE_SIZE))}
            >
              前へ
            </button>
            <span>{pageStart}〜{pageEnd}件を表示</span>
            <button
              className="button button-secondary button-small"
              type="button"
              disabled={!canGoNext || isLoading}
              onClick={() => void loadPage(offset + PAGE_SIZE)}
            >
              次へ
            </button>
          </div>
        </div>

        <aside className="panel detail-panel" aria-label="送信詳細">
          {isDetailLoading ? (
            <p className="loading-state" role="status">詳細を読み込んでいます…</p>
          ) : !detail ? (
            <p className="empty-state">一覧から送信を選択してください。</p>
          ) : (
            <ShipmentDetailPanel
              detail={detail}
              isActionRunning={isActionRunning}
              onResend={handleResend}
              onDelete={handleDelete}
            />
          )}
        </aside>
      </div>
    </section>
  );
}

type ShipmentDetailPanelProps = {
  detail: ShipmentDetail;
  isActionRunning: boolean;
  onResend: () => Promise<void>;
  onDelete: () => Promise<void>;
};

function ShipmentDetailPanel({
  detail,
  isActionRunning,
  onResend,
  onDelete,
}: ShipmentDetailPanelProps) {
  const canResend =
    detail.share_mode === "recipient_restricted" &&
    !["expired", "deleted", "revoked"].includes(detail.status);
  const canDelete = !["deleted", "revoked"].includes(detail.status);

  return (
    <>
      <div className="detail-header">
        <div>
          <span className="status-badge" data-status={detail.status}>
            {statusLabel(detail.status)}
          </span>
          <h2>{detail.subject}</h2>
        </div>
      </div>
      {detail.message && <p className="meta-text">{detail.message}</p>}

      <div className="detail-section">
        <h3>送信情報</h3>
        <ul className="detail-list">
          <li><span className="detail-key">共有方法</span><strong>{shareModeLabel(detail.share_mode)}</strong></li>
          <li><span className="detail-key">有効期限</span><strong>{formatDate(detail.expires_at)}</strong></li>
          <li><span className="detail-key">ダウンロード</span><strong>{detail.download_count}/{detail.max_download_count}</strong></li>
          <li><span className="detail-key">通知成功</span><strong>{detail.notification_summary.sent_count}件</strong></li>
          <li><span className="detail-key">通知失敗</span><strong>{detail.notification_summary.failed_count}件</strong></li>
        </ul>
      </div>

      <div className="detail-section">
        <h3>ファイル</h3>
        <ul className="detail-list">
          {detail.files.map((file) => (
            <li key={file.id}>
              <span>{file.file_name}</span>
              <strong>{formatBytes(file.size)}</strong>
            </li>
          ))}
        </ul>
      </div>

      {detail.recipient_summaries.length > 0 && (
        <div className="detail-section">
          <h3>受信者</h3>
          {detail.recipient_summaries.map((recipient) => (
            <div className="recipient-item" key={recipient.recipient_id}>
              <strong>{recipient.email}</strong>
              <div className="recipient-meta">
                <span>{recipient.has_downloaded ? "受領済み" : "未受領"}</span>
                <span>通知 {recipient.notification_count}回</span>
                <span>DL {recipient.download_count}回</span>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="inline-actions">
        {canResend && (
          <button
            className="button"
            type="button"
            disabled={isActionRunning}
            onClick={() => void onResend()}
          >
            通知を再送
          </button>
        )}
        {canDelete && (
          <button
            className="button button-danger"
            type="button"
            disabled={isActionRunning}
            onClick={() => void onDelete()}
          >
            送信を削除
          </button>
        )}
      </div>
    </>
  );
}
