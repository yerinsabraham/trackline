"use client";

import Link from "next/link";
import type { MouseEvent } from "react";
import { ago, LIGHT_LABEL, modelLabel, type SessionSummary, sessionName, titled } from "@/lib/dashboard";
import AgentLogo, { agentName } from "./AgentLogo";

/** One chat: who it is with, what it is about, how it stands. Three lines, no more. */
export function SessionRow({ s, project, current, onClick, unread }: { s: SessionSummary; project?: string; current?: boolean; onClick?: (e: MouseEvent) => void; unread?: boolean }) {
  const title = sessionName(s, agentName(s.host), project);
  return (
    <Link href={`/app/session?id=${encodeURIComponent(s.id)}`} className="chat-row" aria-current={current ? "true" : undefined} onClick={onClick}>
      <AgentLogo host={s.host} size={46} />
      <span className="chat-row-main">
        <span className="chat-row-top">
          <strong>{s.agentName ? titled(s.agentName, s.agentTitle) : title}</strong>
          {unread && <span className="dot-unread" role="img" aria-label="New messages" />}
          <time>{ago(s.lastAt)}</time>
        </span>
        {s.agentName && <span className="chat-row-title">{s.pinnedAt && <span className="row-pin">Pinned · </span>}{title}</span>}
        <span className="chat-row-state"><b className={`state-${s.light}`}>{s.waiting ? "Waiting for you" : LIGHT_LABEL[s.light]}</b>{modelLabel(s.model) ? ` · ${modelLabel(s.model)}` : ""}</span>
      </span>
    </Link>
  );
}
