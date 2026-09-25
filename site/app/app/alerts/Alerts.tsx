"use client";

import { useEffect, useState } from "react";
import { api, clearSession, rememberNext, session } from "@/lib/account";
import { type AlertSettings, current, deviceLabel, subscribe, support, type Support } from "@/lib/alerts";
import "../dashboard.css";

const KINDS: { key: keyof AlertSettings["prefs"]; label: string; note: string }[] = [
  { key: "red", label: "Needs you", note: "An agent touched something off-limits, or was stopped." },
  { key: "finished", label: "Finished", note: "An agent finished what you asked, with the start of its reply." },
  { key: "drifting", label: "Drifting", note: "A warning, such as a new dependency. Off by default: it can be often." },
];

export default function Alerts() {
  const [s, setS] = useState<AlertSettings | null>(null);
  const [can, setCan] = useState<Support>("ok");
  const [mine, setMine] = useState<string | null>(null);
  const [checked, setChecked] = useState(false);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [slack, setSlack] = useState("");

  const load = async () => {
    const r = await api<AlertSettings>("/app/alerts");
    setS(r);
    // Asking the browser for its subscription can take a few seconds. Until
    // it answers, the page must not offer to turn on what may already be on.
    const cur = await current().catch(() => null);
    setMine(cur?.endpoint ?? null);
    setChecked(true);
  };

  useEffect(() => {
    if (!session()) { rememberNext("/app/alerts"); return window.location.replace("/signin"); }
    setCan(support());
    load().catch((e) => {
      if ((e as { status?: number }).status === 401) { clearSession(); rememberNext("/app/alerts"); window.location.replace("/signin"); }
      else setError((e as Error).message);
    });
  }, []);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true); setError(""); setNote("");
    try { await fn(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };

  const thisDevice = s?.devices.find((d) => d.endpoint === mine);

  return (
    <div className="wrap dash">
      <a href="/app" className="dash-back">← All sessions</a>
      <div className="dash-head">
        <p className="eyebrow">Alerts</p>
        <h1>Hear when an agent needs you</h1>
      </div>
      {error && <p className="account-error" role="alert">{error}</p>}
      {!s && !error && <p className="dash-muted">Loading…</p>}

      {s && (
        <>
          <section className="alert-card">
            <h2>This device</h2>
            {can === "ios-home-screen" && (
              <div className="dash-muted">
                <p>On iPhone, alerts reach trackline only from your home screen:</p>
                <ol className="alert-steps">
                  <li>Tap the Share button in Safari.</li>
                  <li>Choose <strong>Add to Home Screen</strong>.</li>
                  <li>Open trackline from the home screen, sign in, and come back to this page.</li>
                </ol>
              </div>
            )}
            {can === "unsupported" && <p className="dash-muted">This browser cannot receive alerts. Try Chrome, Edge or Firefox, or Safari from the home screen on iPhone.</p>}
            {can === "blocked" && <p className="dash-muted">Notifications are blocked for this site. Allow them in your browser&apos;s site settings, then reload.</p>}
            {can === "ok" && !checked && <p className="dash-muted">Checking this device…</p>}
            {can === "ok" && checked && !thisDevice && (
              <button
                className="btn btn-signal"
                disabled={busy || !s.vapidPublicKey}
                onClick={() => act(async () => {
                  const sub = await subscribe(s.vapidPublicKey!);
                  setS(await api<AlertSettings>("/app/alerts/devices", { method: "POST", body: JSON.stringify({ endpoint: sub.endpoint, keys: sub.keys, label: deviceLabel() }) }));
                  setMine(sub.endpoint ?? null);
                  setNote("Alerts are on for this device.");
                })}
              >
                Turn on alerts here
              </button>
            )}
            {can === "ok" && checked && thisDevice && (
              <div className="alert-row">
                <span>Alerts are on for this {thisDevice.label ?? "device"}.</span>
                <button
                  className="btn btn-quiet"
                  disabled={busy}
                  onClick={() => act(async () => {
                    const r = await api<{ sent: number; failed: number }>("/app/alerts/test", { method: "POST", body: "{}" });
                    setNote(r.sent ? "Sent. It should arrive in a few seconds." : "Nothing reached a device. Turn alerts off and on again here.");
                  })}
                >
                  Send a test
                </button>
              </div>
            )}
            {!s.vapidPublicKey && <p className="dash-muted small">Alerts are not set up on the server yet.</p>}
            {note && <p className="alert-note">{note}</p>}
          </section>

          <section className="alert-card">
            <h2>What to send</h2>
            {KINDS.map((k) => (
              <label key={k.key} className="alert-toggle">
                <input
                  type="checkbox"
                  checked={s.prefs[k.key]}
                  disabled={busy}
                  onChange={(e) => act(async () => {
                    setS(await api<AlertSettings>("/app/alerts", { method: "PUT", body: JSON.stringify({ [k.key]: e.target.checked }) }));
                  })}
                />
                <span><strong>{k.label}</strong><span className="dash-muted small">{k.note}</span></span>
              </label>
            ))}
          </section>

          <section className="alert-card">
            <h2>Devices</h2>
            {s.devices.length === 0 && <p className="dash-muted">None yet.</p>}
            <ul className="device-list">
              {s.devices.map((d) => (
                <li key={d.id}>
                  <div>
                    <strong>{d.label ?? "A browser"}{d.endpoint === mine ? " (this one)" : ""}</strong>
                    <span>{d.lastOkAt ? `Last alert ${new Date(d.lastOkAt).toLocaleString()}` : `Added ${new Date(d.createdAt).toLocaleDateString()}`}</span>
                  </div>
                  <button
                    className="link-danger"
                    onClick={() => act(async () => {
                      if (d.endpoint === mine) await (await current())?.unsubscribe();
                      setS(await api<AlertSettings>(`/app/alerts/devices/${d.id}`, { method: "DELETE" }));
                      if (d.endpoint === mine) setMine(null);
                    })}
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          </section>

          <section className="alert-card">
            <h2>Slack <span className="dash-muted small">optional</span></h2>
            <p className="dash-muted small">
              {s.slack ? "Alerts also go to a Slack channel." : "Paste a Slack incoming webhook to send the same alerts to a channel."}
            </p>
            <div className="alert-row">
              <input
                className="alert-input"
                placeholder="https://hooks.slack.com/services/…"
                value={slack}
                onChange={(e) => setSlack(e.target.value)}
              />
              <button
                className="btn btn-quiet"
                disabled={busy || !slack}
                onClick={() => act(async () => {
                  setS(await api<AlertSettings>("/app/alerts", { method: "PUT", body: JSON.stringify({ slackWebhook: slack.trim() }) }));
                  setSlack("");
                  setNote("Slack is connected.");
                })}
              >
                Save
              </button>
            </div>
            {s.slack && (
              <button className="link-danger" onClick={() => act(async () => {
                setS(await api<AlertSettings>("/app/alerts", { method: "PUT", body: JSON.stringify({ slackWebhook: null }) }));
              })}>Disconnect Slack</button>
            )}
          </section>
        </>
      )}
    </div>
  );
}
