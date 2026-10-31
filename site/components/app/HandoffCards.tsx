"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/account";
import { titled, whileQuiet } from "@/lib/dashboard";
import type { Handoff } from "@/lib/remote";
import { agentName } from "./AgentLogo";

// One agent asking another. A card says it is waiting; the words themselves are
// read in full on their own screen, never in a summary.

export function useHandoffs() {
  const [list, setList] = useState<Handoff[]>([]);
  const load = useCallback(() => { api<{ handoffs: Handoff[] }>("/app/remote/handoffs").then((r) => setList(r.handoffs)).catch(() => {}); }, []);
  useEffect(() => { load(); return whileQuiet(load, 8000, 30_000); }, [load]);
  return { list, load };
}

export const askerOf = (h: Handoff) => (h.from.title ?? h.from.name ?? agentName(h.from.agent));
export const receiverOf = (h: Handoff) => (h.to.title ?? h.to.name);

export function WaitingCards({ list }: { list: Handoff[] }) {
  return (
    <>
      {list.map((h) => (
        <Link key={h.id} href={`/app/approve?id=${encodeURIComponent(h.id)}`} className="ok-card">
          <span><strong>{askerOf(h)} asks {receiverOf(h)}</strong><small>Needs your OK · {h.project}</small></span>
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true" style={{ flex: "none", width: 20, height: 20 }}><path d="m9 18 6-6-6-6" /></svg>
        </Link>
      ))}
    </>
  );
}

const ENDED: Record<string, string> = { declined: "You declined this.", expired: "Nobody approved this in time, so it did not run.", failed: "It could not finish." };

/** What became of the last few: on the chats list, out of the way. */
export function HandoffNews({ list }: { list: Handoff[] }) {
  const running = list.filter((h) => h.status === "approved");
  const ended = list.filter((h) => h.status !== "asked" && h.status !== "approved").slice(0, 2);
  return (
    <>
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
