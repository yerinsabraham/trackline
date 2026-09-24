"use client";

import { useEffect, useState } from "react";
import { api, clearSession, session, type Device, type User } from "@/lib/account";

export default function Account() {
  const [user, setUser] = useState<User | null>(null);
  const [error, setError] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [devices, setDevices] = useState<Device[] | null>(null);

  const loadDevices = () =>
    api<{ devices: Device[] }>("/devices").then((r) => setDevices(r.devices)).catch(() => setDevices([]));

  useEffect(() => {
    if (!session()) return window.location.replace("/signin");
    api<{ user: User }>("/auth/me")
      .then((r) => { setUser(r.user); loadDevices(); })
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
                      onClick={async () => { await api(`/devices/${d.id}`, { method: "DELETE" }); loadDevices(); }}
                    >
                      Disconnect
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <div className="account-actions">
              <a className="btn btn-signal" href="/app">Dashboard</a>
              <button className="btn btn-quiet" onClick={signOut}>Sign out</button>
              <button
                className="btn btn-quiet"
                onClick={async () => { await api("/auth/signout-everywhere", { method: "POST", body: "{}" }); signOut(); }}
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
                    onClick={async () => { await api("/auth/me", { method: "DELETE" }); signOut(); }}
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
