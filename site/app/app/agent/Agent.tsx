"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { api } from "@/lib/account";
import { modelLabel, type Profile, whileQuiet } from "@/lib/dashboard";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import { IconBack } from "@/components/app/icons";
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
    <div className="page profile">
      <div className="profile-bar"><Link href="/app/team" className="app-icon-btn" aria-label="Back to team"><IconBack size={24} /></Link></div>
      <header className="profile-head">
        <AgentLogo host={p.agent.host} size={76} />
        <h1 className="app-h1">{p.agent.title ?? p.agent.name}</h1>
        <p className="profile-name">{p.agent.title ? `${p.agent.name} · ` : ""}AI agent</p>
        {p.agent.description && <p className="profile-job">{p.agent.description}</p>}
      </header>
      <div className="profile-actions">
        {latest && <a className="pill-btn solid" href={`/app/session?id=${encodeURIComponent(latest.id)}`}>Message</a>}
        <a className="pill-btn" href={`/app/remote?with=${encodeURIComponent(p.agent.id)}`}>New chat</a>
      </div>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}
      <section className="plain-list" aria-label="About">
        <div><span>Runs on</span><span>{agentName(p.agent.host)}</span></div>
        {modelLabel(p.model) && <div><span>Model</span><span>{modelLabel(p.model)}</span></div>}
        <div><span>Project</span><span>{p.agent.project.name}</span></div>
      </section>
      <section className="record" aria-label={`Last ${r.days} days`}>
        <h2>Last {r.days} days</h2>
        <dl>
          <div><dd>{r.score.percent === null ? "–" : `${r.score.percent}%`}</dd><dt>On track</dt></div>
          <div><dd>{r.chats}</dd><dt>Chats</dt></div>
          <div><dd>{r.blocked}</dd><dt>Stopped</dt></div>
        </dl>
        {r.partial && <p className="quiet-note">Counted from the latest steps.</p>}
      </section>
      <section className="plain-list as-buttons" aria-label="Manage">
        {p.agent.host !== "cursor" && <button type="button" onClick={() => setSheet("position")}>Set position</button>}
        <button type="button" onClick={() => setSheet("name")}>Rename</button>
        <button type="button" className="danger" onClick={() => setSheet("remove")}>Remove from team</button>
      </section>
      {p.chats.length > 0 && <section aria-label="Chats" className="chat-list-rows">{p.chats.map((c) => <SessionRow key={c.id} s={c} project={c.project.name} />)}</section>}
      {sheet === "name" && <NameSheet label="Name" value={p.agent.name} onClose={() => setSheet(null)}
        onSave={(name) => act(api(path, { method: "PATCH", body: JSON.stringify({ name }) }), load)} />}
      {sheet === "position" && <PositionSheet title={p.agent.title} description={p.agent.description} onClose={() => setSheet(null)}
        onSave={(body) => act(api(path, { method: "PATCH", body: JSON.stringify(body) }), load)} />}
      {sheet === "remove" && <ConfirmSheet title={`Remove ${p.agent.title ?? p.agent.name} from your team? Its chats stay.`} action="Remove from team" onClose={() => setSheet(null)}
        onConfirm={() => act(api(path, { method: "DELETE" }), () => window.location.assign("/app/team"))} />}
    </div>
  );
}
