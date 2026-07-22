import type { PresignedUploadPart } from "@/lib/types";

export type CompletedUploadPart = {
  part_number: number;
  etag: string;
};

type UploadMultipartOptions = {
  file: File;
  partSize: number;
  parts: PresignedUploadPart[];
  completedParts?: CompletedUploadPart[];
  concurrency?: number;
  maxAttempts?: number;
  onProgress?: (progress: number) => void;
  onPartCompleted?: (part: CompletedUploadPart) => void;
};

/**
 * Presigned URLへファイルを直接PUTする。
 * 同一セッション内の完了済みパートは再送せず、失敗したパートだけ再試行できる。
 */
export async function uploadMultipartFile({
  file,
  partSize,
  parts,
  completedParts = [],
  concurrency = 3,
  maxAttempts = 3,
  onProgress,
  onPartCompleted,
}: UploadMultipartOptions): Promise<CompletedUploadPart[]> {
  const completed = new Map(completedParts.map((part) => [part.part_number, part.etag]));
  const loadedByPart = new Map<number, number>();

  for (const part of parts) {
    if (completed.has(part.part_number)) {
      loadedByPart.set(part.part_number, partByteLength(file, partSize, part.part_number));
    }
  }

  const reportProgress = () => {
    const loaded = Array.from(loadedByPart.values()).reduce((sum, value) => sum + value, 0);
    onProgress?.(file.size === 0 ? 1 : Math.min(1, loaded / file.size));
  };
  reportProgress();

  const pending = parts.filter((part) => !completed.has(part.part_number));
  let cursor = 0;

  async function worker() {
    while (cursor < pending.length) {
      const part = pending[cursor];
      cursor += 1;
      const start = (part.part_number - 1) * partSize;
      const end = Math.min(start + partSize, file.size);
      const blob = file.slice(start, end);

      let lastError: unknown;
      for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
        loadedByPart.set(part.part_number, 0);
        reportProgress();
        try {
          const etag = await putPart(part.presigned_url, blob, (loaded) => {
            loadedByPart.set(part.part_number, loaded);
            reportProgress();
          });
          const completedPart = { part_number: part.part_number, etag };
          completed.set(part.part_number, etag);
          loadedByPart.set(part.part_number, blob.size);
          reportProgress();
          onPartCompleted?.(completedPart);
          lastError = undefined;
          break;
        } catch (error) {
          lastError = error;
          if (attempt < maxAttempts) {
            await delay(400 * 2 ** (attempt - 1));
          }
        }
      }

      if (lastError) {
        throw lastError;
      }
    }
  }

  const workerCount = Math.max(1, Math.min(concurrency, pending.length || 1));
  await Promise.all(Array.from({ length: workerCount }, () => worker()));

  return Array.from(completed.entries())
    .map(([part_number, etag]) => ({ part_number, etag }))
    .sort((a, b) => a.part_number - b.part_number);
}

function putPart(
  url: string,
  blob: Blob,
  onProgress: (loaded: number) => void,
): Promise<string> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open("PUT", url);
    request.upload.addEventListener("progress", (event) => {
      if (event.lengthComputable) {
        onProgress(event.loaded);
      }
    });
    request.addEventListener("load", () => {
      if (request.status < 200 || request.status >= 300) {
        reject(new Error(`S3へのパート送信に失敗しました（HTTP ${request.status}）`));
        return;
      }
      const etag = request.getResponseHeader("ETag");
      if (!etag) {
        reject(
          new Error(
            "S3レスポンスのETagを取得できません。Bucket CORSのExposeHeadersにETagを追加してください。",
          ),
        );
        return;
      }
      resolve(etag);
    });
    request.addEventListener("error", () => {
      reject(new Error("S3への接続に失敗しました。ネットワークまたはBucket CORSを確認してください。"));
    });
    request.addEventListener("abort", () => reject(new Error("アップロードが中断されました。")));
    request.send(blob);
  });
}

function partByteLength(file: File, partSize: number, partNumber: number) {
  const start = (partNumber - 1) * partSize;
  return Math.max(0, Math.min(partSize, file.size - start));
}

function delay(milliseconds: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, milliseconds));
}
