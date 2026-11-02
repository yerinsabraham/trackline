"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/account";
import { LIGHT_LABEL, modelLabel, sessionName, titled, whileVisible } from "@/lib/dashboard";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import { useApp } from "@/components/app/AppShell";
import FirstRun from "@/components/app/FirstRun";
import UpdateCard from "@/components/app/UpdateCard";
import { useHandoffs, WaitingCards } from "@/components/app/HandoffCards";

// Where the app opens: one sentence on how things stand, whatever needs you,
// three numbers, and your agents. Everything else is one tap away.

const DAY = 24 * 60 * 60 * 1000;

function greeting(): string {
  const h = new Date().getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
}

export default function Home() {
  const { user, projects, error } = useApp();
  const handoffs = useHandoffs();
  const [onTrack, setOnTrack] = useState<{ percent: number | null; aligned: number; checked: number } | null | undefined>(undefined);
  useEffect(() => {
    // On its own slow clock: this reads a whole day of actions.
    const load = () => { api<{ score: { percent: number | null; aligned: number; checked: number } }>("/app/ontrack?days=1").then((r) => setOnTrack(r.score)).catch(() => setOnTrack(null)); };
    load();
    return whileVisible(load, 60_000);
  }, []);

  const chats = (projects ?? []).flatMap((p) => p.sessions.map((s) => ({ ...s, projectName: p.name }))).sort((a, b) => +new Date(b.lastAt) - +new Date(a.lastAt));
  const working = chats.filter((s) => s.light === "working");
  const needsYou = chats.filter((s) => s.light === "needs-you");
  const today = chats.filter((s) => Date.now() - +new Date(s.lastAt) < DAY);
  const done = today.filter((s) => s.light === "done" || s.light === "idle");
  const waiting = handoffs.list.filter((h) => h.status === "asked");
  const need = needsYou.length + waiting.length;
  const first = user?.name?.split(/\s+/)[0];

  if (projects && chats.length === 0) return (
    <div className="page">
      <header><h1 className="app-h1">{greeting()}{first ? `, ${first}` : ""}</h1><p className="app-sub">Three steps and your agents show up here.</p></header>
      <FirstRun projects={projects} />
    </div>
  );

  const summary = !projects ? "" : [
    working.length ? `${working.length} ${working.length === 1 ? "agent is" : "agents are"} working.` : "Nothing is running right now.",
    need ? `${need} ${need === 1 ? "thing needs" : "things need"} your OK.` : "",
  ].filter(Boolean).join(" ");

  return (
    <div className="page">
      <header>
        <h1 className="app-h1">{greeting()}{first ? `, ${first}` : ""}</h1>
        <p className="app-sub">{summary}</p>
      </header>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}

      <WaitingCards list={waiting} />
      <UpdateCard />
      {needsYou.slice(0, 3).map((s) => (
        <Link key={s.id} href={`/app/session?id=${encodeURIComponent(s.id)}`} className="ok-card">
          <span><strong>{s.agentName ? titled(s.agentName, s.agentTitle) : agentName(s.host)}</strong><small>{LIGHT_LABEL[s.light]} · {s.projectName}</small></span>
        </Link>
      ))}

      <section className="tiles3" aria-label="Today">
        <Link href="/app/ontrack"><strong>{onTrack === undefined ? "–" : onTrack?.percent == null ? "–" : `${onTrack.percent}%`}</strong><span>On track</span>{onTrack?.percent != null && <small>{onTrack.aligned} of {onTrack.checked} checked steps</small>}</Link>
        <Link href="/app/sessions?status=working"><strong>{projects ? working.length : "–"}</strong><span>Working</span>{working.length > 0 && <small>{working.slice(0, 2).map((s) => s.agentTitle ?? s.agentName ?? agentName(s.host)).join(", ")}</small>}</Link>
        <Link href="/app/sessions?status=done"><strong>{projects ? done.length : "–"}</strong><span>Done today</span><small>Across {new Set(today.map((s) => s.projectName)).size || 0} project{new Set(today.map((s) => s.projectName)).size === 1 ? "" : "s"}</small></Link>
      </section>

      <section aria-labelledby="agents">
        <div className="section-head">
          <h2 id="agents">Your agents</h2>
          <Link href="/app/team">See team</Link>
        </div>
        {!projects && [0, 1, 2].map((i) => <div key={i} className="skel" style={{ height: 60, marginBottom: 8 }} />)}
        {chats.slice(0, 4).map((s) => (
          <Link key={s.id} href={`/app/session?id=${encodeURIComponent(s.id)}`} className="agent-row">
            <AgentLogo host={s.host} size={42} />
            <span>
              <strong>{s.agentName ? titled(s.agentName, s.agentTitle) : agentName(s.host)}</strong>
              <small>{[modelLabel(s.model), sessionName(s, agentName(s.host), s.projectName)].filter(Boolean).join(" · ")}</small>
            </span>
            <i className={`state-dot ${s.light}`} role="img" aria-label={LIGHT_LABEL[s.light]} />
          </Link>
        ))}
      </section>
    </div>
  );
}
