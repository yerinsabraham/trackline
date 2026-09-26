"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/account";
import { ago, requestLabel, type Score } from "@/lib/dashboard";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import Ring, { band, verdict } from "@/components/app/Ring";
import { IconChevron } from "@/components/app/icons";

// On track, explained: how much of what the agents did matched what was
// asked, and, for the rest, which check caught it, where, and in which
// session. The number is the product; this page is its evidence.

type Data = {
  days: number;
  partial: boolean;
  score: Score;
  projects: { id: string; name: string; score: Score }[];
  agents: { host: string; score: Score }[];
  checks: { check: string; count: number; blocked: number; example: string }[];
  offTrack: { id: string; at: string; sessionId: string; project: string; host: string | null; check: string; summary: string; severity: string; target: string | null; request: string | null }[];
  sessions: { id: string; host: string | null; project: string; request: string | null; lastAt: string; score: Score }[];
  trend: { day: string; percent: number | null; checked: number }[];
};

const RANGES: [number, string][] = [[1, "Today"], [7, "7 days"], [30, "30 days"]];

// Each check in a line a person can act on.
const CHECK: Record<string, { name: string; means: string }> = {
  "off-limits": { name: "Off-limits files", means: "Writes to files that should never change, like .env and keys." },
  "dependency-added": { name: "New dependencies", means: "Packages added that the request did not ask for." },
  scope: { name: "Outside the request", means: "Changes to parts of the project the request was not about." },
  "diff-size": { name: "Size of change", means: "Changes far larger than the request called for." },
  repetition: { name: "Going in circles", means: "The same action tried again and again." },
  "tool-policy": { name: "Tool policy", means: "Tools your policy says never to call, or not without approval first." },
};
const checkName = (c: string) => CHECK[c]?.name ?? c;
const within = (d: number) => (d === 1 ? "today" : `in the last ${d} days`);

