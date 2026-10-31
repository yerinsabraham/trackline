"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/account";
import { LIGHT_LABEL, type Member, modelLabel, whileQuiet } from "@/lib/dashboard";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";

// The agents a person chose to keep, whoever is working first. A chat is not a
// member until someone adds it.

export default function Team() {
  const [team, setTeam] = useState<Member[] | null>(null);
  const [error, setError] = useState("");
  const load = useCallback(() => { api<{ team: Member[] }>("/app/team").then((r) => { setTeam(r.team); setError(""); }).catch((e) => setError((e as Error).message)); }, []);
  useEffect(() => { load(); return whileQuiet(load, 8000, 30_000); }, [load]);
  const working = team?.filter((m) => m.chat?.light === "working").length ?? 0;
  const sorted = [...(team ?? [])].sort((a, b) => Number(b.chat?.light === "working") - Number(a.chat?.light === "working"));
  return (
    <div className="page">
      <header>
        <h1 className="app-h1">Team</h1>
        {!!team?.length && <p className="app-sub">{team.length} {team.length === 1 ? "agent" : "agents"}{working ? `, ${working} working` : ""}.</p>}
      </header>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}
      {!team && !error && [0, 1, 2].map((i) => <div key={i} className="skel" style={{ height: 112 }} />)}
      {team?.length === 0 && <div className="card empty"><strong>No one on your team yet.</strong><span>In any chat, open the three dots, then Add to team.</span></div>}
      <section className="team-grid" aria-label="Team members">
        {sorted.map((m) => (
          <Link key={m.id} href={`/app/agent?id=${encodeURIComponent(m.id)}`} className="team-card">
            <span className="team-card-top">
              <AgentLogo host={m.host} size={46} />
              <span><strong>{m.title ?? m.name}</strong><small>{[agentName(m.host), modelLabel(m.model)].filter(Boolean).join(" · ")}</small></span>
              {m.chat && <b className={`state-${m.chat.light}`}>{LIGHT_LABEL[m.chat.light]}</b>}
            </span>
            {m.description && <span className="team-card-job">{m.description}</span>}
          </Link>
        ))}
      </section>
      {!!team?.length && <p className="quiet-note">Add an agent from any chat: open the three dots, then Add to team.</p>}
    </div>
  );
}
