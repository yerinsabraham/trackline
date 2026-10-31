"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { api } from "@/lib/account";
import { proof, signHandoff } from "@/lib/remote";
import AgentLogo from "@/components/app/AgentLogo";
import { usePasskeys } from "@/components/app/Chat";
import { askerOf, receiverOf, useHandoffs } from "@/components/app/HandoffCards";
import { IconBack } from "@/components/app/icons";

// One agent asking another, on a screen of its own. The whole task is shown as
// written: a summary is how a bad request gets approved. Everything the
// signature binds that a person would care about is said here.

export default function Approve() {
  const id = useSearchParams().get("id") ?? "";
  const router = useRouter();
  const { list } = useHandoffs();
  const keys = usePasskeys();
  const h = list.find((x) => x.id === id);
  const [back, setBack] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const path = `/app/remote/handoffs/${encodeURIComponent(id)}`;
  const act = async (work: () => Promise<unknown>) => {
    setBusy(true); setError("");
    try { await work(); router.push("/app"); } catch (e) { setError((e as Error).message); setBusy(false); }
  };
  return (
    <div className="approve">
      <header><Link href="/app" className="app-icon-btn" aria-label="Back"><IconBack size={24} /></Link></header>
      <main>
        <h1 className="app-h1">Needs your OK</h1>
        {!h && <p className="app-sub">{list.length ? "This one is no longer waiting." : "Loading…"}</p>}
        {h && (
          <>
            <div className="approve-who">
              <AgentLogo host={h.from.agent} size={42} />
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true" style={{ width: 20, height: 20, flex: "none" }}><path d="M5 12h14M13 6l6 6-6 6" /></svg>
              <AgentLogo host={h.to.host} size={42} />
              <span><strong>{askerOf(h)} asks {receiverOf(h)}</strong><small>{h.project} · {h.sign.session ? `continues ${h.to.name}'s last chat` : "a new chat"}</small></span>
            </div>
            <pre className="approve-task" aria-label="The whole request">{h.task}</pre>
            {h.reason && <p className="composer-note warn" style={{ textAlign: "left" }}>{h.reason}</p>}
            {h.from.working
              ? <p className="app-sub">{askerOf(h)} is still working. You can approve this when it stops.</p>
              : <label className="approve-back"><input type="checkbox" checked={back} onChange={(e) => setBack(e.target.checked)} /> Send the result back to {askerOf(h)}</label>}
            {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}
          </>
        )}
      </main>
      {h && h.status === "asked" && (
        <footer>
          {!h.from.working && <button type="button" className="approve-yes" disabled={busy || !!keys.missing} onClick={() => act(async () => {
            const body = await signHandoff(h, await keys.get(), back);
            await api(`${path}/approve`, { method: "POST", body: JSON.stringify(body) });
          })}>{busy ? `Waiting for ${proof()}…` : "Approve"}</button>}
          <button type="button" className="approve-no" disabled={busy} onClick={() => act(() => api(`${path}/decline`, { method: "POST", body: "{}" }))}>Decline</button>
        </footer>
      )}
    </div>
  );
}
