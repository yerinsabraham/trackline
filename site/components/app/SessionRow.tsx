"use client";

import Link from "next/link";
import type { MouseEvent } from "react";
import { ago, LIGHT_LABEL, type SessionSummary, sessionName, titled } from "@/lib/dashboard";
import { plain } from "@/components/ReplyText";
import AgentLogo, { agentName } from "./AgentLogo";

/** One chat in a list: who it is with, in bold, then what it is about. */
export function SessionRow({ s, project, current, onClick, unread }: { s: SessionSummary; project?: string; current?: boolean; onClick?: (e: MouseEvent) => void; unread?: boolean }) {
  const note = s.reply ? plain(s.reply.text) : null;
  const name = sessionName(s, agentName(s.host), project);
  return (
    <Link href={`/app/session?id=${encodeURIComponent(s.id)}`} className="row-link" aria-current={current ? "true" : undefined} onClick={onClick}>
      <AgentLogo host={s.host} size={34} />
      <span className="row-main">
        <span className="row-top">
          <strong>{s.pinnedAt && <span className="row-pin">Pinned · </span>}{s.agentName ? titled(s.agentName, s.agentTitle) : name}</strong>
          {unread && <span className="dot-unread" role="img" aria-label="New messages" />}
          <span className={`pill ${s.light}`}>{s.light === "working" && <span className="dot working" />}{LIGHT_LABEL[s.light]}</span>
        </span>
        {s.agentName && <span className="row-title">{name}</span>}
        <span className="row-meta">{project ? `${project} · ` : ""}{ago(s.lastAt)}</span>
        {note && <span className="row-note">{note}</span>}
      </span>
    </Link>
  );
}
