"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";
import { api, ApiClientError } from "@/lib/api";

type Mode = "login" | "register";

export function AuthPanel() {
  const router = useRouter();
  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIsSubmitting(true);

    try {
      if (mode === "login") {
        await api.login({ email, password });
      } else {
        await api.register({
          email,
          password,
          display_name: displayName.trim() || undefined,
        });
      }
      router.push("/shipments");
      router.refresh();
    } catch (caught) {
      if (caught instanceof ApiClientError) {
        setError(caught.message);
      } else {
        setError("通信に失敗しました。APIサーバーの起動状態を確認してください。");
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  function changeMode(nextMode: Mode) {
    setMode(nextMode);
    setError(null);
  }

  return (
    <section className="auth-card" aria-labelledby="auth-title">
      <h1 id="auth-title">{mode === "login" ? "おかえりなさい" : "VaultSendを始める"}</h1>
      <p>
        {mode === "login"
          ? "登録済みのメールアドレスでログインしてください。"
          : "アカウントを作成すると、送信履歴と受領状況を管理できます。"}
      </p>

      <div className="segmented" role="tablist" aria-label="認証方法">
        <button
          type="button"
          role="tab"
          aria-selected={mode === "login"}
          onClick={() => changeMode("login")}
        >
          ログイン
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === "register"}
          onClick={() => changeMode("register")}
        >
          新規登録
        </button>
      </div>

      <form className="form-stack" onSubmit={handleSubmit}>
        {mode === "register" && (
          <div className="field">
            <label htmlFor="display-name">表示名</label>
            <input
              id="display-name"
              name="display_name"
              type="text"
              maxLength={80}
              autoComplete="name"
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              placeholder="山田 太郎"
            />
          </div>
        )}

        <div className="field">
          <label htmlFor="email">メールアドレス</label>
          <input
            id="email"
            name="email"
            type="email"
            maxLength={320}
            autoComplete="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder="you@example.com"
          />
        </div>

        <div className="field">
          <label htmlFor="password">パスワード</label>
          <input
            id="password"
            name="password"
            type="password"
            minLength={8}
            autoComplete={mode === "login" ? "current-password" : "new-password"}
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            placeholder="8文字以上"
          />
        </div>
        <p className="form-help">パスワードは8文字以上で入力してください。</p>

        {error && (
          <p className="alert alert-error" role="alert">
            {error}
          </p>
        )}

        <button className="button" type="submit" disabled={isSubmitting}>
          {isSubmitting
            ? "処理中…"
            : mode === "login"
              ? "ログイン"
              : "アカウントを作成"}
        </button>
      </form>
    </section>
  );
}
