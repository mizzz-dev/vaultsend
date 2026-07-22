"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, ApiClientError } from "@/lib/api";
import type { AccessInspectResponse } from "@/lib/types";
import styles from "./recipient-download-panel.module.css";

type FatalError = {
  title: string;
  message: string;
  code?: string;
};

export function RecipientDownloadPanel() {
  const params = useParams<{ token: string }>();
  const token = typeof params.token === "string" ? params.token : "";
  const [data, setData] = useState<AccessInspectResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [fatalError, setFatalError] = useState<FatalError | null>(null);
  const [isVerified, setIsVerified] = useState(false);
  const [password, setPassword] = useState("");
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [isVerifying, setIsVerifying] = useState(false);
  const [activeFileId, setActiveFileId] = useState<string | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);

  const loadAccess = useCallback(async () => {
    if (!token) {
      setFatalError({
        title: "受信リンクを確認できません",
        message: "URLが途中で切れている可能性があります。送信者から届いたリンクを開き直してください。",
        code: "invalid_token",
      });
      setIsLoading(false);
      return;
    }

    setIsLoading(true);
    setFatalError(null);
    try {
      const response = await api.inspectAccess(token);
      setData(response);
      setIsVerified(!response.requires_password);
    } catch (caught) {
      setFatalError(toFatalError(caught));
    } finally {
      setIsLoading(false);
    }
  }, [token]);

  useEffect(() => {
    void loadAccess();
  }, [loadAccess]);

  const totalSize = useMemo(
    () => data?.files.reduce((sum, file) => sum + file.size_bytes, 0) ?? 0,
    [data],
  );

  async function verifyPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!password.trim() || isVerifying) return;

    setPasswordError(null);
    setDownloadError(null);
    setIsVerifying(true);
    try {
      const response = await api.verifyAccess(token, password);
      if (!response.granted) {
        setPasswordError("パスワードを確認できませんでした。");
        return;
      }
      setIsVerified(true);
      setPassword("");
    } catch (caught) {
      if (caught instanceof ApiClientError) {
        if (caught.code === "invalid_password") {
          setPasswordError("パスワードが一致しません。");
          return;
        }
        if (caught.code === "verify_locked") {
          setPasswordError("入力回数が上限に達しました。時間をおいて再度お試しください。");
          return;
        }
        if (caught.code === "password_required") {
          setPasswordError("パスワードを入力してください。");
          return;
        }
      }
      setPasswordError("パスワード確認に失敗しました。通信状態を確認して再試行してください。");
    } finally {
      setIsVerifying(false);
    }
  }

  async function downloadFile(fileId: string) {
    if (activeFileId) return;

    setDownloadError(null);
    setActiveFileId(fileId);
    try {
      const response = await api.getDownloadURL(token, fileId);
      window.location.assign(response.url);
    } catch (caught) {
      if (caught instanceof ApiClientError) {
        if (caught.code === "access_verification_required") {
          setIsVerified(false);
          setPassword("");
          setPasswordError("確認状態の有効期限が切れました。パスワードを再入力してください。");
          return;
        }
        if (caught.code === "download_limit_exceeded") {
          setFatalError({
            title: "ダウンロード上限に達しました",
            message: "この受信リンクで利用できるダウンロード回数を超えています。必要な場合は送信者へ再送を依頼してください。",
            code: caught.code,
          });
          return;
        }
        if (caught.code === "download_rate_limited") {
          setDownloadError("短時間に操作が集中しています。少し時間をおいて再度お試しください。");
          return;
        }
      }
      setDownloadError("ダウンロードの準備に失敗しました。通信状態を確認して再試行してください。");
    } finally {
      setActiveFileId(null);
    }
  }

  if (isLoading) {
    return (
      <main className={styles.page}>
        <section className={styles.loadingCard} aria-live="polite">
          <span className={styles.spinner} aria-hidden="true" />
          <h1>受信リンクを確認しています</h1>
          <p>ファイル情報と有効期限を安全に確認しています。</p>
        </section>
      </main>
    );
  }

  if (fatalError) {
    return (
      <main className={styles.page}>
        <section className={styles.errorCard}>
          <span className={styles.errorIcon} aria-hidden="true">!</span>
          <h1>{fatalError.title}</h1>
          <p>{fatalError.message}</p>
          {fatalError.code && <small>エラーコード: {fatalError.code}</small>}
          <div className={styles.centerActions}>
            <button className={styles.secondaryButton} type="button" onClick={() => void loadAccess()}>
              もう一度確認
            </button>
            <Link className={styles.primaryButton} href="/">
              VaultSendトップへ
            </Link>
          </div>
        </section>
      </main>
    );
  }

  if (!data) return null;

  const expiresAt = new Date(data.shipment.expires_at);

  return (
    <main className={styles.page}>
      <section className={styles.accessCard}>
        <header className={styles.cardHeader}>
          <div className={styles.brandLine}>
            <span className={styles.brandMark} aria-hidden="true">V</span>
            <span>VaultSend</span>
          </div>
          <span className={styles.secureLabel}>暗号化された受信リンク</span>
        </header>

        <div className={styles.contentGrid}>
          <div className={styles.mainColumn}>
            <div className={styles.headingBlock}>
              <p className={styles.caption}>ファイルが届いています</p>
              <h1>{data.shipment.subject}</h1>
              {data.shipment.message && <p className={styles.message}>{data.shipment.message}</p>}
            </div>

            {data.requires_password && !isVerified ? (
              <form className={styles.passwordPanel} onSubmit={verifyPassword}>
                <div className={styles.lockIcon} aria-hidden="true">●</div>
                <div>
                  <h2>パスワードで保護されています</h2>
                  <p>送信者から案内されたパスワードを入力してください。</p>
                </div>
                <label className={styles.field} htmlFor="access-password">
                  <span>パスワード</span>
                  <input
                    id="access-password"
                    type="password"
                    autoComplete="current-password"
                    maxLength={256}
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    placeholder="パスワードを入力"
                    required
                    autoFocus
                  />
                </label>
                {passwordError && <p className={styles.inlineError} role="alert">{passwordError}</p>}
                <button className={styles.primaryButton} type="submit" disabled={isVerifying || !password.trim()}>
                  {isVerifying ? "確認しています…" : "ファイルを表示"}
                </button>
              </form>
            ) : (
              <div className={styles.fileSection}>
                <div className={styles.fileSectionHeader}>
                  <div>
                    <h2>受信ファイル</h2>
                    <p>{data.files.length}件・合計 {formatBytes(totalSize)}</p>
                  </div>
                  {data.requires_password && <span className={styles.verifiedLabel}>確認済み</span>}
                </div>

                {downloadError && <p className={styles.inlineError} role="alert">{downloadError}</p>}

                {data.files.length === 0 ? (
                  <p className={styles.emptyState}>ダウンロード可能なファイルがありません。</p>
                ) : (
                  <ul className={styles.fileList}>
                    {data.files.map((file) => (
                      <li key={file.id} className={styles.fileRow}>
                        <span className={styles.fileType}>{fileExtension(file.original_name)}</span>
                        <span className={styles.fileInfo}>
                          <strong>{file.original_name}</strong>
                          <small>{formatBytes(file.size_bytes)}</small>
                        </span>
                        <button
                          className={styles.downloadButton}
                          type="button"
                          onClick={() => void downloadFile(file.id)}
                          disabled={Boolean(activeFileId)}
                          aria-label={`${file.original_name}をダウンロード`}
                        >
                          {activeFileId === file.id ? "準備中…" : "ダウンロード"}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </div>

          <aside className={styles.summaryPanel}>
            <h2>受信情報</h2>
            <dl>
              <div>
                <dt>有効期限</dt>
                <dd>{formatDate(expiresAt)}</dd>
              </div>
              <div>
                <dt>共有方法</dt>
                <dd>{data.shipment.share_mode === "recipient_restricted" ? "受信者限定" : "URL共有"}</dd>
              </div>
              <div>
                <dt>最大DL回数</dt>
                <dd>{data.shipment.max_download_count}回</dd>
              </div>
            </dl>
            <div className={styles.securityNote}>
              <strong>安全に受け取るために</strong>
              <p>ダウンロードURLは発行後まもなく期限切れになります。共有端末ではファイル保存後に履歴を確認してください。</p>
            </div>
          </aside>
        </div>
      </section>
    </main>
  );
}

function toFatalError(caught: unknown): FatalError {
  if (!(caught instanceof ApiClientError)) {
    return {
      title: "受信リンクを確認できません",
      message: "通信状態を確認して、もう一度お試しください。",
    };
  }

  switch (caught.code) {
    case "token_expired":
    case "shipment_expired":
      return {
        title: "受信リンクの有効期限が切れています",
        message: "このリンクからはファイルを受け取れません。送信者へ再送を依頼してください。",
        code: caught.code,
      };
    case "download_limit_exceeded":
      return {
        title: "ダウンロード上限に達しました",
        message: "この受信リンクで利用できるダウンロード回数を超えています。送信者へ再送を依頼してください。",
        code: caught.code,
      };
    case "access_forbidden":
      return {
        title: "この受信リンクは利用できません",
        message: "送信者によって無効化されたか、ファイルが削除された可能性があります。",
        code: caught.code,
      };
    case "token_not_found":
    case "invalid_token":
    case "shipment_not_found":
      return {
        title: "受信リンクが見つかりません",
        message: "URLが正しいか確認し、送信者から届いたリンクを開き直してください。",
        code: caught.code,
      };
    default:
      return {
        title: "受信リンクを確認できません",
        message: "通信状態を確認して、もう一度お試しください。",
        code: caught.code,
      };
  }
}

function formatBytes(value: number) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size.toFixed(index === 0 || size >= 10 ? 0 : 1)} ${units[index]}`;
}

function fileExtension(fileName: string) {
  const extension = fileName.split(".").pop();
  return extension && extension !== fileName ? extension.slice(0, 4).toUpperCase() : "FILE";
}

function formatDate(value: Date) {
  return new Intl.DateTimeFormat("ja-JP", {
    year: "numeric",
    month: "long",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(value);
}
