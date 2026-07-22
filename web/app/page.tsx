import Link from "next/link";

const features = [
  {
    title: "大容量でも止まりにくい",
    description: "S3 multipart upload を前提に、ファイルを分割して直接アップロードします。",
  },
  {
    title: "受信者と期限を制御",
    description: "URL共有と受信者限定共有、期限、パスワード、ダウンロード回数を設定できます。",
  },
  {
    title: "送信後も追跡できる",
    description: "通知履歴、受領状況、再送、論理削除を一つの送信履歴から管理できます。",
  },
];

export default function HomePage() {
  return (
    <>
      <section className="hero">
        <div className="shell hero-grid">
          <div>
            <p className="eyebrow">Secure file delivery</p>
            <h1>大きなファイルを、必要な相手にだけ。</h1>
            <p className="hero-copy">
              VaultSend は、期限・回数・受信者を制御しながら大容量ファイルを届けるための
              ファイル送信サービスです。送信後の通知とダウンロード状況も確認できます。
            </p>
            <div className="hero-actions">
              <Link className="button" href="/send">
                ファイルを送る
              </Link>
              <Link className="button button-secondary" href="/shipments">
                送信履歴を見る
              </Link>
            </div>
          </div>

          <div className="hero-panel" aria-label="ファイル送信イメージ">
            <div className="hero-panel-header">
              <div>
                <strong>契約書類を送信中</strong>
                <p>受信者限定・7日間有効</p>
              </div>
              <span className="status-badge" data-status="sent">安全に転送</span>
            </div>
            <div className="hero-file">
              <span className="file-icon" aria-hidden="true">PDF</span>
              <div>
                <strong>agreement-2026.pdf</strong>
                <span>1.8 GB・8パート</span>
              </div>
              <strong>72%</strong>
            </div>
            <div className="progress-track" aria-label="アップロード進捗 72%">
              <div className="progress-value" />
            </div>
            <ul className="detail-list" aria-label="送信設定">
              <li><span className="detail-key">受信者</span><strong>2名</strong></li>
              <li><span className="detail-key">有効期限</span><strong>7日</strong></li>
              <li><span className="detail-key">回数制限</span><strong>10回</strong></li>
            </ul>
          </div>
        </div>
      </section>

      <section className="shell feature-grid" aria-label="VaultSendの特徴">
        {features.map((feature) => (
          <article className="feature-item" key={feature.title}>
            <h2>{feature.title}</h2>
            <p>{feature.description}</p>
          </article>
        ))}
      </section>
    </>
  );
}
