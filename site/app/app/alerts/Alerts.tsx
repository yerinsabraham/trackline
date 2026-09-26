"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/account";
import { ago } from "@/lib/dashboard";
import { type AlertSettings, current, deviceLabel, subscribe, support, type Support } from "@/lib/alerts";
import { canPrompt, isIPhone, onInstallChange, promptInstall } from "@/lib/install";
import { withStepUp } from "@/lib/security";
import { IconBell, IconPhone } from "@/components/app/icons";

const KINDS: { key: keyof AlertSettings["prefs"]; label: string; note: string }[] = [
  { key: "red", label: "Needs you", note: "An agent was stopped, a task you sent could not run, or your Mac is asking for permission." },
  { key: "finished", label: "Finished", note: "An agent, or a task you sent, finished, with the start of its reply." },
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
  const [, rerender] = useState(0);

  const load = async () => {
    setS(await api<AlertSettings>("/app/alerts"));
    // Asking the browser for its subscription can take a few seconds. Until
    // it answers, the page must not offer to turn on what may already be on.
    const cur = await current().catch(() => null);
    setMine(cur?.endpoint ?? null);
    setChecked(true);
  };

  useEffect(() => {
    setCan(support());
    load().catch((e) => setError((e as Error).message));
    return onInstallChange(() => rerender((n) => n + 1));
  }, []);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true); setError(""); setNote("");
    try { await fn(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };

  const thisDevice = s?.devices.find((d) => d.endpoint === mine);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 18, maxWidth: 720 }}>
      <header className="rise">
        <h1 className="app-h1">Alerts</h1>
        <p className="app-sub">Hear when an agent needs you, or when a task finishes.</p>
      </header>
      {error && <p className="composer-note error" style={{ textAlign: "left" }} role="alert">{error}</p>}
      {!s && !error && <div className="skel" style={{ height: 180 }} />}

      {s && (
        <>
          <section className="card rise rise-1" aria-labelledby="here">
            <div className="card-head">
              <span className="agent-logo" style={{ width: 36, height: 36 }}><span style={{ width: 18, display: "flex" }}><IconPhone /></span></span>
              <h2 id="here">This {deviceLabel() === "This browser" ? "browser" : deviceLabel()}</h2>
              {thisDevice && <span className="pill working" style={{ marginLeft: "auto" }}>On</span>}
            </div>
            <div className="card-body">
              {can === "ios-home-screen" && (
                <>
                  <p style={{ margin: 0, fontSize: 14.5 }}>On iPhone, alerts reach trackline only when it is on your Home Screen:</p>
                  <ol className="install-steps">
                    <li>Tap <span className="kbd"><ShareIcon /> Share</span> at the bottom of Safari.</li>
                    <li>Choose <strong>Add to Home Screen</strong>, then <strong>Add</strong>.</li>
                    <li>Open trackline from your Home Screen, and turn alerts on here.</li>
                  </ol>
                </>
              )}
              {can === "unsupported" && <p className="app-muted" style={{ margin: 0 }}>This browser cannot receive alerts. Try Chrome, Edge or Firefox, or Safari from the Home Screen on iPhone.</p>}
              {can === "blocked" && <p className="app-muted" style={{ margin: 0 }}>Notifications are blocked for this site. Allow them in your browser&apos;s site settings, then reload.</p>}
              {can === "ok" && !checked && <div className="skel" style={{ height: 40 }} />}
              {can === "ok" && checked && !thisDevice && (
                <button className="btn-act" style={{ alignSelf: "flex-start" }} disabled={busy || !s.vapidPublicKey} onClick={() => act(async () => {
                  const sub = await subscribe(s.vapidPublicKey!);
                  setS(await api<AlertSettings>("/app/alerts/devices", { method: "POST", body: JSON.stringify({ endpoint: sub.endpoint, keys: sub.keys, label: deviceLabel() }) }));
                  setMine(sub.endpoint ?? null);
                  setNote("Alerts are on here.");
                })}><span style={{ width: 16, display: "flex" }}><IconBell /></span>Turn on alerts here</button>
              )}
              {can === "ok" && checked && thisDevice && (
                <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
                  <span style={{ fontSize: 14 }}>Alerts come to this {thisDevice.label ?? "device"}.</span>
                  <button className="btn-plain" style={{ marginLeft: "auto", height: 36 }} disabled={busy} onClick={() => act(async () => {
                    const r = await api<{ sent: number; failed: number }>("/app/alerts/test", { method: "POST", body: "{}" });
                    setNote(r.sent ? "Sent. It should arrive in a few seconds." : "Nothing reached a device. Turn alerts off and on again here.");
                  })}>Send a test</button>
                </div>
              )}
              {!isIPhone() && canPrompt() && (
                <button className="btn-plain" style={{ alignSelf: "flex-start" }} onClick={() => promptInstall()}>Install trackline as an app</button>
              )}
              {!s.vapidPublicKey && <p className="app-muted" style={{ margin: 0, fontSize: 13 }}>Alerts are not set up on the server yet.</p>}
              {note && <p style={{ margin: 0, fontSize: 14, color: "var(--ok-ink)" }}>{note}</p>}
            </div>
          </section>

          <section className="card rise rise-2" aria-labelledby="what">
            <div className="card-head"><h2 id="what">What to send</h2></div>
            {KINDS.map((k) => (
              <label key={k.key} className="row-link switch-row">
                <span className="row-main">
                  <strong style={{ fontSize: 14.5 }}>{k.label}</strong>
                  <span className="app-muted" style={{ fontSize: 13, lineHeight: 1.45 }}>{k.note}</span>
                </span>
                <input type="checkbox" role="switch" className="switch" checked={s.prefs[k.key]} disabled={busy}
                  onChange={(e) => act(async () => {
                    setS(await api<AlertSettings>("/app/alerts", { method: "PUT", body: JSON.stringify({ [k.key]: e.target.checked }) }));
                  })} />
              </label>
            ))}
          </section>

          <section className="card rise rise-3" aria-labelledby="devices">
            <div className="card-head"><h2 id="devices">Devices that get alerts</h2></div>
            {s.devices.length === 0 && <div className="empty">None yet.</div>}
            {s.devices.map((d) => (
              <div key={d.id} className="row-link" style={{ alignItems: "center" }}>
                <span className="row-main">
                  <strong style={{ fontSize: 14 }}>{d.label ?? "A browser"}{d.endpoint === mine ? " · this one" : ""}</strong>
                  <span className="app-muted" style={{ fontSize: 13 }}>{d.lastOkAt ? `Last alert ${ago(d.lastOkAt)}` : `Added ${new Date(d.createdAt).toLocaleDateString()}`}</span>
                </span>
                <button className="btn-plain" style={{ height: 32, color: "var(--red-ink)" }} disabled={busy} onClick={() => act(async () => {
                  if (d.endpoint === mine) await (await current())?.unsubscribe();
                  setS(await api<AlertSettings>(`/app/alerts/devices/${d.id}`, { method: "DELETE" }));
                  if (d.endpoint === mine) setMine(null);
                })}>Remove</button>
              </div>
            ))}
          </section>

          <section className="card rise rise-4" aria-labelledby="slack">
            <div className="card-head"><h2 id="slack">Slack</h2><span className="app-muted" style={{ fontSize: 13 }}>optional</span></div>
            <div className="card-body">
              <span className="app-muted" style={{ fontSize: 13.5 }}>{s.slack ? "The same alerts also go to a Slack channel." : "Paste a Slack incoming webhook to send the same alerts to a channel."}</span>
              <div style={{ display: "flex", gap: 8 }}>
                <input className="account-input" aria-label="Slack webhook" placeholder="https://hooks.slack.com/services/…" value={slack} onChange={(e) => setSlack(e.target.value)} />
                <button className="btn-plain" disabled={busy || !slack} onClick={() => act(async () => {
                  setS(await withStepUp(() => api<AlertSettings>("/app/alerts", { method: "PUT", body: JSON.stringify({ slackWebhook: slack.trim() }) })));
                  setSlack(""); setNote("Slack is connected.");
                })}>Save</button>
              </div>
              {s.slack && <button className="btn-danger-text" style={{ alignSelf: "flex-start" }} onClick={() => act(async () => {
                setS(await withStepUp(() => api<AlertSettings>("/app/alerts", { method: "PUT", body: JSON.stringify({ slackWebhook: null }) })));
              })}>Disconnect Slack</button>}
            </div>
          </section>
        </>
      )}
    </div>
  );
}

/** Safari's Share symbol, drawn so the step names what the person will see. */
export function ShareIcon() {
  return (
    <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M12 3v12M8 7l4-4 4 4" /><path d="M6 11H5a1 1 0 0 0-1 1v8a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-8a1 1 0 0 0-1-1h-1" />
    </svg>
  );
}
