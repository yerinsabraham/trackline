"use client";

import { useState } from "react";
import { ago, LIGHT_LABEL, type SessionSummary } from "@/lib/dashboard";
import { plain } from "@/components/ReplyText";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import { useApp } from "@/components/app/AppShell";
import { IconAlert, IconLaptop } from "@/components/app/icons";

type Filter = "all" | "working" | "needs-you" | "done";
const FILTERS: [Filter, string][] = [["all", "All"], ["working", "Working"], ["needs-you", "Needs you"], ["done", "Done"]];
const DAY = 24 * 60 * 60 * 1000;

function greeting(): string {
  const h = new Date().getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
}

export default function Overview() {
  const { user, projects, remote, error } = useApp();
  const [filter, setFilter] = useState<Filter>("all");

  const sessions = (projects ?? [])
    .flatMap((p) => p.sessions.map((s) => ({ ...s, project: p.name })))
    .sort((a, b) => +new Date(b.lastAt) - +new Date(a.lastAt));
  const needsYou = sessions.filter((s) => s.light === "needs-you");
  const working = sessions.filter((s) => s.light === "working");
  const today = sessions.filter((s) => Date.now() - +new Date(s.lastAt) < DAY);
  const doneToday = today.filter((s) => s.light === "done" || s.light === "idle").length;
  // On track: of the actions checked in the last day, how many matched the
  // request. Nothing checked is said as such, never shown as a number.
  const checked = today.reduce((n, s) => n + s.score.session.checked, 0);
  const aligned = today.reduce((n, s) => n + s.score.session.aligned, 0);
  const onTrack = checked ? Math.round((aligned / checked) * 100) : null;

  const shown = sessions.filter((s) =>
    filter === "all" ? true : filter === "done" ? s.light === "done" || s.light === "idle" : s.light === filter).slice(0, 12);

  const first = user?.name?.split(/\s+/)[0];
  const summary = !projects ? "" :
    working.length || needsYou.length
      ? [working.length && `${working.length} ${working.length === 1 ? "agent is" : "agents are"} working`, needsYou.length && `${needsYou.length} ${needsYou.length === 1 ? "needs" : "need"} you`].filter(Boolean).join(". ") + "."
      : "Nothing is running right now.";

  const hosts = ["claude-code", "codex", "cursor"];
  const agentState = (h: string) => {
    const a = projects?.flatMap((p) => p.agents).filter((x) => x.host === h) ?? [];
    if (a.some((x) => x.reported)) return { label: "Watching", tone: "ok" };
    if (a.some((x) => x.wired)) return { label: h === "codex" ? "Trust its hook" : "Waiting for first action", tone: "wait" };
    return { label: "Set up", tone: "off" };
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <header className="rise">
        <h1 className="app-h1">{greeting()}{first ? `, ${first}` : ""}</h1>
        <p className="app-sub">{summary}</p>
      </header>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}

      <section className="stats rise rise-1" aria-label="Today">
        <Stat dot="needs-you" label="Needs you" value={projects ? needsYou.length : null} note={needsYou[0] ? `${agentName(needsYou[0].host)} is waiting` : "Nothing waiting"} />
        <Stat dot="working" label="Working" value={projects ? working.length : null} note={working.length ? [...new Set(working.map((s) => agentName(s.host)))].join(", ") : "No agent running"} />
        <Stat dot="done" label="Done today" value={projects ? doneToday : null} note={`Across ${new Set(today.map((s) => s.project)).size} project${new Set(today.map((s) => s.project)).size === 1 ? "" : "s"}`} desktopOnly />
        <Stat dot="on-track" label="On track" value={projects ? (onTrack === null ? "—" : `${onTrack}%`) : null} note={onTrack === null ? "Nothing checked today" : "of actions matched the request"} />
      </section>

      <div className="ov-grid">
        <section className="card rise rise-2" aria-labelledby="recent">
          <div className="card-head" style={{ flexWrap: "wrap" }}>
            <h2 id="recent">Recent sessions</h2>
            <div className="chips" role="tablist" aria-label="Filter" style={{ marginLeft: "auto" }}>
              {FILTERS.map(([f, label]) => (
                <button key={f} role="tab" className="chip" aria-selected={filter === f} onClick={() => setFilter(f)}>{label}</button>
              ))}
            </div>
          </div>
          {!projects && <div style={{ padding: 16, display: "grid", gap: 10 }}>{[0, 1, 2].map((i) => <div key={i} className="skel" style={{ height: 64 }} />)}</div>}
          {projects && sessions.length === 0 && (
            <div className="empty">
              <strong>No sessions yet.</strong>
              <span>In a project on your laptop, run <code>trackline connect</code>. What your agent does there shows up here as it happens.</span>
            </div>
          )}
          {projects && sessions.length > 0 && shown.length === 0 && <div className="empty">Nothing here.</div>}
          {shown.map((s) => <SessionRow key={s.id} s={s} project={s.project} />)}
          {sessions.length > shown.length && shown.length > 0 && (
            <a href="/app/sessions" className="row-link" style={{ justifyContent: "center", fontSize: 13, color: "var(--ink-2)" }}>All sessions</a>
          )}
        </section>

        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
          {needsYou.length > 0 && (
            <section className="card rise rise-3" aria-labelledby="needs" style={{ borderColor: "#f3c9bd" }}>
              <div className="card-head">
                <span style={{ width: 18, color: "#c8321a", display: "flex" }}><IconAlert /></span>
                <h2 id="needs">Needs you</h2>
              </div>
              {needsYou.slice(0, 3).map((s) => (
                <a key={s.id} href={`/app/session?id=${encodeURIComponent(s.id)}`} className="row-link">
                  <AgentLogo host={s.host} size={30} />
                  <span className="row-main">
                    <span className="row-top"><strong>{s.request ?? "A request"}</strong><span className="when">{ago(s.lastAt)}</span></span>
                    <span className="row-note">trackline stopped an action. Open the session to see why.</span>
                  </span>
                </a>
              ))}
            </section>
          )}

          <section className="card rise rise-3" aria-labelledby="machines">
            <div className="card-head">
              <h2 id="machines">Machines</h2>
              <a href="/app/machines">Manage</a>
            </div>
            <div className="card-body">
              {!remote && <div className="skel" style={{ height: 44 }} />}
              {remote?.machines.length === 0 && (
                <p style={{ margin: 0, fontSize: 14, color: "var(--ink-2)" }}>No laptop takes tasks yet. In a project: <code className="mono" style={{ fontSize: 12.5 }}>trackline remote enable</code></p>
              )}
              {remote?.machines.map((m) => (
                <div key={m.id} style={{ display: "flex", alignItems: "center", gap: 12 }}>
                  <span className="agent-logo" style={{ width: 38, height: 38 }}><span style={{ width: 20, display: "flex" }}><IconLaptop /></span></span>
                  <div style={{ display: "grid", gap: 2, minWidth: 0 }}>
                    <strong style={{ fontSize: 14 }}>{m.name}</strong>
                    <span style={{ fontSize: 13, display: "flex", alignItems: "center", gap: 6, color: m.stopped ? "var(--red-ink)" : m.online ? "var(--ok-ink)" : "var(--muted)" }}>
                      <span className={`dot ${m.online && !m.stopped ? "working" : ""}`} />
                      {m.stopped ? "Stopped" : m.online ? "Online · ready for tasks" : m.lastSeenAt ? `Asleep · seen ${ago(m.lastSeenAt)}` : "Not seen yet"}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </section>

          <section className="card rise rise-4" aria-labelledby="agents">
            <div className="card-head"><h2 id="agents">Agents</h2></div>
            <div className="card-body">
              {hosts.map((h) => {
                const st = agentState(h);
                return (
                  <a key={h} href={`/app/sessions?agent=${h}`} style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 14 }}>
                    <AgentLogo host={h} size={28} />
                    {agentName(h)}
                    <span style={{ marginLeft: "auto", fontSize: 13, color: st.tone === "ok" ? "var(--ok-ink)" : st.tone === "wait" ? "var(--amber-ink)" : "var(--act)" }}>{st.label}</span>
                  </a>
                );
              })}
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}

function Stat({ dot, label, value, note, desktopOnly }: { dot: string; label: string; value: number | string | null; note: string; desktopOnly?: boolean }) {
  return (
    <div className={`stat ${desktopOnly ? "desktop-only" : ""}`}>
      <span className="stat-label"><span className={`dot ${dot}`} style={dot === "working" ? { animation: "none" } : undefined} />{label}</span>
      {value === null ? <span className="skel" style={{ height: 30, width: 48 }} /> : <span className="stat-num">{value}</span>}
      <span className="stat-note">{note}</span>
    </div>
  );
}

export function SessionRow({ s, project, current }: { s: SessionSummary; project?: string; current?: boolean }) {
  const note = s.reply ? plain(s.reply.text) : null;
  return (
    <a href={`/app/session?id=${encodeURIComponent(s.id)}`} className="row-link" aria-current={current ? "true" : undefined}>
      <AgentLogo host={s.host} size={34} />
      <span className="row-main">
        <span className="row-top">
          <strong>{s.request ?? "Request not recorded"}</strong>
          <span className={`pill ${s.light}`}>{s.light === "working" && <span className="dot working" />}{LIGHT_LABEL[s.light]}</span>
        </span>
        <span className="row-meta">{project ? `${project} · ` : ""}{ago(s.lastAt)}</span>
        {note && <span className="row-note">{note}</span>}
      </span>
    </a>
  );
}
