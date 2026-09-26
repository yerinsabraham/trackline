"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/account";
import { ago, whileVisible } from "@/lib/dashboard";
import { Cmd } from "@/components/app/FirstRun";
import { IconChevron, IconServer } from "@/components/app/icons";

// Deployed agents, from their traces: what `trackline serve --connect`
// concluded about each conversation. Tool names, findings, incidents and
// counts; what a customer or the model said never reaches this page, because
// it never leaves the machine running the agent.

type Service = { name: string; conversations: number; flagged: number; findings: number; incidents: number; tokens: number; lastAt: string | null };
type Conversation = {
  id: string; service: string; conversation: string; startedAt: string;
  tools: { name: string; calls: number; errors: number }[];
  findings: { check: string; severity: string; summary: string; tool?: string }[];
  incidents: { kind: string; summary: string }[];
  modelCalls: number; tokens: number; modelLatencyMs: number | null; contentAvailable: boolean;
  unmeasured: { check: string; reason: string }[] | null;
};
type Data = { days: number; services: Service[]; conversations: Conversation[]; connected: boolean };

const RANGES: [number, string][] = [[1, "Today"], [7, "7 days"], [30, "30 days"]];
const INCIDENT: Record<string, string> = { "tool-errors": "Tool failing", loop: "Loop", latency: "Slow", tokens: "Token spike" };
const within = (d: number) => (d === 1 ? "today" : `in the last ${d} days`);

export default function Production() {
  const [days, setDays] = useState(1);
  const [service, setService] = useState("");
  const [data, setData] = useState<Data | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    const d = Number(q.get("days"));
    if ([1, 7, 30].includes(d)) setDays(d);
    setService(q.get("service") ?? "");
  }, []);
  useEffect(() => {
    let current = true;
    const load = () => api<Data>(`/app/production?days=${days}${service ? `&service=${encodeURIComponent(service)}` : ""}`)
      .then((d) => { if (current) { setData(d); setError(""); } })
      .catch((e) => { if (current) setError((e as Error).message); });
    setData((x) => (x && x.days === days ? x : null));
    load();
    const stop = whileVisible(load, 15000);
    return () => { current = false; stop(); };
  }, [days, service]);

  const go = (d: number, s: string) => {
    setDays(d); setService(s);
    const q = new URLSearchParams();
    if (d !== 1) q.set("days", String(d));
    if (s) q.set("service", s);
    window.history.replaceState(null, "", `/app/production${q.size ? `?${q}` : ""}`);
  };

  const shown = data?.conversations ?? [];
  const flagged = shown.filter((c) => c.findings.length || c.incidents.length).length;

  return (
    <div className="ot">
      <header className="ot-head rise">
        <div>
          <h1 className="app-h1">Production</h1>
          <p className="app-sub">Your deployed agents, checked from their traces.</p>
        </div>
        {data?.connected && (
          <div className="seg ot-range" role="tablist" aria-label="Range">
            {RANGES.map(([d, label]) => <button key={d} role="tab" aria-selected={days === d} onClick={() => go(d, service)}>{label}</button>)}
          </div>
        )}
      </header>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}

      {!data && !error && <div className="card" style={{ padding: 16, display: "grid", gap: 10 }}>{[0, 1, 2].map((i) => <div key={i} className="skel" style={{ height: 56 }} />)}</div>}

      {data && !data.connected && <Setup />}

      {data?.connected && (
        <>
          <section className="pr-services rise rise-1" aria-label="Services">
            {data.services.length === 0 && <div className="card empty"><strong>Nothing {within(days)}.</strong><span>No conversations arrived in this range.</span></div>}
            {data.services.map((s) => (
              <button key={s.name} className={`card pr-service ${s.flagged ? "has" : ""}`} aria-pressed={service === s.name} onClick={() => go(days, service === s.name ? "" : s.name)}>
                <span className="pr-service-name"><span style={{ width: 18, display: "flex" }}><IconServer /></span>{s.name}</span>
                <span className="pr-stats">
                  <span><strong>{s.conversations}</strong> conversation{s.conversations === 1 ? "" : "s"}</span>
                  <span className={s.findings ? "bad" : ""}><strong>{s.findings}</strong> policy</span>
                  <span className={s.incidents ? "warn" : ""}><strong>{s.incidents}</strong> incident{s.incidents === 1 ? "" : "s"}</span>
                </span>
                <span className="row-meta">{s.lastAt ? `last ${ago(s.lastAt)}` : ""} · {s.tokens.toLocaleString()} tokens</span>
              </button>
            ))}
          </section>

          <section className="card rise rise-2" aria-labelledby="convs">
            <div className="card-head">
              <h2 id="convs">{service ? service : "Conversations"}</h2>
              <span className="row-meta" style={{ marginLeft: "auto" }}>{flagged} of {shown.length} flagged</span>
            </div>
            {shown.length === 0 && <div className="empty">Nothing {within(days)}.</div>}
            {shown.map((c) => <Row key={c.id} c={c} showService={!service} />)}
          </section>
          <p className="ot-note">What customers and the model said never leaves the machine running the agent. This page shows tool names, what trackline found, and counts.</p>
        </>
      )}
    </div>
  );
}

