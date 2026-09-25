"use client";

import { useEffect, useState } from "react";
import { api, type Device } from "@/lib/account";
import { ago } from "@/lib/dashboard";
import { addPasskey, withStepUp } from "@/lib/security";
import { Avatar, signOut, useApp } from "@/components/app/AppShell";
import { IconKey, IconLaptop, IconPhone, IconSignOut } from "@/components/app/icons";

// Who you are, how you prove it, and the ways out: sign out here, sign out
// everywhere, or delete everything.

type Security = {
  factors: {
    passkeys: { id: string; name: string; createdAt: string; lastUsedAt: string | null }[];
    totp: boolean;
    recoveryCodesRemaining: number;
  };
  audit: { id: string; kind: string; ip: string | null; userAgent: string | null; createdAt: string }[];
};

type Tab = "security" | "machines" | "activity";

export default function Account() {
  const { user } = useApp();
  const [tab, setTab] = useState<Tab>("security");
  const [devices, setDevices] = useState<Device[] | null>(null);
  const [security, setSecurity] = useState<Security | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [totp, setTotp] = useState<{ secret: string; otpauth: string } | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [error, setError] = useState("");

  const loadDevices = () => api<{ devices: Device[] }>("/devices").then((r) => setDevices(r.devices)).catch(() => setDevices([]));
  const loadSecurity = () => api<Security>("/auth/security").then(setSecurity).catch(() => setSecurity(null));
  useEffect(() => { loadDevices(); loadSecurity(); }, []);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true); setError(""); setNote("");
    try { await fn(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };

  const methods = user?.signInMethods.map((m) => (m === "github" ? "GitHub" : "Google")).join(" and ");
  const phoneish = (name: string) => /phone|android|pixel|galaxy/i.test(name);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 22, maxWidth: 1080 }}>
      <header className="acct-head rise">
        <Avatar user={user} size={72} />
        <div style={{ display: "grid", gap: 4, minWidth: 0 }}>
          <h1 className="app-h1">{user?.name ?? (user ? "Signed in" : "Loading…")}</h1>
          <span className="app-muted" style={{ fontSize: 14 }}>{user?.email}{methods ? ` · Signed in with ${methods}` : ""}</span>
        </div>
        <button className="btn-plain acct-signout" onClick={signOut}><span style={{ width: 16, display: "flex" }}><IconSignOut /></span>Sign out</button>
      </header>

      <div className="acct-tabs rise rise-1" role="tablist" aria-label="Account">
        {([["security", "Security"], ["machines", "Machines"], ["activity", "Activity"]] as [Tab, string][]).map(([t, label]) => (
          <button key={t} role="tab" aria-selected={tab === t} onClick={() => setTab(t)}>{label}</button>
        ))}
      </div>

      {error && <p className="composer-note error" style={{ textAlign: "left" }} role="alert">{error}</p>}
      {note && <p className="composer-note" style={{ textAlign: "left", color: "var(--ok-ink)" }}>{note}</p>}

      {tab === "security" && (
        <div className="acct-grid rise rise-2">
          <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
            <section className="card" aria-labelledby="pk">
              <div className="card-head">
                <div style={{ display: "grid", gap: 2 }}>
                  <h2 id="pk">Passkeys</h2>
                  <span className="app-muted" style={{ fontSize: 13 }}>Face ID or Touch ID. Needed to send tasks and change security settings.</span>
                </div>
                <button className="btn-act" style={{ marginLeft: "auto", height: 36, flex: "none" }} disabled={busy} onClick={() => act(async () => {
                  const name = window.prompt("Name this passkey", /iPhone|Android/i.test(navigator.userAgent) ? "Phone" : "Laptop") || "Passkey";
                  await withStepUp(() => addPasskey(name));
                  setNote("Passkey added.");
                  await loadSecurity();
                })}>Add a passkey</button>
              </div>
              {!security && <div className="card-body"><div className="skel" style={{ height: 44 }} /></div>}
              {security?.factors.passkeys.length === 0 && <div className="empty">No passkey yet. Add one to send tasks from this app.</div>}
              {security?.factors.passkeys.map((p) => (
                <div key={p.id} className="row-link" style={{ alignItems: "center" }}>
                  <span className="agent-logo" style={{ width: 38, height: 38 }}><span style={{ width: 18, display: "flex" }}>{phoneish(p.name) ? <IconPhone /> : <IconKey />}</span></span>
                  <span className="row-main">
                    <strong style={{ fontSize: 14 }}>{p.name}</strong>
                    <span className="app-muted" style={{ fontSize: 13 }}>Added {new Date(p.createdAt).toLocaleDateString()}{p.lastUsedAt ? ` · last used ${ago(p.lastUsedAt)}` : ""}</span>
                  </span>
                  <button className="btn-plain" style={{ height: 32, color: "var(--red-ink)" }} disabled={busy} onClick={() => act(async () => {
                    await withStepUp(() => api(`/auth/passkeys/${p.id}`, { method: "DELETE" }));
                    await loadSecurity();
                  })}>Remove</button>
                </div>
              ))}
            </section>

            <section className="card" aria-labelledby="backup">
              <div className="card-head"><h2 id="backup">If you lose your phone</h2></div>
              <div className="row-link" style={{ alignItems: "center" }}>
                <span className="row-main">
                  <strong style={{ fontSize: 14 }}>Authenticator app</strong>
                  <span className="app-muted" style={{ fontSize: 13 }}>Six-digit codes from an app such as 1Password.</span>
                </span>
                <span className="app-muted" style={{ fontSize: 13 }}>{security?.factors.totp ? "On" : "Off"}</span>
                <button className="btn-plain" style={{ height: 32 }} disabled={busy} onClick={() => act(async () => {
                  setTotp(await withStepUp(() => api<{ secret: string; otpauth: string }>("/auth/totp/start", { method: "POST", body: "{}" })));
                })}>{security?.factors.totp ? "Replace" : "Set up"}</button>
              </div>
              {totp && (
                <div className="card-body" style={{ background: "#fafaf8" }}>
                  <span className="app-muted" style={{ fontSize: 13 }}>Add this secret to your authenticator app, then enter the code it shows.</span>
                  <code className="mono" style={{ fontSize: 13, overflowWrap: "anywhere" }}>{totp.secret}</code>
                  <div style={{ display: "flex", gap: 8 }}>
                    <input className="account-input" inputMode="numeric" autoComplete="one-time-code" aria-label="Six-digit code" value={totpCode} onChange={(e) => setTotpCode(e.target.value)} placeholder="123456" />
                    <button className="btn-act" disabled={busy || !totpCode} onClick={() => act(async () => {
                      await api("/auth/totp/verify", { method: "POST", body: JSON.stringify({ code: totpCode }) });
                      setTotp(null); setTotpCode(""); setNote("Authenticator app added."); await loadSecurity();
                    })}>Verify</button>
                  </div>
                </div>
              )}
              <div className="row-link" style={{ alignItems: "center" }}>
                <span className="row-main">
                  <strong style={{ fontSize: 14 }}>Recovery codes</strong>
                  <span className="app-muted" style={{ fontSize: 13 }}>Ten one-time codes. Keep them somewhere safe.</span>
                </span>
                <span style={{ fontSize: 13, color: security?.factors.recoveryCodesRemaining ? "var(--ok-ink)" : "var(--muted)" }}>{security ? `${security.factors.recoveryCodesRemaining} left` : ""}</span>
                <button className="btn-plain" style={{ height: 32 }} disabled={busy} onClick={() => act(async () => {
                  const r = await withStepUp(() => api<{ codes: string[] }>("/auth/recovery-codes", { method: "POST", body: "{}" }));
                  setRecoveryCodes(r.codes);
                  await loadSecurity();
                })}>Make new codes</button>
              </div>
              {recoveryCodes.length > 0 && (
                <div className="card-body" style={{ background: "#fafaf8" }}>
                  <span style={{ fontSize: 13, fontWeight: 600 }}>Save these now. trackline will not show them again.</span>
                  <pre className="mono" style={{ margin: 0, fontSize: 13.5, lineHeight: 1.7 }}>{recoveryCodes.join("\n")}</pre>
                </div>
              )}
            </section>
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
            <RecentActivity security={security} limit={5} more={() => setTab("activity")} />
            <section className="card" aria-labelledby="everywhere" style={{ borderColor: "#f3c9bd" }}>
              <div className="card-body">
                <h2 id="everywhere" style={{ margin: 0, fontSize: 16 }}>Everywhere at once</h2>
                <p className="app-muted" style={{ margin: 0, fontSize: 13.5, lineHeight: 1.5 }}>Signs out every browser and phone, and stops remote on every laptop.</p>
                <button className="btn-plain" disabled={busy} onClick={() => act(async () => {
                  await withStepUp(() => api("/auth/signout-everywhere", { method: "POST", body: "{}" }));
                  signOut();
                })}>Sign out everywhere</button>
                {!confirming ? (
                  <button className="btn-danger-text" onClick={() => setConfirming(true)}>Delete my account</button>
                ) : (
                  <div style={{ display: "grid", gap: 8 }}>
                    <p style={{ margin: 0, fontSize: 13.5 }}>This deletes your account and everything trackline holds for it. It cannot be undone.</p>
                    <button className="btn-act" style={{ background: "#b42318" }} disabled={busy} onClick={() => act(async () => {
                      await withStepUp(() => api("/auth/me", { method: "DELETE" }));
                      signOut();
                    })}>Delete my account</button>
                    <button className="btn-plain" onClick={() => setConfirming(false)}>Keep it</button>
                  </div>
                )}
              </div>
            </section>
          </div>
        </div>
      )}

      {tab === "machines" && (
        <section className="card rise" aria-labelledby="connected" style={{ maxWidth: 760 }}>
          <div className="card-head">
            <div style={{ display: "grid", gap: 2 }}>
              <h2 id="connected">Connected machines</h2>
              <span className="app-muted" style={{ fontSize: 13 }}>Linked with <code className="mono">trackline connect</code>. Remote tasks are under <a href="/app/machines" style={{ textDecoration: "underline" }}>Machines</a>.</span>
            </div>
          </div>
          {devices === null && <div className="card-body"><div className="skel" style={{ height: 44 }} /></div>}
          {devices?.length === 0 && <div className="empty">None yet. Run <code>trackline connect</code> in a project.</div>}
          {devices?.map((d) => (
            <div key={d.id} className="row-link" style={{ alignItems: "center" }}>
              <span className="agent-logo" style={{ width: 38, height: 38 }}><span style={{ width: 18, display: "flex" }}><IconLaptop /></span></span>
              <span className="row-main">
                <strong style={{ fontSize: 14 }}>{d.name}</strong>
                <span className="app-muted" style={{ fontSize: 13 }}>Connected {new Date(d.createdAt).toLocaleDateString()}{d.lastUsedAt ? ` · last seen ${ago(d.lastUsedAt)}` : ""}</span>
              </span>
              <button className="btn-plain" style={{ height: 32, color: "var(--red-ink)" }} disabled={busy} onClick={() => act(async () => {
                await withStepUp(() => api(`/devices/${d.id}`, { method: "DELETE" }));
                loadDevices();
              })}>Disconnect</button>
            </div>
          ))}
        </section>
      )}

      {tab === "activity" && <div style={{ maxWidth: 760 }}><RecentActivity security={security} limit={50} /></div>}
    </div>
  );
}

function RecentActivity({ security, limit, more }: { security: Security | null; limit: number; more?: () => void }) {
  return (
    <section className="card" aria-labelledby="recent-activity">
      <div className="card-head"><h2 id="recent-activity">Recent security activity</h2></div>
      <div className="card-body">
        {!security && <div className="skel" style={{ height: 80 }} />}
        {security?.audit.length === 0 && <span className="app-muted" style={{ fontSize: 14 }}>No security activity yet.</span>}
        {security?.audit.slice(0, limit).map((a) => (
          <div key={a.id} style={{ display: "flex", justifyContent: "space-between", gap: 12, fontSize: 13.5 }}>
            <span style={{ textTransform: "capitalize" }}>{a.kind.replace(/[_-]/g, " ")}</span>
            <span className="app-muted" style={{ whiteSpace: "nowrap" }}>{ago(a.createdAt)}</span>
          </div>
        ))}
        {more && security && security.audit.length > limit && <button className="btn-plain" style={{ height: 34 }} onClick={more}>See all activity</button>}
      </div>
    </section>
  );
}
