"use client";

import {
  ChangeEvent,
  DragEvent,
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { api, ApiClientError } from "@/lib/api";
import { calculateFileSha256 } from "@/lib/file-hash";
import {
  CompletedUploadPart,
  uploadMultipartFile,
} from "@/lib/multipart-upload";
import type {
  CreateShipmentResponse,
  CreateUploadResponse,
} from "@/lib/types";
import styles from "./send-wizard.module.css";

const MAX_FILES = 20;
const MAX_FILE_SIZE = 10 * 1024 * 1024 * 1024;

type WizardStep = "files" | "uploading" | "settings" | "complete";
type UploadStatus =
  | "pending"
  | "hashing"
  | "uploading"
  | "completing"
  | "completed"
  | "failed";

type UploadTask = {
  id: string;
  file: File;
  status: UploadStatus;
  hashProgress: number;
  uploadProgress: number;
  checksum?: string;
  session?: CreateUploadResponse;
  completedParts: CompletedUploadPart[];
  fileId?: string;
  error?: string;
};

const statusLabels: Record<UploadStatus, string> = {
  pending: "待機中",
  hashing: "チェックサム計算中",
  uploading: "アップロード中",
  completing: "完了処理中",
  completed: "完了",
  failed: "失敗",
};

export function SendWizard() {
  const router = useRouter();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const tasksRef = useRef<UploadTask[]>([]);
  const shipmentIdRef = useRef<string | undefined>(undefined);

  const [step, setStep] = useState<WizardStep>("files");
  const [tasks, setTasks] = useState<UploadTask[]>([]);
  const [isAuthenticating, setIsAuthenticating] = useState(true);
  const [isRunning, setIsRunning] = useState(false);
  const [isDragActive, setIsDragActive] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [subject, setSubject] = useState("");
  const [message, setMessage] = useState("");
  const [shareMode, setShareMode] = useState<"url_shared" | "recipient_restricted">(
    "recipient_restricted",
  );
  const [recipientText, setRecipientText] = useState("");
  const [expiresInDays, setExpiresInDays] = useState(7);
  const [maxDownloads, setMaxDownloads] = useState(10);
  const [password, setPassword] = useState("");
  const [result, setResult] = useState<CreateShipmentResponse | null>(null);

  useEffect(() => {
    let active = true;
    api
      .me()
      .catch((caught) => {
        if (active && caught instanceof ApiClientError && caught.status === 401) {
          router.replace("/auth?next=/send");
          return;
        }
        if (active) {
          setError(errorMessage(caught));
        }
      })
      .finally(() => {
        if (active) setIsAuthenticating(false);
      });
    return () => {
      active = false;
    };
  }, [router]);

  useEffect(() => {
    if (!isRunning) return;
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warnBeforeUnload);
    return () => window.removeEventListener("beforeunload", warnBeforeUnload);
  }, [isRunning]);

  const replaceTasks = useCallback((next: UploadTask[]) => {
    tasksRef.current = next;
    setTasks(next);
  }, []);

  const updateTask = useCallback(
    (taskId: string, changes: Partial<UploadTask>) => {
      replaceTasks(
        tasksRef.current.map((task) =>
          task.id === taskId ? { ...task, ...changes } : task,
        ),
      );
    },
    [replaceTasks],
  );

  const totalSize = useMemo(
    () => tasks.reduce((sum, task) => sum + task.file.size, 0),
    [tasks],
  );

  const overallProgress = useMemo(() => {
    if (totalSize === 0) return 0;
    const weighted = tasks.reduce((sum, task) => {
      let progress = 0;
      switch (task.status) {
        case "hashing":
          progress = task.hashProgress * 0.15;
          break;
        case "uploading":
          progress = 0.15 + task.uploadProgress * 0.8;
          break;
        case "completing":
          progress = 0.97;
          break;
        case "completed":
          progress = 1;
          break;
        default:
          progress = 0;
      }
      return sum + progress * task.file.size;
    }, 0);
    return weighted / totalSize;
  }, [tasks, totalSize]);

  const hasFailedTask = tasks.some((task) => task.status === "failed");

  function handleFileInput(event: ChangeEvent<HTMLInputElement>) {
    addFiles(Array.from(event.target.files ?? []));
    event.target.value = "";
  }

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setIsDragActive(false);
    addFiles(Array.from(event.dataTransfer.files));
  }

  function addFiles(files: File[]) {
    if (step !== "files" || isRunning) return;
    setError(null);
    setNotice(null);

    const existingKeys = new Set(
      tasksRef.current.map((task) => fileKey(task.file)),
    );
    const accepted: UploadTask[] = [];
    const rejected: string[] = [];

    for (const file of files) {
      if (existingKeys.has(fileKey(file))) continue;
      if (tasksRef.current.length + accepted.length >= MAX_FILES) {
        rejected.push(`ファイル数は${MAX_FILES}件までです`);
        break;
      }
      if (file.size <= 0) {
        rejected.push(`${file.name}: 空のファイルは送信できません`);
        continue;
      }
      if (file.size > MAX_FILE_SIZE) {
        rejected.push(`${file.name}: 10GBを超えています`);
        continue;
      }
      if (file.name.length > 255) {
        rejected.push(`${file.name}: ファイル名が長すぎます`);
        continue;
      }
      existingKeys.add(fileKey(file));
      accepted.push({
        id: crypto.randomUUID(),
        file,
        status: "pending",
        hashProgress: 0,
        uploadProgress: 0,
        completedParts: [],
      });
    }

    replaceTasks([...tasksRef.current, ...accepted]);
    if (rejected.length > 0) setError(rejected.join("\n"));
  }

  function removeFile(taskId: string) {
    if (step !== "files" || isRunning) return;
    replaceTasks(tasksRef.current.filter((task) => task.id !== taskId));
  }

  async function startUploads() {
    if (tasksRef.current.length === 0 || isRunning) return;
    setError(null);
    setNotice(null);
    setStep("uploading");
    setIsRunning(true);

    try {
      for (const seed of tasksRef.current) {
        let task = tasksRef.current.find((item) => item.id === seed.id);
        if (!task || task.status === "completed") continue;

        try {
          if (!task.checksum) {
            updateTask(task.id, {
              status: "hashing",
              error: undefined,
              hashProgress: 0,
            });
            const checksum = await calculateFileSha256(task.file, (progress) =>
              updateTask(task!.id, { hashProgress: progress }),
            );
            updateTask(task.id, { checksum, hashProgress: 1 });
            task = { ...task, checksum, hashProgress: 1 };
          }

          let session = task.session;
          if (!session) {
            session = await api.createUpload({
              shipment_id: shipmentIdRef.current,
              file_name: task.file.name,
              file_size: task.file.size,
              content_type: task.file.type || "application/octet-stream",
              checksum_sha256: task.checksum!,
            });
            shipmentIdRef.current = session.shipment_id;
            updateTask(task.id, { session });
          } else if (new Date(session.expires_at).getTime() <= Date.now()) {
            throw new Error(
              "アップロードURLの有効期限が切れました。未完了データを整理後、最初からやり直してください。",
            );
          }

          updateTask(task.id, {
            status: "uploading",
            error: undefined,
          });

          const completedParts = await uploadMultipartFile({
            file: task.file,
            partSize: session.part_size,
            parts: session.parts,
            completedParts: task.completedParts,
            concurrency: 3,
            maxAttempts: 3,
            onProgress: (progress) =>
              updateTask(task!.id, { uploadProgress: progress }),
            onPartCompleted: (part) => {
              const latest = tasksRef.current.find((item) => item.id === task!.id);
              const merged = mergeCompletedParts(latest?.completedParts ?? [], part);
              updateTask(task!.id, { completedParts: merged });
            },
          });

          updateTask(task.id, {
            status: "completing",
            uploadProgress: 1,
            completedParts,
          });
          const completed = await api.completeUpload(
            session.upload_session_id,
            completedParts,
          );
          updateTask(task.id, {
            status: "completed",
            fileId: completed.file_id,
            uploadProgress: 1,
            error: undefined,
          });
        } catch (caught) {
          const messageText = errorMessage(caught);
          updateTask(task.id, { status: "failed", error: messageText });
          throw caught;
        }
      }

      setStep("settings");
      setNotice("すべてのファイルをアップロードしました。送信条件を設定してください。");
    } catch (caught) {
      setError(errorMessage(caught));
    } finally {
      setIsRunning(false);
    }
  }

  async function finalizeShipment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isRunning || !shipmentIdRef.current) return;
    setError(null);
    setNotice(null);

    const recipients = parseRecipients(recipientText);
    const validationError = validateSettings({
      subject,
      message,
      shareMode,
      recipients,
      expiresInDays,
      maxDownloads,
      password,
    });
    if (validationError) {
      setError(validationError);
      return;
    }

    const fileIds = tasksRef.current
      .map((task) => task.fileId)
      .filter((fileId): fileId is string => Boolean(fileId));
    if (fileIds.length !== tasksRef.current.length) {
      setError("アップロード完了済みのファイル情報が不足しています。再試行してください。");
      return;
    }

    setIsRunning(true);
    try {
      const created = await api.createShipment({
        shipment_id: shipmentIdRef.current,
        file_ids: fileIds,
        subject: subject.trim(),
        message: message.trim() || undefined,
        share_mode: shareMode,
        recipients:
          shareMode === "recipient_restricted"
            ? recipients.map((email) => ({ email }))
            : [],
        expires_at: buildExpiry(expiresInDays),
        max_download_count: maxDownloads,
        password: password || undefined,
      });
      setResult(created);
      setStep("complete");
    } catch (caught) {
      setError(errorMessage(caught));
    } finally {
      setIsRunning(false);
    }
  }

  async function copyAccessUrl() {
    if (!result?.access_url) return;
    try {
      await navigator.clipboard.writeText(result.access_url);
      setNotice("共有URLをコピーしました。");
    } catch {
      setError("共有URLをコピーできませんでした。手動で選択してコピーしてください。");
    }
  }

  function resetWizard() {
    shipmentIdRef.current = undefined;
    replaceTasks([]);
    setStep("files");
    setSubject("");
    setMessage("");
    setShareMode("recipient_restricted");
    setRecipientText("");
    setExpiresInDays(7);
    setMaxDownloads(10);
    setPassword("");
    setResult(null);
    setError(null);
    setNotice(null);
  }

  if (isAuthenticating) {
    return <p className={styles.loading}>ログイン状態を確認しています…</p>;
  }

  return (
    <section className={styles.wrapper}>
      <div className={styles.heading}>
        <div>
          <p className={styles.kicker}>Secure delivery</p>
          <h1>新しいファイル送信</h1>
          <p>ファイルをS3へ直接分割アップロードし、期限と受信者を設定して送信します。</p>
        </div>
        <Link className={styles.historyLink} href="/shipments">
          送信履歴へ
        </Link>
      </div>

      <StepIndicator step={step} />

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}
      {notice && (
        <p className={styles.notice} role="status">
          {notice}
        </p>
      )}

      {step === "files" && (
        <div className={styles.panel}>
          <div
            className={`${styles.dropzone} ${isDragActive ? styles.dropzoneActive : ""}`}
            onDragOver={(event) => {
              event.preventDefault();
              setIsDragActive(true);
            }}
            onDragLeave={() => setIsDragActive(false)}
            onDrop={handleDrop}
          >
            <input
              ref={fileInputRef}
              className={styles.visuallyHidden}
              type="file"
              multiple
              onChange={handleFileInput}
            />
            <strong>ファイルをここへドロップ</strong>
            <span>またはファイル選択から追加</span>
            <button
              className={styles.primaryButton}
              type="button"
              onClick={() => fileInputRef.current?.click()}
            >
              ファイルを選択
            </button>
            <small>1ファイル10GB、最大20ファイルまで</small>
          </div>

          {tasks.length > 0 && (
            <>
              <div className={styles.fileSummary}>
                <strong>{tasks.length}ファイル</strong>
                <span>合計 {formatBytes(totalSize)}</span>
              </div>
              <ul className={styles.fileList}>
                {tasks.map((task) => (
                  <li key={task.id} className={styles.fileRow}>
                    <span className={styles.fileType}>{fileExtension(task.file.name)}</span>
                    <span className={styles.fileName}>
                      <strong>{task.file.name}</strong>
                      <small>{formatBytes(task.file.size)}</small>
                    </span>
                    <button
                      className={styles.textButton}
                      type="button"
                      onClick={() => removeFile(task.id)}
                    >
                      削除
                    </button>
                  </li>
                ))}
              </ul>
              <div className={styles.actions}>
                <button
                  className={styles.primaryButton}
                  type="button"
                  onClick={() => void startUploads()}
                >
                  アップロードを開始
                </button>
              </div>
            </>
          )}
        </div>
      )}

      {step === "uploading" && (
        <div className={styles.panel}>
          <div className={styles.progressHeader}>
            <div>
              <h2>ファイルをアップロードしています</h2>
              <p>画面を閉じずに完了までお待ちください。</p>
            </div>
            <strong>{Math.round(overallProgress * 100)}%</strong>
          </div>
          <Progress value={overallProgress} label="全体進捗" />
          <ul className={styles.uploadList}>
            {tasks.map((task) => {
              const progress = taskProgress(task);
              return (
                <li key={task.id} className={styles.uploadRow}>
                  <div className={styles.uploadMeta}>
                    <span>
                      <strong>{task.file.name}</strong>
                      <small>{statusLabels[task.status]}</small>
                    </span>
                    <span>{Math.round(progress * 100)}%</span>
                  </div>
                  <Progress value={progress} label={`${task.file.name}の進捗`} />
                  {task.error && <p className={styles.inlineError}>{task.error}</p>}
                </li>
              );
            })}
          </ul>
          {hasFailedTask && !isRunning && (
            <div className={styles.actions}>
              <button
                className={styles.primaryButton}
                type="button"
                onClick={() => void startUploads()}
              >
                失敗した処理を再試行
              </button>
            </div>
          )}
        </div>
      )}

      {step === "settings" && (
        <form className={styles.settingsGrid} onSubmit={finalizeShipment}>
          <div className={styles.panel}>
            <h2>送信内容</h2>
            <div className={styles.field}>
              <label htmlFor="subject">件名</label>
              <input
                id="subject"
                maxLength={200}
                required
                value={subject}
                onChange={(event) => setSubject(event.target.value)}
                placeholder="例：7月分の契約書"
              />
            </div>
            <div className={styles.field}>
              <label htmlFor="message">メッセージ</label>
              <textarea
                id="message"
                maxLength={5000}
                rows={5}
                value={message}
                onChange={(event) => setMessage(event.target.value)}
                placeholder="受信者への補足を入力"
              />
            </div>

            <fieldset className={styles.fieldset}>
              <legend>共有方法</legend>
              <label className={styles.radioRow}>
                <input
                  type="radio"
                  name="share-mode"
                  checked={shareMode === "recipient_restricted"}
                  onChange={() => setShareMode("recipient_restricted")}
                />
                <span>
                  <strong>受信者限定</strong>
                  <small>指定したメールアドレスへ専用リンクを送ります</small>
                </span>
              </label>
              <label className={styles.radioRow}>
                <input
                  type="radio"
                  name="share-mode"
                  checked={shareMode === "url_shared"}
                  onChange={() => setShareMode("url_shared")}
                />
                <span>
                  <strong>URL共有</strong>
                  <small>送信完了後に共有URLを発行します</small>
                </span>
              </label>
            </fieldset>

            {shareMode === "recipient_restricted" && (
              <div className={styles.field}>
                <label htmlFor="recipients">受信者メールアドレス</label>
                <textarea
                  id="recipients"
                  rows={5}
                  value={recipientText}
                  onChange={(event) => setRecipientText(event.target.value)}
                  placeholder={"a@example.com\nb@example.com"}
                  required
                />
                <small>改行またはカンマ区切り。最大20件まで。</small>
              </div>
            )}
          </div>

          <aside className={styles.panel}>
            <h2>セキュリティ設定</h2>
            <div className={styles.twoColumns}>
              <div className={styles.field}>
                <label htmlFor="expires">有効日数</label>
                <input
                  id="expires"
                  type="number"
                  min={1}
                  max={14}
                  value={expiresInDays}
                  onChange={(event) => setExpiresInDays(Number(event.target.value))}
                  required
                />
              </div>
              <div className={styles.field}>
                <label htmlFor="downloads">最大DL回数</label>
                <input
                  id="downloads"
                  type="number"
                  min={1}
                  max={100}
                  value={maxDownloads}
                  onChange={(event) => setMaxDownloads(Number(event.target.value))}
                  required
                />
              </div>
            </div>
            <div className={styles.field}>
              <label htmlFor="send-password">パスワード（任意）</label>
              <input
                id="send-password"
                type="password"
                minLength={8}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="設定する場合は8文字以上"
              />
            </div>

            <div className={styles.summaryBox}>
              <strong>送信対象</strong>
              <ul>
                {tasks.map((task) => (
                  <li key={task.id}>
                    <span>{task.file.name}</span>
                    <span>{formatBytes(task.file.size)}</span>
                  </li>
                ))}
              </ul>
              <div className={styles.summaryTotal}>
                <span>合計</span>
                <strong>{formatBytes(totalSize)}</strong>
              </div>
            </div>

            <button
              className={styles.primaryButton}
              type="submit"
              disabled={isRunning}
            >
              {isRunning ? "送信を確定しています…" : "この内容で送信"}
            </button>
          </aside>
        </form>
      )}

      {step === "complete" && result && (
        <div className={`${styles.panel} ${styles.completePanel}`}>
          <span className={styles.completeIcon} aria-hidden="true">✓</span>
          <h2>送信を受け付けました</h2>
          <p>
            {result.share_mode === "recipient_restricted"
              ? `${result.recipients.length}名の受信者へ通知をキュー登録しました。`
              : "共有URLを発行しました。必要な相手へ安全な方法でお知らせください。"}
          </p>

          {result.access_url && (
            <div className={styles.urlBox}>
              <label htmlFor="access-url">共有URL</label>
              <div>
                <input id="access-url" readOnly value={result.access_url} />
                <button
                  className={styles.primaryButton}
                  type="button"
                  onClick={() => void copyAccessUrl()}
                >
                  コピー
                </button>
              </div>
            </div>
          )}

          <dl className={styles.resultDetails}>
            <div><dt>Shipment ID</dt><dd>{result.id}</dd></div>
            <div><dt>有効期限</dt><dd>{formatDate(result.expires_at)}</dd></div>
            <div><dt>最大DL回数</dt><dd>{result.max_download_count}回</dd></div>
          </dl>

          <div className={styles.actions}>
            <Link className={styles.secondaryButton} href={`/shipments`}>
              送信履歴を確認
            </Link>
            <button
              className={styles.primaryButton}
              type="button"
              onClick={resetWizard}
            >
              続けて送信
            </button>
          </div>
        </div>
      )}
    </section>
  );
}

