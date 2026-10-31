"use client";

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { api } from "@/lib/account";
import { modelLabel, type Profile, scoreLine, whileQuiet } from "@/lib/dashboard";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import { ConfirmSheet, NameSheet, PositionSheet } from "@/components/app/SessionMenu";
import { SessionRow } from "@/components/app/SessionRow";

// A record, not a biography: who it is, and what it has actually done.

export default function Agent() {
  const id = useSearchParams().get("id") ?? "";
  const [p, setP] = useState<Profile | null>(null);
  const [error, setError] = useState("");
  const [sheet, setSheet] = useState<"name" | "position" | "remove" | null>(null);
  const path = `/app/team/${encodeURIComponent(id)}`;
  const load = useCallback(() => { api<Profile>(path).then(setP).catch((e) => setError((e as Error).message)); }, [path]);
  useEffect(() => { load(); return whileQuiet(load, 8000, 30_000); }, [load]);
  const act = (work: Promise<unknown>, after: () => void) => work.then(after).catch((e) => setError((e as Error).message));

  if (!p) return error ? <p className="composer-note error">{error}</p> : <div className="skel" style={{ height: 160 }} />;
  const r = p.record;
  const latest = p.chats[0];
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 860 }}>
      <header className="agent-profile rise">
        <AgentLogo host={p.agent.host} size={56} />
        <div>
          <h1 className="app-h1">{p.agent.name}</h1>
          <span className="agent-badge">AI agent</span>
          {p.agent.title && <p className="agent-title"><strong>{p.agent.title}</strong></p>}
          {p.agent.description && <p className="row-meta">{p.agent.description}</p>}
          <p className="row-meta">{[agentName(p.agent.host), modelLabel(p.model), p.agent.project.name].filter(Boolean).join(" · ")}</p>
        </div>
      </header>
      <div className="agent-actions">
        {latest && <a className="btn-act" href={`/app/session?id=${encodeURIComponent(latest.id)}`}>Message</a>}
        <a className="btn-plain" href={`/app/remote?with=${encodeURIComponent(p.agent.id)}`}>New chat</a>
        {p.agent.host !== "cursor" && <button type="button" className="btn-plain" onClick={() => setSheet("position")}>Set position</button>}
        <button type="button" className="btn-plain" onClick={() => setSheet("name")}>Rename</button>
        <button type="button" className="btn-plain" onClick={() => setSheet("remove")}>Remove from team</button>
      </div>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}
      <section className="card agent-record rise" aria-label="Record">
        <div className="card-head"><h2>Last {r.days} days</h2></div>
        <dl>
          <div><dt>Chats</dt><dd>{r.chats}</dd></div>
          <div><dt>Steps</dt><dd>{r.steps}{r.partial ? "+" : ""}</dd></div>
          <div><dt>Replies</dt><dd>{r.replies}</dd></div>
          <div><dt>Findings</dt><dd>{r.findings}</dd></div>
          <div><dt>Stopped</dt><dd>{r.blocked}</dd></div>
        </dl>
        <p className="row-meta" style={{ padding: "0 18px 16px" }}>{scoreLine(r.score)}</p>
      </section>
      {p.chats.length > 0 && <section className="card rise" aria-label="Chats">{p.chats.map((c) => <SessionRow key={c.id} s={c} project={c.project.name} />)}</section>}
      {sheet === "name" && <NameSheet label="Name" value={p.agent.name} onClose={() => setSheet(null)}
        onSave={(name) => act(api(path, { method: "PATCH", body: JSON.stringify({ name }) }), load)} />}
      {sheet === "position" && <PositionSheet title={p.agent.title} description={p.agent.description} onClose={() => setSheet(null)}
        onSave={(body) => act(api(path, { method: "PATCH", body: JSON.stringify(body) }), load)} />}
      {sheet === "remove" && <ConfirmSheet title={`Remove ${p.agent.name} from your team? Its chats stay.`} action="Remove from team" onClose={() => setSheet(null)}
        onConfirm={() => act(api(path, { method: "DELETE" }), () => window.location.assign("/app/team"))} />}
    </div>
  );
}
