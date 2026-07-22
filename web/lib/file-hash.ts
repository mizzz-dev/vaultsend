import { sha256 } from "@noble/hashes/sha2.js";
import { bytesToHex } from "@noble/hashes/utils.js";

const HASH_CHUNK_SIZE = 4 * 1024 * 1024;

/**
 * ファイル全体をArrayBufferへ展開せず、一定サイズずつ読み込んでSHA-256を計算する。
 * 10GB級ファイルでもブラウザのメモリ使用量がファイルサイズに比例しないようにする。
 */
export async function calculateFileSha256(
  file: File,
  onProgress?: (progress: number) => void,
): Promise<string> {
  const hash = sha256.create();
  let offset = 0;
  let chunkIndex = 0;

  while (offset < file.size) {
    const end = Math.min(offset + HASH_CHUNK_SIZE, file.size);
    const chunk = new Uint8Array(await file.slice(offset, end).arrayBuffer());
    hash.update(chunk);
    offset = end;
    chunkIndex += 1;
    onProgress?.(file.size === 0 ? 1 : offset / file.size);

    // 長時間の同期計算でUI更新が止まらないよう、一定間隔でイベントループへ制御を返す。
    if (chunkIndex % 8 === 0) {
      await new Promise<void>((resolve) => window.setTimeout(resolve, 0));
    }
  }

  return bytesToHex(hash.digest());
}