function Row({ c, showService }: { c: Conversation; showService: boolean }) {
  const bad = c.findings.length > 0;
  const warn = !bad && c.incidents.length > 0;
  const label = bad ? "Policy" : warn ? INCIDENT[c.incidents[0].kind] ?? "Incident" : c.unmeasured?.length ? "Not checked" : "Clean";
  const note = c.findings[0]?.summary ?? c.incidents[0]?.summary ?? c.unmeasured?.[0]?.reason ?? c.tools.map((t) => t.name).join(", ");
  return (
    <details className="pr-row">
      <summary className="row-link">
        <span className="row-main">
          <span className="row-top">
            <strong className="mono">{c.conversation}</strong>
            <span className={`pill ${bad ? "needs-you" : warn ? "drifting" : ""}`}>{label}</span>
          </span>
          <span className="row-meta">{showService ? `${c.service} · ` : ""}{ago(c.startedAt)}</span>
          {note && <span className="row-note">{note}</span>}
        </span>
        <span className="pr-chev" aria-hidden="true"><IconChevron /></span>
      </summary>
      <div className="pr-detail">
        {c.findings.map((f, i) => <p key={`f${i}`} className="pr-line bad"><span>Policy</span>{f.summary}</p>)}
        {c.incidents.map((f, i) => <p key={`i${i}`} className="pr-line warn"><span>{INCIDENT[f.kind] ?? f.kind}</span>{f.summary}</p>)}
        {c.unmeasured?.map((u, i) => <p key={`u${i}`} className="pr-line"><span>Not checked</span>{u.reason}</p>)}
        <div className="pr-tools">
          {c.tools.length === 0 && <span className="row-meta">No tools called.</span>}
          {c.tools.map((t) => (
            <span key={t.name} className={`pr-tool ${t.errors ? "err" : ""}`}>{t.name}<b>×{t.calls}</b>{t.errors > 0 && <em>{t.errors} failed</em>}</span>
          ))}
        </div>
        <p className="row-meta">{c.modelCalls} model call{c.modelCalls === 1 ? "" : "s"} · {c.tokens.toLocaleString()} tokens{c.modelLatencyMs ? ` · ${c.modelLatencyMs < 1000 ? `${c.modelLatencyMs}ms` : `${(c.modelLatencyMs / 1000).toFixed(1)}s`} per call` : ""}</p>
      </div>
    </details>
  );
}

function Setup() {
  return (
    <section className="card fr rise rise-1" aria-labelledby="pr-setup">
      <div className="fr-head">
        <h2 id="pr-setup">Watch a deployed agent</h2>
        <p className="fr-laptop" style={{ display: "flex" }}>On the machine that receives your agent&apos;s traces.</p>
      </div>
      <ol className="fr-steps">
        <li className="fr-step now">
          <span className="fr-n">1</span>
          <div className="fr-body">
            <strong>Connect it to this account</strong>
            <Cmd text="trackline connect" />
          </div>
        </li>
        <li className="fr-step now">
          <span className="fr-n">2</span>
          <div className="fr-body">
            <strong>Receive its traces</strong>
            <Cmd text="trackline serve --connect" />
            <p>Point your OpenTelemetry exporter at <code className="mono">http://127.0.0.1:4318/v1/traces</code>.</p>
          </div>
        </li>
        <li className="fr-step">
          <span className="fr-n">3</span>
          <div className="fr-body">
            <strong>Conversations appear here</strong>
            <p>Tool names, policy findings, outages, loops and slowdowns. Never what customers or the model said.</p>
          </div>
        </li>
      </ol>
    </section>
  );
}
