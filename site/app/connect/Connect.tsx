"use client";

import { useEffect, useState } from "react";
import { api, clearSession, rememberNext, session } from "@/lib/account";

type Pending = { deviceName: string; expiresAt: string };

export default function Connect() {
  const [code, setCode] = useState("");
  const [pending, setPending] = useState<Pending | null>(null);
  const [state, setState] = useState<"entering" | "checking" | "confirm" | "approved" | "declined">("entering");
  const [error, setError] = useState("");

  const look = async (c: string) => {
    setError("");
    setState("checking");
    try {
      setPending(await api<Pending>(`/devices/code/${encodeURIComponent(c)}`));
      setState("confirm");
    } catch (e) {
      if ((e as { status?: number }).status === 401) {
        clearSession();
        rememberNext(`/connect?code=${encodeURIComponent(c)}`);
        return window.location.replace("/signin");
      }
      setError((e as Error).message);
      setState("entering");
    }
  };

  useEffect(() => {
    const c = new URLSearchParams(window.location.search).get("code") ?? "";
    setCode(c);
    if (!session()) {
      rememberNext(`/connect${c ? `?code=${encodeURIComponent(c)}` : ""}`);
      window.location.replace("/signin");
      return;
    }
    if (c) look(c);
  }, []);

  const decide = async (approve: boolean) => {
    setError("");
    try {
      await api(`/devices/code/${encodeURIComponent(code)}/${approve ? "approve" : "deny"}`, { method: "POST", body: "{}" });
      setState(approve ? "approved" : "declined");
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <div className="wrap account-page">
      <div className="account-card">
        <p className="eyebrow">trackline account</p>

        {(state === "entering" || state === "checking") && (
          <>
            <h1>Connect a machine</h1>
            <p className="account-sub">Enter the code <code>trackline connect</code> showed in your terminal.</p>
            <form
              className="connect-form"
              onSubmit={(e) => { e.preventDefault(); if (code.trim()) look(code.trim()); }}
            >
              <input
                className="code-input"
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
                placeholder="BCDF-GHJK"
                autoComplete="off"
                spellCheck={false}
                aria-label="Code"
              />
              <button className="btn btn-dark" type="submit" disabled={state === "checking"}>Continue</button>
            </form>
          </>
        )}

        {state === "confirm" && pending && (
          <>
            <h1>Connect this machine?</h1>
            <div className="connect-device">
              <span className="connect-name">{pending.deviceName}</span>
              <span className="connect-code">{code.toUpperCase()}</span>
            </div>
            <p className="account-sub">
              Only approve if <strong>you</strong> just ran <code>trackline connect</code> on this machine, and the code
              matches the one in your terminal. It will be able to send what trackline notices to your account.
            </p>
            <div className="account-actions">
              <button className="btn btn-signal" onClick={() => decide(true)}>Approve</button>
              <button className="btn btn-quiet" onClick={() => decide(false)}>Decline</button>
            </div>
          </>
        )}

        {state === "approved" && (
          <>
            <h1>Connected</h1>
            <p className="account-sub">Go back to your terminal. It finishes on its own in a few seconds.</p>
            <a className="btn btn-quiet" href="/account">See your machines</a>
          </>
        )}

        {state === "declined" && (
          <>
            <h1>Declined</h1>
            <p className="account-sub">That machine was not connected.</p>
          </>
        )}

        {error && <p className="account-error" role="alert">{error}</p>}
      </div>
    </div>
  );
}
