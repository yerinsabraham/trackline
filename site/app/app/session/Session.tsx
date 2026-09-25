"use client";

import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/account";
import { LIGHT_LABEL, type FeedEvent, type Reply, scoreLine, type SessionView, whileVisible } from "@/lib/dashboard";
import { active, jobState, type Job, type Passkey, signPrompt } from "@/lib/remote";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import { useApp } from "@/components/app/AppShell";
import { Activity, AgentReply, Composer, Finding, type Line, MyMessage } from "@/components/app/Chat";
import { IconBack } from "@/components/app/icons";
import { SessionRow } from "../Overview";

// One session as a conversation: what you asked, what the agent did (as a
// terminal would show it), anything trackline stopped, and the agent's reply.
// Underneath, a box that continues the same session on your laptop.

const VERB: Record<string, string> = {
  "write-file": "Write", "edit-file": "Edit", "delete-file": "Delete", "read-file": "Read",
  "run-command": "Run", "call-tool": "Tool", other: "Act",
};
const time = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });

function line(e: FeedEvent): Line {
  const target = e.installs.length ? `install ${e.installs.join(", ")}` : e.paths.length ? e.paths.slice(0, 3).join(", ") + (e.paths.length > 3 ? ` +${e.paths.length - 3}` : "") : e.tool ?? "";
  const tone = e.blocked || e.severity === "block" ? "block" : e.severity === "warn" ? "warn" : undefined;
  return {
    key: e.id, time: time(e.at), text: target,
    verb: tone === "block" ? "Blocked" : e.action === "call-tool" && e.tool ? e.tool : VERB[e.action] ?? "Act",
    tone, why: tone ? e.findings[0]?.summary : undefined,
  };
}

type Turn = { key: string; request: string | null; events: FeedEvent[]; reply?: Reply };

function turns(events: FeedEvent[], replies: Reply[]): Turn[] {
  const out: Turn[] = [];
  for (const e of [...events].sort((a, b) => +new Date(a.at) - +new Date(b.at))) {
    const k = e.turn ?? e.request ?? "";
    const last = out[out.length - 1];
    if (last && last.key === k) last.events.push(e);
    else out.push({ key: k, request: e.request, events: [e] });
  }
  const byTurn = new Map(out.map((t) => [t.key, t]));
  const loose: Reply[] = [];
  for (const r of replies) {
    const t = r.turn ? byTurn.get(r.turn) : undefined;
    if (t) t.reply = r; else loose.push(r);
  }
  // A reply with no actions of its own (a question answered in words) is a
  // turn of its own, placed by when it came.
  for (const r of loose) out.push({ key: `reply-${r.at}`, request: null, events: [], reply: r });
  return out.sort((a, b) => +new Date(a.events[0]?.at ?? a.reply!.at) - +new Date(b.events[0]?.at ?? b.reply!.at));
}

type Sent = { job: string; text: string; status: string; code: string | null; reason: string | null };

