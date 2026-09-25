"use client";

import { useEffect, useState } from "react";
import { api, clearSession, session, type Device, type User } from "@/lib/account";
import { addPasskey, withStepUp } from "@/lib/security";

type Security = {
  factors: {
    passkeys: { id: string; name: string; createdAt: string; lastUsedAt: string | null }[];
    totp: boolean;
    recoveryCodesRemaining: number;
  };
  audit: { id: string; kind: string; ip: string | null; userAgent: string | null; createdAt: string }[];
};

export default function Account() {
  const [user, setUser] = useState<User | null>(null);
  const [error, setError] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [devices, setDevices] = useState<Device[] | null>(null);
  const [security, setSecurity] = useState<Security | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [totp, setTotp] = useState<{ secret: string; otpauth: string } | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");

  const loadDevices = () =>
    api<{ devices: Device[] }>("/devices").then((r) => setDevices(r.devices)).catch(() => setDevices([]));
  const loadSecurity = () =>
    api<Security>("/auth/security").then(setSecurity).catch(() => setSecurity(null));

  const act = async (fn: () => Promise<void>) => {
    setBusy(true); setError(""); setNote("");
    try { await fn(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };

  useEffect(() => {
    if (!session()) return window.location.replace("/signin");
    api<{ user: User }>("/auth/me")
      .then((r) => { setUser(r.user); loadDevices(); loadSecurity(); })
      .catch((e) => {
        if ((e as { status?: number }).status === 401) { clearSession(); window.location.replace("/signin"); }
        else setError((e as Error).message);
      });
  }, []);

  const signOut = () => { clearSession(); window.location.replace("/signin"); };

  return (
    <div className="wrap account-page">
      <div className="account-card">
        <p className="eyebrow">trackline account</p>
        {!user && !error && <h1>Loading…</h1>}
        {error && <p className="account-error" role="alert">{error}</p>}
        {user && (
          <>
            <div className="account-who">
              {user.avatarUrl && (
                // Google's photo host refuses requests that carry a referrer.
                <img
                  src={user.avatarUrl}
                  alt=""
                  width={56}
                  height={56}
                  referrerPolicy="no-referrer"
                  onError={(e) => { e.currentTarget.style.display = 'none'; }}
                />
              )}
              <div>
                <h1>{user.name ?? "Signed in"}</h1>
                {user.email && <p className="account-sub">{user.email}</p>}
              </div>
            </div>
            <p className="account-sub">
              Signed in with {user.signInMethods.map((m) => (m === "github" ? "GitHub" : "Google")).join(" and ")}.
            </p>
            <h2 className="account-h2">Connected machines</h2>
            {devices === null && <p className="account-sub">Loading…</p>}
            {devices?.length === 0 && (
              <p className="account-sub">None yet. Run <code>trackline connect</code> in a project to link a machine.</p>
            )}
            {devices && devices.length > 0 && (
              <ul className="device-list">
                {devices.map((d) => (
                  <li key={d.id}>
                    <div>
                      <strong>{d.name}</strong>
                      <span>
                        Connected {new Date(d.createdAt).toLocaleDateString()}
                        {d.lastUsedAt ? ` · last seen ${new Date(d.lastUsedAt).toLocaleString()}` : ""}
                      </span>
                    </div>
                    <button
                      className="link-danger"
                      disabled={busy}
                      onClick={() => act(async () => { await withStepUp(() => api(`/devices/${d.id}`, { method: "DELETE" })); loadDevices(); })}
                    >
                      Disconnect
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <h2 className="account-h2">Security</h2>
            {!security && <p className="account-sub">Loading security settings…</p>}
            {security && (
              <>
                <p className="account-sub">
                  Passkeys: {security.factors.passkeys.length} · Authenticator app: {security.factors.totp ? "on" : "off"} · Recovery codes: {security.factors.recoveryCodesRemaining}
                </p>
                <div className="account-actions">
                  <button
                    className="btn btn-quiet"
                    disabled={busy}
                    onClick={() => act(async () => {
                      const name = window.prompt("Name this passkey", "My passkey") || "Passkey";
                      await withStepUp(() => addPasskey(name));
                      setNote("Passkey added.");
                      await loadSecurity();
                    })}
                  >
                    Add a passkey
                  </button>
                  <button
                    className="btn btn-quiet"
                    disabled={busy}
                    onClick={() => act(async () => {
                      setTotp(await withStepUp(() => api<{ secret: string; otpauth: string }>("/auth/totp/start", { method: "POST", body: "{}" })));
                    })}
                  >
                    Use an authenticator app
                  </button>
                  <button
                    className="btn btn-quiet"
                    disabled={busy}
                    onClick={() => act(async () => {
                      const r = await withStepUp(() => api<{ codes: string[] }>("/auth/recovery-codes", { method: "POST", body: "{}" }));
                      setRecoveryCodes(r.codes);
                      await loadSecurity();
                    })}
                  >
                    Recovery codes
                  </button>
                </div>
                {security.factors.passkeys.length > 0 && (
                  <ul className="device-list">
                    {security.factors.passkeys.map((p) => (
                      <li key={p.id}>
                        <div><strong>{p.name}</strong><span>Added {new Date(p.createdAt).toLocaleDateString()}{p.lastUsedAt ? ` · last used ${new Date(p.lastUsedAt).toLocaleString()}` : ""}</span></div>
                        <button className="link-danger" disabled={busy} onClick={() => act(async () => {
                          await withStepUp(() => api(`/auth/passkeys/${p.id}`, { method: "DELETE" }));
                          await loadSecurity();
                        })}>Remove</button>
                      </li>
                    ))}
                  </ul>
                )}
                {totp && (
                  <div className="account-panel">
                    <p className="account-sub">Add this secret to your authenticator app, then enter the code it shows.</p>
                    <code>{totp.secret}</code>
                    <input className="account-input" value={totpCode} onChange={(e) => setTotpCode(e.target.value)} placeholder="123456" />
                    <button className="btn btn-signal" disabled={busy || !totpCode} onClick={() => act(async () => {
                      await api("/auth/totp/verify", { method: "POST", body: JSON.stringify({ code: totpCode }) });
                      setTotp(null); setTotpCode(""); setNote("Authenticator app added."); await loadSecurity();
                    })}>Verify code</button>
                  </div>
                )}
                {recoveryCodes.length > 0 && (
                  <div className="account-panel">
                    <p className="account-sub">Save these now. Trackline will not show them again.</p>
                    <pre>{recoveryCodes.join("\n")}</pre>
                  </div>
                )}
                {note && <p className="account-note">{note}</p>}
                <h2 className="account-h2">Recent security activity</h2>
                {security.audit.length === 0 && <p className="account-sub">No security activity yet.</p>}
                {security.audit.length > 0 && (
                  <ul className="device-list">
                    {security.audit.slice(0, 8).map((a) => (
                      <li key={a.id}><div><strong>{a.kind.replace(/_/g, " ")}</strong><span>{new Date(a.createdAt).toLocaleString()}{a.ip ? ` · ${a.ip}` : ""}</span></div></li>
                    ))}
                  </ul>
                )}
              </>
            )}
            <div className="account-actions">
              <a className="btn btn-signal" href="/app">Dashboard</a>
              <button className="btn btn-quiet" onClick={signOut}>Sign out</button>
              <button
                className="btn btn-quiet"
                disabled={busy}
                onClick={() => act(async () => { await withStepUp(() => api("/auth/signout-everywhere", { method: "POST", body: "{}" })); signOut(); })}
              >
                Sign out everywhere
              </button>
            </div>
            <div className="account-danger">
              {!confirming ? (
                <button className="link-danger" onClick={() => setConfirming(true)}>Delete account</button>
              ) : (
                <>
                  <p>This deletes your account and everything trackline holds for it. It cannot be undone.</p>
                  <button
                    className="btn btn-danger"
                    onClick={() => act(async () => { await withStepUp(() => api("/auth/me", { method: "DELETE" })); signOut(); })}
                  >
                    Delete my account
                  </button>{" "}
                  <button className="link-quiet" onClick={() => setConfirming(false)}>Keep it</button>
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