export default function OnTrack() {
  const [days, setDays] = useState(1);
  const [data, setData] = useState<Data | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    const d = Number(new URLSearchParams(window.location.search).get("days"));
    if ([1, 7, 30].includes(d)) setDays(d);
  }, []);
  useEffect(() => {
    // An answer for a range no longer chosen is dropped: switching quickly,
    // or arriving on ?days=7, must never show another range's numbers.
    let current = true;
    setData((x) => (x && x.days === days ? x : null));
    api<Data>(`/app/ontrack?days=${days}`).then((d) => { if (current) setData(d); }).catch((e) => { if (current) setError((e as Error).message); });
    return () => { current = false; };
  }, [days]);

  const pick = (d: number) => {
    setDays(d);
    window.history.replaceState(null, "", d === 1 ? "/app/ontrack" : `/app/ontrack?days=${d}`);
  };

  const s = data?.score;
  return (
    <div className="ot">
      <header className="ot-head rise">
        <div>
          <h1 className="app-h1">On track</h1>
          <p className="app-sub">Whether your agents did what you asked.</p>
        </div>
        <div className="seg ot-range" role="tablist" aria-label="Range">
          {RANGES.map(([d, label]) => <button key={d} role="tab" aria-selected={days === d} onClick={() => pick(d)}>{label}</button>)}
        </div>
      </header>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}

      <section className={`card ot-hero rise rise-1 band-${band(s?.percent ?? null)}`} aria-label="Score">
        {!data ? <div className="skel" style={{ width: 168, height: 168, borderRadius: "50%" }} /> : <Ring key={days} percent={s!.percent} />}
        <div className="ot-hero-text">
          {!data ? <><div className="skel" style={{ height: 30, width: 200 }} /><div className="skel" style={{ height: 60 }} /></> : (
            <>
              <span className={`ot-verdict band-${band(s!.percent)}`}>{verdict(s!.percent)}</span>
              {s!.percent === null ? (
                <p className="ot-lead">Nothing trackline could check happened {within(days)}. When an agent writes, edits or runs something, it shows here.</p>
              ) : (
                <p className="ot-lead">Of <strong>{s!.checked}</strong> action{s!.checked === 1 ? "" : "s"} trackline could check {within(days)}, <strong>{s!.aligned}</strong> matched what you asked{s!.checked - s!.aligned ? <>, and <strong>{s!.checked - s!.aligned}</strong> did not</> : ""}.</p>
              )}
              {s!.notCheckable > 0 && (
                <p className="ot-note">{s!.notCheckable} more could not be checked, such as reads and commands trackline cannot see into. They do not count either way.</p>
              )}
              {data.partial && <p className="ot-note">Very busy: this counts the latest 5,000 actions.</p>}
            </>
          )}
        </div>
      </section>

      {data && data.trend.length > 0 && (
        <section className="card rise rise-2" aria-labelledby="trend">
          <div className="card-head"><h2 id="trend">Day by day</h2></div>
          <div className="ot-trend" role="list">
            {data.trend.map((t, i) => (
              <div key={t.day} className="ot-bar" role="listitem" title={`${new Date(t.day).toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short" })}: ${t.percent === null ? "nothing checked" : `${t.percent}% of ${t.checked}`}`}>
                <span className={`ot-bar-fill band-${band(t.percent)}`} style={{ height: t.percent === null ? 4 : `${Math.max(t.percent, 4)}%`, animationDelay: `${i * 18}ms` }} />
                {(data.trend.length <= 7 || i % 5 === 0 || i === data.trend.length - 1) && (
                  <span className="ot-bar-day">{new Date(t.day).toLocaleDateString(undefined, data.trend.length <= 7 ? { weekday: "short" } : { day: "numeric", month: "short" })}</span>
                )}
              </div>
            ))}
          </div>
        </section>
      )}

      {data && s!.percent !== null && (
        <div className="ot-grid">
          <section className="card rise rise-2" aria-labelledby="why">
            <div className="card-head"><h2 id="why">What went off track</h2></div>
            {data.checks.length === 0 ? (
              <div className="empty"><strong>Nothing.</strong><span>Every action trackline checked matched what you asked.</span></div>
            ) : data.checks.map((c) => (
              <div key={c.check} className="ot-check">
                <div className="ot-check-top">
                  <strong>{checkName(c.check)}</strong>
                  <span className="ot-count">{c.count}</span>
                </div>
                <span className="app-muted" style={{ fontSize: 13 }}>{CHECK[c.check]?.means}</span>
                <span className="ot-example">“{c.example}”</span>
                {c.blocked > 0 && <span className="pill needs-you" style={{ alignSelf: "flex-start" }}>{c.blocked} stopped</span>}
              </div>
            ))}
          </section>

          <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
            <section className="card rise rise-3" aria-labelledby="by-project">
              <div className="card-head"><h2 id="by-project">By project</h2></div>
              <div className="card-body">{data.projects.map((p) => <Bar key={p.id} label={<span className="mono">{p.name}</span>} score={p.score} />)}</div>
            </section>
            <section className="card rise rise-4" aria-labelledby="by-agent">
              <div className="card-head"><h2 id="by-agent">By agent</h2></div>
              <div className="card-body">{data.agents.map((a) => <Bar key={a.host} label={<><AgentLogo host={a.host} size={22} />{agentName(a.host)}</>} score={a.score} />)}</div>
            </section>
          </div>
        </div>
      )}

      {data && data.offTrack.length > 0 && (
        <section className="card rise" aria-labelledby="actions">
          <div className="card-head"><h2 id="actions">Actions that went off track</h2><span className="app-muted" style={{ fontSize: 13, marginLeft: "auto" }}>newest first</span></div>
          {data.offTrack.map((o) => (
            <a key={o.id} href={`/app/session?id=${encodeURIComponent(o.sessionId)}`} className="row-link">
              <AgentLogo host={o.host} size={32} />
              <span className="row-main">
                <span className="row-top">
                  <strong>{o.summary}</strong>
                  <span className={`pill ${o.severity === "block" ? "needs-you" : "drifting"}`}>{o.severity === "block" ? "Stopped" : "Warned"}</span>
                </span>
                <span className="row-meta">{checkName(o.check)} · {o.project} · {ago(o.at)}</span>
                {o.request && <span className="row-note">Asked: {requestLabel(o.request)}</span>}
              </span>
              <span className="ot-go"><IconChevron /></span>
            </a>
          ))}
        </section>
      )}

      {data && data.sessions.length > 0 && (
        <section className="card rise" aria-labelledby="sessions">
          <div className="card-head"><h2 id="sessions">Sessions, least on track first</h2></div>
          {data.sessions.map((x) => (
            <a key={x.id} href={`/app/session?id=${encodeURIComponent(x.id)}`} className="row-link" style={{ alignItems: "center" }}>
              <Ring percent={x.score.percent} size={44} stroke={5} label={false} />
              <span className="row-main">
                <span className="row-top"><strong>{requestLabel(x.request) ?? "Request not recorded"}</strong><span className={`ot-pct band-${band(x.score.percent)}`}>{x.score.percent === null ? "—" : `${x.score.percent}%`}</span></span>
                <span className="row-meta">{agentName(x.host)} · {x.project} · {ago(x.lastAt)}</span>
                <span className="row-note">{x.score.percent === null ? "Nothing checkable" : `${x.score.aligned} of ${x.score.checked} checked actions matched`}{x.score.notCheckable ? ` · ${x.score.notCheckable} not checkable` : ""}</span>
              </span>
            </a>
          ))}
        </section>
      )}

      {data && (
        <details className="ot-how">
          <summary>How this is worked out</summary>
          <p>Every action an agent takes is checked as it happens: files it must not touch, dependencies it was not asked to add, changes outside the request, changes far larger than asked, and going in circles. An action is on track when no check found a problem with it.</p>
          <p>The percentage is on-track actions out of the actions trackline could check. Actions no check could judge, such as reading a file, are counted apart and never help or hurt the number. With nothing checked, there is no number.</p>
        </details>
      )}
    </div>
  );
}

function Bar({ label, score }: { label: React.ReactNode; score: Score }) {
  const p = score.percent;
  return (
    <div className="ot-row">
      <span className="ot-row-label">{label}</span>
      <span className="ot-track"><span className={`ot-fill band-${band(p)}`} style={{ width: p === null ? 0 : `${Math.max(p, 2)}%` }} /></span>
      <span className={`ot-pct band-${band(p)}`}>{p === null ? "—" : `${p}%`}</span>
      <span className="ot-row-sub">{p === null ? "nothing checked" : `${score.aligned}/${score.checked}`}</span>
    </div>
  );
}
