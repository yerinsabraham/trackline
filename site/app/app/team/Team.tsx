"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/account";
import { LIGHT_LABEL, type Member, modelLabel, titled, whileQuiet } from "@/lib/dashboard";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";

// The agents a person chose to keep, by project, working ones first. A chat is
// not a member until someone adds it.

export default function Team() {
  const [team, setTeam] = useState<Member[] | null>(null);
  const [error, setError] = useState("");
  const load = useCallback(() => { api<{ team: Member[] }>("/app/team").then((r) => { setTeam(r.team); setError(""); }).catch((e) => setError((e as Error).message)); }, []);
  useEffect(() => { load(); return whileQuiet(load, 8000, 30_000); }, [load]);
  const projects = [...new Set(team?.map((m) => m.project.name))];
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 860 }}>
      <header className="rise"><h1 className="app-h1">Team</h1></header>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}
      {!team && !error && [0, 1, 2].map((i) => <div key={i} className="skel" style={{ height: 72 }} />)}
      {team?.length === 0 && <div className="card empty"><strong>No one on your team yet.</strong><span>Open a chat, tap the agent&apos;s name, then Add to team.</span></div>}
      {projects.map((p) => (
        <section key={p} className="card rise" aria-label={p}>
          <div className="card-head"><h2>{p}</h2></div>
          {team!.filter((m) => m.project.name === p).sort((a, b) => Number(b.chat?.light === "working") - Number(a.chat?.light === "working")).map((m) => (
            <Link key={m.id} href={`/app/agent?id=${encodeURIComponent(m.id)}`} className="row-link">
              <AgentLogo host={m.host} size={34} />
              <span className="row-main">
                <span className="row-top">
                  <strong>{titled(m.name, m.title)}</strong>
                  {m.chat && <span className={`pill ${m.chat.light}`}>{m.chat.light === "working" && <span className="dot working" />}{LIGHT_LABEL[m.chat.light]}</span>}
                </span>
                <span className="row-meta">{[agentName(m.host), modelLabel(m.model)].filter(Boolean).join(" · ")}</span>
              </span>
            </Link>
          ))}
        </section>
      ))}
    </div>
  );
}