function StepIndicator({ step }: { step: WizardStep }) {
  const currentIndex = { files: 0, uploading: 1, settings: 2, complete: 3 }[step];
  const labels = ["ファイル選択", "アップロード", "送信設定", "完了"];
  return (
    <ol className={styles.steps} aria-label="送信手順">
      {labels.map((label, index) => (
        <li key={label} data-current={index === currentIndex} data-completed={index < currentIndex}>
          <span>{index < currentIndex ? "✓" : index + 1}</span>
          <strong>{label}</strong>
        </li>
      ))}
    </ol>
  );
}

function Progress({ value, label }: { value: number; label: string }) {
  const percent = Math.max(0, Math.min(100, Math.round(value * 100)));
  return (
    <div
      className={styles.progressTrack}
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={percent}
    >
      <span style={{ width: `${percent}%` }} />
    </div>
  );
}

function taskProgress(task: UploadTask) {
  switch (task.status) {
    case "hashing":
      return task.hashProgress * 0.15;
    case "uploading":
      return 0.15 + task.uploadProgress * 0.8;
    case "completing":
      return 0.97;
    case "completed":
      return 1;
    default:
      return 0;
  }
}

function mergeCompletedParts(
  current: CompletedUploadPart[],
  incoming: CompletedUploadPart,
) {
  return [...current.filter((part) => part.part_number !== incoming.part_number), incoming].sort(
    (a, b) => a.part_number - b.part_number,
  );
}

