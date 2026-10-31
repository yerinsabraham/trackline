"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/account";
import { titled, whileQuiet } from "@/lib/dashboard";
import { type Handoff, proof, signHandoff } from "@/lib/remote";
import { agentName } from "./AgentLogo";
import { usePasskeys } from "./Chat";

// One agent asking another, waiting on the person. The whole task is shown as
// written, never a summary: a summary is how a bad request gets approved.

const ENDED: Record<string, string> = { declined: "You declined this.", expired: "Nobody approved this in time, so it did not run.", failed: "It could not finish." };

function Card({ h, keys, onDone }: { h: Handoff; keys: ReturnType<typeof usePasskeys>; onDone: () => void }) {
  const [back, setBack] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const asker = h.from.name ? titled(h.from.name, h.from.title) : agentName(h.from.agent);
  const to = titled(h.to.name, h.to.title);
  const act = async (work: () => Promise<unknown>) => {
    setBusy(true); setError("");
    try { await work(); onDone(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };
  const path = `/app/remote/handoffs/${encodeURIComponent(h.id)}`;
  return (
    <section className="card handoff rise" aria-label={`Handoff ${h.number}`}>
      <p className="handoff-head"><strong>{asker}</strong> asks <strong>{to}</strong></p>
      <p className="row-meta">In {h.project} · {h.sign.session ? `continues ${h.to.name}'s last chat, with what it holds` : "a new chat"}</p>
      <pre className="handoff-task">{h.task}</pre>
      {h.reason && <p className="composer-note warn" style={{ textAlign: "left" }}>{h.reason}</p>}
      {h.from.working ? <p className="row-meta">{asker} is still working. You can approve this when it stops.</p> : (
        <>
          <label className="handoff-back"><input type="checkbox" checked={back} onChange={(e) => setBack(e.target.checked)} /> Send the result back to {asker} once</label>
          <div className="handoff-actions">
            <button type="button" className="btn-act" disabled={busy || !!keys.missing} onClick={() => act(async () => {
              const body = await signHandoff(h, await keys.get(), back);
              await api(`${path}/approve`, { method: "POST", body: JSON.stringify(body) });
            })}>{busy ? `Waiting for ${proof()}…` : "Approve"}</button>
            <button type="button" className="btn-plain" disabled={busy} onClick={() => act(() => api(`${path}/decline`, { method: "POST", body: "{}" }))}>Decline</button>
          </div>
        </>
      )}
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}
    </section>
  );
}

export default function HandoffCards() {
  const [list, setList] = useState<Handoff[]>([]);
  const keys = usePasskeys();
  const load = useCallback(() => { api<{ handoffs: Handoff[] }>("/app/remote/handoffs").then((r) => setList(r.handoffs)).catch(() => {}); }, []);
  useEffect(() => { load(); return whileQuiet(load, 8000, 30_000); }, [load]);
  const waiting = list.filter((h) => h.status === "asked");
  const ended = list.filter((h) => h.status !== "asked" && h.status !== "approved").slice(0, 3);
  const running = list.filter((h) => h.status === "approved");
  return (
    <>
      {waiting.map((h) => <Card key={h.id} h={h} keys={keys} onDone={load} />)}
      {running.map((h) => <p key={h.id} className="handoff-line"><strong>{titled(h.to.name, h.to.title)}</strong> is working on what was asked.</p>)}
      {ended.map((h) => (
        <p key={h.id} className="handoff-line">
          <strong>{titled(h.to.name, h.to.title)}</strong> {h.status === "done" ? "finished what was asked." : ENDED[h.status] ?? h.status}
          {h.job && <> <a href={`/app/remote?job=${encodeURIComponent(h.job)}`}>See it</a></>}
        </p>
      ))}
    </>
  );
}
