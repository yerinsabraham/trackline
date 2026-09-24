"use client";

import { useEffect, useRef, useState } from "react";
import { api, GOOGLE_CLIENT_ID, session, setSession, startGithub, takeNext } from "@/lib/account";

type GoogleId = {
  accounts: { id: {
    initialize: (o: { client_id: string; callback: (r: { credential: string }) => void }) => void;
    renderButton: (el: HTMLElement, o: Record<string, unknown>) => void;
  } };
};

export default function SignIn() {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const googleEl = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (session()) window.location.replace(takeNext());
  }, []);

  useEffect(() => {
    // Google's own button, from Google's script: the ID token it returns is
    // checked by the API against trackline's client ID.
    const s = document.createElement("script");
    s.src = "https://accounts.google.com/gsi/client";
    s.async = true;
    s.onload = () => {
      const g = (window as unknown as { google?: GoogleId }).google;
      if (!g || !googleEl.current) return;
      g.accounts.id.initialize({
        client_id: GOOGLE_CLIENT_ID,
        callback: async ({ credential }) => {
          setBusy(true);
          setError("");
          try {
            const r = await api<{ token: string }>("/auth/google", { method: "POST", body: JSON.stringify({ credential }) });
            setSession(r.token);
            window.location.replace(takeNext());
          } catch (e) {
            setError((e as Error).message);
            setBusy(false);
          }
        },
      });
      // Google caps the button at 400px; match the GitHub button's width.
      const width = Math.min(400, googleEl.current.offsetWidth || 320);
      g.accounts.id.renderButton(googleEl.current, {
        theme: "outline", size: "large", width, text: "continue_with", locale: "en", logo_alignment: "center",
      });
    };
    document.head.appendChild(s);
    return () => { s.remove(); };
  }, []);

  return (
    <div className="wrap account-page">
      <div className="account-card">
        <p className="eyebrow">trackline account</p>
        <h1>Sign in</h1>
        <p className="account-sub">
          An account is optional. It adds a live dashboard for your agents, on any device. trackline works fully without one.
        </p>
        <div className="account-actions">
          <button
            className="btn btn-dark"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              setError("");
              try { await startGithub(); } catch (e) { setError((e as Error).message); setBusy(false); }
            }}
          >
            <svg width="18" height="18" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8a8 8 0 0 0 5.47 7.59c.4.07.55-.17.55-.38v-1.33c-2.23.48-2.7-1.07-2.7-1.07-.36-.92-.89-1.17-.89-1.17-.73-.5.05-.49.05-.49.81.06 1.23.83 1.23.83.72 1.23 1.88.87 2.34.67.07-.52.28-.87.51-1.07-1.78-.2-3.65-.89-3.65-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 0 1 4 0c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48v2.2c0 .21.15.46.55.38A8 8 0 0 0 16 8c0-4.42-3.58-8-8-8Z" /></svg>
            Continue with GitHub
          </button>
          <div ref={googleEl} className="google-slot" />
        </div>
        {error && <p className="account-error" role="alert">{error}</p>}
        <p className="account-fine">
          trackline reads your name and email from GitHub or Google. Never your repositories.{" "}
          <a href="/privacy">Privacy</a>
        </p>
      </div>
    </div>
  );
}
