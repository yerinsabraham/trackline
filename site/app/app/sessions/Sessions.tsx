"use client";

import { useEffect, useState } from "react";
import AgentLogo from "@/components/app/AgentLogo";
import { useApp } from "@/components/app/AppShell";
import { SessionRow } from "../Overview";

// Every session, by agent: Claude Code, Codex and Cursor each as their own
// list, the way the sidebar names them.

const AGENTS: [string, string][] = [["", "All"], ["claude-code", "Claude Code"], ["codex", "Codex"], ["cursor", "Cursor"]];
type Filter = "all" | "working" | "needs-you" | "done";
const FILTERS: [Filter, string][] = [["all", "All"], ["working", "Working"], ["needs-you", "Needs you"], ["done", "Done"]];

export default function Sessions() {
  const { projects } = useApp();
  const [agent, setAgent] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    setAgent(q.get("agent") ?? "");
    const st = q.get("status");
    if (st === "working" || st === "needs-you" || st === "done") setFilter(st);
  }, []);
  const pick = (a: string) => {
    setAgent(a);
    window.history.replaceState(null, "", a ? `/app/sessions?agent=${a}` : "/app/sessions");
    window.dispatchEvent(new Event("app:agent"));
  };

  const all = (projects ?? [])
    .flatMap((p) => p.sessions.map((s) => ({ ...s, project: p.name })))
    .filter((s) => !agent || s.host === agent)
    .sort((a, b) => +new Date(b.lastAt) - +new Date(a.lastAt));
  const matches = (s: (typeof all)[number], f: Filter) =>
    f === "all" ? true : f === "done" ? s.light === "done" || s.light === "idle" : s.light === f;
  const shown = all.filter((s) => matches(s, filter));
  const startOfDay = new Date(); startOfDay.setHours(0, 0, 0, 0);
  const groups: [string, typeof shown][] = [
    ["Today", shown.filter((s) => new Date(s.lastAt) >= startOfDay)],
    ["Earlier", shown.filter((s) => new Date(s.lastAt) < startOfDay)],
  ];
  const wired = !agent || !!projects?.some((p) => p.agents.some((a) => a.host === agent && a.wired));

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 860 }}>
      <header className="rise">
        <h1 className="app-h1">Sessions</h1>
      </header>
      <div className="seg rise rise-1" role="tablist" aria-label="Agent">
        {AGENTS.map(([a, label]) => (
          <button key={a || "all"} role="tab" aria-selected={agent === a} onClick={() => pick(a)}>
            {a && <AgentLogo host={a} size={20} />}
            <span className="seg-label">{label}</span>
          </button>
        ))}
      </div>
      <div className="chips rise rise-2" role="tablist" aria-label="Status">
        {FILTERS.map(([f, label]) => (
          <button key={f} role="tab" className="pick" aria-selected={filter === f} onClick={() => setFilter(f)}>
            {label} {projects ? all.filter((s) => matches(s, f)).length : ""}
          </button>
        ))}
      </div>

      {!projects && [0, 1, 2, 3].map((i) => <div key={i} className="skel" style={{ height: 72 }} />)}
      {projects && !wired && (
        <div className="card empty">
          <AgentLogo host={agent} size={40} />
          <strong>trackline is not set up for {AGENTS.find(([a]) => a === agent)?.[1]} yet.</strong>
          <span>In a project on your laptop: <code>trackline init --host {agent === "claude-code" ? "claude" : agent}</code></span>
        </div>
      )}
      {projects && wired && shown.length === 0 && <div className="card empty">No sessions here yet.</div>}
      {groups.map(([name, list]) => list.length > 0 && (
        <section key={name} aria-label={name} style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          <div className="app-nav-label" style={{ padding: "6px 2px 0" }}>{name}</div>
          <div className="card rise">
            {list.map((s) => <SessionRow key={s.id} s={s} project={s.project} />)}
          </div>
        </section>
      ))}
    </div>
  );
}
