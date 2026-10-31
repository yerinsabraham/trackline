"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api } from "@/lib/account";
import { type Member, modelLabel } from "@/lib/dashboard";
import AgentLogo, { agentName } from "./AgentLogo";

// Who a chat is with, opened by tapping the name. Most chats' agents are not on
// the team, so this is where one joins it, or joins a member it really belongs
// with. What it does goes by ids: the name is only what the agent is called.

export default function AgentSheet({ chat, host, name, model, agentId, project, onClose, onChanged }: {
  chat: string; host: string; name: string; model?: string | null; agentId?: string | null; project: string;
  onClose: () => void; onChanged: () => void;
}) {
  const [mode, setMode] = useState<"menu" | "name" | "move">("menu");
  const [value, setValue] = useState(name);
  const [team, setTeam] = useState<Member[] | null>(null);
  const [error, setError] = useState("");
  const path = `/app/sessions/${encodeURIComponent(chat)}`;
  const act = async (work: () => Promise<unknown>) => {
    try { await work(); onChanged(); onClose(); } catch (e) { setError((e as Error).message); }
  };
  useEffect(() => {
    if (mode === "move") api<{ team: Member[] }>("/app/team").then((r) => setTeam(r.team)).catch((e) => setError((e as Error).message));
  }, [mode]);
  const rename = () => act(() => (agentId
    ? api(`/app/team/${encodeURIComponent(agentId)}`, { method: "PATCH", body: JSON.stringify({ name: value }) })
    : api(path, { method: "PATCH", body: JSON.stringify({ agentName: value }) })));
  const same = team?.filter((m) => m.host === host && m.project.id === project && m.id !== agentId) ?? [];
  return (
    <div className="sheet-back" onClick={onClose}>
      <div className="sheet" role="dialog" aria-label={`Agent: ${name}`} onClick={(e) => e.stopPropagation()}>
        {mode === "name" ? (
          <form onSubmit={(e) => { e.preventDefault(); rename(); }}>
            <label htmlFor="agent-name" className="sheet-label">Name</label>
            <input id="agent-name" className="sheet-input" autoFocus value={value} maxLength={60} onChange={(e) => setValue(e.target.value)} />
            <button type="submit" className="btn-act sheet-wide" disabled={!value.trim()}>Save</button>
          </form>
        ) : mode === "move" ? (
          <>
            <strong className="sheet-title">Move this chat to</strong>
            {team && same.length === 0 && <p className="sheet-note">Nobody on your team works with {agentName(host)} in this project yet.</p>}
            {same.map((m) => <button key={m.id} type="button" className="sheet-item" onClick={() => act(() => api(`${path}/move`, { method: "POST", body: JSON.stringify({ agent: m.id }) }))}>{m.name}</button>)}
          </>
        ) : (
          <>
            <div className="agent-head">
              <AgentLogo host={host} size={36} />
              <div>
                <strong className="sheet-title">{name}</strong>
                <span className="row-meta">AI agent · {[agentName(host), modelLabel(model)].filter(Boolean).join(" · ")}</span>
              </div>
            </div>
            {agentId
              ? <Link className="sheet-item" href={`/app/agent?id=${encodeURIComponent(agentId)}`}>View profile</Link>
              : <button type="button" className="sheet-item" onClick={() => act(() => api(`${path}/team`, { method: "POST", body: "{}" }))}>Add to team</button>}
            <button type="button" className="sheet-item" onClick={() => setMode("name")}>Rename</button>
            <button type="button" className="sheet-item" onClick={() => setMode("move")}>Move to…</button>
            {agentId && <button type="button" className="sheet-item" onClick={() => act(() => api(`${path}/move`, { method: "POST", body: JSON.stringify({ agent: null }) }))}>Take this chat out</button>}
          </>
        )}
        {error && <p className="composer-note error">{error}</p>}
      </div>
    </div>
  );
}
