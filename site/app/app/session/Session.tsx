"use client";

import { useEffect, useRef, useState } from "react";
import { api, clearSession, rememberNext, session } from "@/lib/account";
import {
  describe, type FeedEvent, HOST_LABEL, LIGHT_LABEL, scoreLine, type SessionView, whileVisible,
} from "@/lib/dashboard";
import "../dashboard.css";

const CHECK_LABEL: Record<string, string> = {
  "off-limits": "Off-limits files",
  "dependency-added": "New dependency",
  scope: "Scope",
  "diff-size": "Size of change",
  repetition: "Repetition",
  "tool-policy": "Tool policy",
  config: "Configuration",
  rules: "Rules",
};

const time = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });

export default function Session() {
  const [view, setView] = useState<SessionView["session"] | null>(null);
  const [events, setEvents] = useState<FeedEvent[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const [live, setLive] = useState(true);
  const cursor = useRef<string | null>(null);

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("id") ?? "";
    const here = `/app/session?id=${encodeURIComponent(id)}`;
    if (!session()) { rememberNext(here); return window.location.replace("/signin"); }

    let busy = false;
    const load = async () => {
      if (busy) return;
      busy = true;
      try {
        const after = cursor.current ? `?after=${encodeURIComponent(cursor.current)}` : "";
        const r = await api<SessionView>(`/app/sessions/${encodeURIComponent(id)}${after}`);
        cursor.current = r.cursor;
        setView(r.session);
        if (r.events.length) {
          setEvents((old) => {
            const byId = new Map(old.map((e) => [e.id, e]));
            for (const e of r.events) byId.set(e.id, e);
            return [...byId.values()].sort((a, b) => +new Date(b.at) - +new Date(a.at));
          });
        }
        setLive(true);
        setError("");
      } catch (e) {
        const status = (e as { status?: number }).status;
        if (status === 401) { clearSession(); rememberNext(here); window.location.replace("/signin"); }
        else if (status === 404) setError("This session does not exist, or is not yours.");
        else setLive(false);
      } finally {
        busy = false;
      }
    };
    load();
    return whileVisible(load, 3000);
  }, []);

  if (error) {
    return (
      <div className="wrap dash">
        <a href="/app" className="dash-back">← All sessions</a>
        <p className="account-error" role="alert">{error}</p>
      </div>
    );
  }
  if (!view) return <div className="wrap dash"><p className="dash-muted">Loading…</p></div>;

  // A heading wherever the request changes, so the feed reads as what you
  // asked, then what the agent did about it.
  const rows: ({ kind: "request"; text: string | null; key: string } | { kind: "event"; e: FeedEvent })[] = [];
  let last: string | null | undefined;
  for (const e of events) {
    const k = e.turn ?? e.request;
    if (k !== last) rows.push({ kind: "request", text: e.request, key: `r-${e.id}` });
    last = k;
    rows.push({ kind: "event", e });
  }

  return (
    <div className="wrap dash">
      <a href="/app" className="dash-back">← All sessions</a>
      <div className="dash-head">
        <p className="eyebrow">{view.project.name} · {HOST_LABEL[view.host] ?? view.host}</p>
      </div>

      <div className={`dash-status light-${view.light}`}>
        <div className="dash-status-top">
          <span className="dash-dot" aria-hidden />
          <strong>{LIGHT_LABEL[view.light]}</strong>
          <span className={`dash-live ${live ? "" : "off"}`}>{live ? "Live" : "Reconnecting…"}</span>
        </div>
        <p className="dash-status-score">{scoreLine(view.score.request)}</p>
        {scoreLine(view.score.session) !== scoreLine(view.score.request) && (
          <p className="dash-muted small">Whole session: {scoreLine(view.score.session)}</p>
        )}
      </div>

      <div className="dash-asked">
        <p className="eyebrow">What you asked</p>
        <p>{view.request ?? <span className="dash-muted">Not recorded.</span>}</p>
      </div>

      <ol className="dash-feed">
        {rows.map((row) => {
          if (row.kind === "request") {
            return (
              <li key={row.key} className="dash-feed-request">
                {row.text ? `You asked: ${row.text}` : "A request"}
              </li>
            );
          }
          const e = row.e;
          const expandable = e.findings.length > 0 || e.unmeasured.length > 0 || !e.checked;
          const mark = e.severity === "block" ? "needs-you" : e.severity === "warn" ? "drifting" : e.checked ? "on-track" : "quiet";
          return (
            <li key={e.id} className={`dash-item light-${mark}`}>
              <button
                className="dash-item-row"
                onClick={() => expandable && setOpen(open === e.id ? null : e.id)}
                aria-expanded={expandable ? open === e.id : undefined}
                disabled={!expandable}
              >
                <span className="dash-dot" aria-hidden />
                <span className="dash-item-text">
                  {describe(e)}
                  {e.blocked && <em> · stopped</em>}
                  {!e.checked && <em> · not checkable</em>}
                </span>
                <span className="dash-item-time">{time(e.at)}</span>
              </button>
              {open === e.id && (
                <div className="dash-why">
                  {e.findings.map((f, i) => (
                    <div key={i} className="dash-finding">
                      <p className="eyebrow">{CHECK_LABEL[f.check] ?? f.check} · {f.severity}</p>
                      <p>{f.summary}</p>
                      {f.suggestion && <p className="dash-muted">{f.suggestion}</p>}
                    </div>
                  ))}
                  {!e.checked && e.unmeasured.length === 0 && (
                    <p className="dash-muted">No check applies to this kind of action.</p>
                  )}
                  {e.unmeasured.map((u, i) => (
                    <p key={i} className="dash-muted">{CHECK_LABEL[u.check] ?? u.check}: {u.reason}</p>
                  ))}
                  {e.tool && <p className="dash-muted small">Tool: {e.tool}</p>}
                </div>
              )}
            </li>
          );
        })}
      </ol>
      {events.length === 0 && <p className="dash-muted">No actions in the last 30 days.</p>}
    </div>
  );
}