export default function Session() {
  const { projects } = useApp();
  const [id, setId] = useState("");
  const [view, setView] = useState<SessionView | null>(null);
  const [events, setEvents] = useState<FeedEvent[]>([]);
  const [replies, setReplies] = useState<Reply[]>([]);
  const [error, setError] = useState("");
  const [sent, setSent] = useState<Sent[]>([]);
  const cursor = useRef<string | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const keys = useRef<Passkey[] | null>(null);

  useEffect(() => {
    const sid = new URLSearchParams(window.location.search).get("id") ?? "";
    setId(sid);
    let busy = false;
    const load = async () => {
      if (busy) return;
      busy = true;
      try {
        const after = cursor.current ? `?after=${encodeURIComponent(cursor.current)}` : "";
        const r = await api<SessionView>(`/app/sessions/${encodeURIComponent(sid)}${after}`);
        if (!cursor.current) setView(r); else setView((v) => (v ? { ...v, session: r.session } : r));
        cursor.current = r.cursor;
        if (r.events.length) setEvents((old) => [...new Map([...old, ...r.events].map((e) => [e.id, e])).values()]);
        if (r.replies.length) setReplies((old) => [...new Map([...old, ...r.replies].map((x) => [x.id ?? x.at, x])).values()]);
        setError("");
      } catch (e) {
        if ((e as { status?: number }).status === 404) setError("This session does not exist, or is not yours.");
      } finally {
        busy = false;
      }
    };
    load();
    return whileVisible(load, 2500);
  }, []);

  // Follow the conversation as it grows, unless you have scrolled up to read.
  useEffect(() => {
    const el = scroller.current;
    if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 240) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  }, [events.length, replies.length, sent.length]);

  // Tasks sent from the box: followed until the laptop says how they went.
  useEffect(() => {
    if (!sent.some((s) => active(s.status))) return;
    const t = setInterval(async () => {
      const next = await Promise.all(sent.map(async (s) => {
        if (!active(s.status)) return s;
        const j = await api<Job>(`/app/remote/jobs/${encodeURIComponent(s.job)}`).catch(() => null);
        return j ? { ...s, status: j.status, code: j.code, reason: j.reason } : s;
      }));
      setSent(next);
    }, 1500);
    return () => clearInterval(t);
  }, [sent]);

  const s = view?.session;
  const cont = view?.continueWith ?? null;
  const siblings = (projects ?? []).flatMap((p) => p.sessions.map((x) => ({ ...x, project: p.name })))
    .filter((x) => !s || x.host === s.host).sort((a, b) => +new Date(b.lastAt) - +new Date(a.lastAt));
  const list = turns(events, replies);
  const working = s?.light === "working" || sent.some((x) => active(x.status));

  const blocked = !s ? "Loading…"
    : !cont ? (s.host === "cursor" ? "Replying to Cursor from here is not available yet." : "To reply from here, turn on remote for this project on your laptop: trackline remote enable")
    : cont.machine.stopped ? `Remote is stopped on ${cont.machine.name}. Turn it on again on the laptop.`
    : undefined;

  const send = async (text: string) => {
    if (!cont) return;
    if (!keys.current) keys.current = (await api<{ passkeys: Passkey[] }>("/app/remote/passkeys")).passkeys;
    if (!keys.current.length) throw new Error("Add a passkey on your account page first: every message is signed with it.");
    const body = await signPrompt(cont.machine.id, cont.project, cont.agent, text, keys.current, cont.session);
    const r = await api<{ id: string }>("/app/remote/jobs", { method: "POST", body: JSON.stringify(body) });
    setSent((x) => [...x, { job: r.id, text, status: "queued", code: null, reason: null }]);
  };

  return (
    <div className="chat-page">
      <aside className="chat-list" aria-label={`${agentName(s?.host)} sessions`}>
        <div className="chat-list-head">
          {s && <AgentLogo host={s.host} size={28} />}
          <h2>{agentName(s?.host)}</h2>
        </div>
        {siblings.map((x) => <SessionRow key={x.id} s={x} project={x.project} current={x.id === id} />)}
      </aside>

      <section className="chat-pane">
        <header className="chat-head">
          <a href={s ? `/app/sessions?agent=${s.host}` : "/app/sessions"} className="app-icon-btn chat-back" aria-label="Back to sessions"><IconBack /></a>
          {s && <AgentLogo host={s.host} size={32} />}
          <div style={{ display: "grid", gap: 2, minWidth: 0, flexGrow: 1 }}>
            <h1>{s?.request ?? (error ? "Session" : "Loading…")}</h1>
            <span className="row-meta">{s ? `${agentName(s.host)} · ${s.project.name}${cont ? ` · ${cont.machine.name}` : ""}` : ""}</span>
          </div>
          {s && <span className={`pill ${s.light}`}>{s.light === "working" && <span className="dot working" />}{LIGHT_LABEL[s.light]}</span>}
        </header>

        <div className="chat-scroll" ref={scroller}>
          <div className="convo">
            {error && <p className="composer-note error">{error}</p>}
            {!view && !error && [0, 1, 2].map((i) => <div key={i} className="skel" style={{ height: i === 1 ? 120 : 56 }} />)}
            {s && <p className="chat-divider">{scoreLine(s.score.session)}</p>}
            {view && list.length === 0 && <p className="chat-divider">No actions in the last 30 days.</p>}
            {list.map((t, i) => {
              const last = i === list.length - 1;
              const stops = t.events.filter((e) => e.findings.length && (e.blocked || e.severity));
              return (
                <div key={t.key || i} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
                  {t.request && <MyMessage text={t.request} />}
                  <Activity lines={t.events.map(line)} live={last && working && !t.reply} />
                  {stops.slice(0, 3).map((e) => (
                    <Finding key={e.id} tone={e.blocked || e.severity === "block" ? "block" : "warn"} title={e.findings[0].summary}>
                      {e.findings[0].suggestion ?? (e.blocked ? `${agentName(s?.host)} was told why and carried on.` : undefined)}
                    </Finding>
                  ))}
                  {t.reply && <AgentReply host={s?.host ?? null} text={t.reply.text} />}
                </div>
              );
            })}
            {sent.map((x) => (
              <MyMessage key={x.job} text={x.text} note={`${cont?.machine.name ?? "Laptop"} · ${jobState(x)}`} />
            ))}
          </div>
        </div>

        <div className="chat-compose">
          <Composer
            placeholder={`Reply to ${agentName(s?.host)}…`}
            blocked={blocked}
            note={cont ? `Signed with your passkey · continues this session on ${cont.machine.name}${cont.machine.online ? "" : " (asleep: it waits 3 minutes)"}` : undefined}
            onSend={send}
          />
        </div>
      </section>
    </div>
  );
}