function parseRecipients(value: string) {
  return Array.from(
    new Set(
      value
        .split(/[\n,]/)
        .map((email) => email.trim().toLowerCase())
        .filter(Boolean),
    ),
  );
}

function validateSettings(input: {
  subject: string;
  message: string;
  shareMode: "url_shared" | "recipient_restricted";
  recipients: string[];
  expiresInDays: number;
  maxDownloads: number;
  password: string;
}) {
  if (!input.subject.trim()) return "件名を入力してください。";
  if (input.subject.length > 200) return "件名は200文字以内で入力してください。";
  if (input.message.length > 5000) return "メッセージは5000文字以内で入力してください。";
  if (input.shareMode === "recipient_restricted") {
    if (input.recipients.length === 0) return "受信者を1件以上入力してください。";
    if (input.recipients.length > 20) return "受信者は20件までです。";
    if (input.recipients.some((email) => !isEmail(email))) {
      return "受信者メールアドレスの形式を確認してください。";
    }
  }
  if (!Number.isInteger(input.expiresInDays) || input.expiresInDays < 1 || input.expiresInDays > 14) {
    return "有効日数は1日から14日の範囲で指定してください。";
  }
  if (!Number.isInteger(input.maxDownloads) || input.maxDownloads < 1 || input.maxDownloads > 100) {
    return "最大ダウンロード回数は1回から100回の範囲で指定してください。";
  }
  if (input.password && input.password.length < 8) {
    return "パスワードを設定する場合は8文字以上必要です。";
  }
  return null;
}

function buildExpiry(days: number) {
  const safetyMargin = days < 14 ? 60_000 : 0;
  return new Date(Date.now() + days * 24 * 60 * 60 * 1000 + safetyMargin).toISOString();
}

function isEmail(value: string) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value) && value.length <= 320;
}

function fileKey(file: File) {
  return `${file.name}:${file.size}:${file.lastModified}`;
}

function fileExtension(fileName: string) {
  const extension = fileName.split(".").pop();
  return extension && extension !== fileName ? extension.slice(0, 4).toUpperCase() : "FILE";
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

function formatDate(value: string) {
  return new Intl.DateTimeFormat("ja-JP", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function errorMessage(caught: unknown) {
  if (caught instanceof ApiClientError) return caught.message;
  if (caught instanceof Error) return caught.message;
  return "処理に失敗しました。通信状態を確認して再試行してください。";
}
