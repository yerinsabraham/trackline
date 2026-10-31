"use client";

import Link from "next/link";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { api, markRead, readAt, readDraft, saveDraft } from "@/lib/account";
import { LIGHT_LABEL, type FeedEvent, type Reply, requestLabel, scoreLine, sessionName, type SessionJob, type SessionView, whileQuiet } from "@/lib/dashboard";
import { active, type Attachment, proof, imagesFor, jobState, previewsFor, recall, remember, rememberImages, signDecision, signPrompt } from "@/lib/remote";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import { useApp } from "@/components/app/AppShell";
import { Activity, AgentReply, Composer, HelperLine, Finding, JumpToNewest, LaptopNotice, type Line, MyMessage, PasskeyNeeded, Question, ResumeHere, SendAgain, useChatScroll, usePasskeys } from "@/components/app/Chat";
import { plain } from "@/components/ReplyText";
import { IconBack, IconMore, IconStop } from "@/components/app/icons";
import AgentSheet from "@/components/app/AgentSheet";
import SessionMenu from "@/components/app/SessionMenu";
import { SessionRow } from "@/components/app/SessionRow";

// One session as a conversation: what you asked, what the agent did (as a
// terminal would show it), anything trackline stopped, and the agent's reply.
// Underneath, a box that continues the same session on your laptop.

const VERB: Record<string, string> = {
  "write-file": "Write", "edit-file": "Edit", "delete-file": "Delete", "read-file": "Read",
  "run-command": "Run", "call-tool": "Tool", other: "Act",
};
const time = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });

function toLine(e: FeedEvent): Line {
  const target = e.installs.length ? `install ${e.installs.join(", ")}` : e.paths.length ? e.paths.slice(0, 3).join(", ") + (e.paths.length > 3 ? ` +${e.paths.length - 3}` : "") : e.tool ?? "";
  const tone = e.blocked || e.severity === "block" ? "block" : e.severity === "warn" ? "warn" : undefined;
  return {
    key: e.id, time: time(e.at), text: target,
    verb: tone === "block" ? "Blocked" : e.action === "call-tool" && e.tool ? e.tool : VERB[e.action] ?? "Act",
    tone, why: tone ? e.findings[0]?.summary : undefined,
  };
}

type Turn = { key: string; request: string | null; events: FeedEvent[]; replies: Reply[] };

// A reply carries more of the request than an action can.
const fuller = (a: string | null, b?: string | null) => ((b?.length ?? 0) > (a?.length ?? 0) ? b ?? null : a);
const startOf = (t: Turn) => +new Date(t.events[0]?.at ?? t.replies[0].at);

/** Exported for its test: this is the rule that keeps a conversation whole. */
export function turns(events: FeedEvent[], replies: Reply[]): Turn[] {
  const out: Turn[] = [];
  for (const e of [...events].sort((a, b) => +new Date(a.at) - +new Date(b.at))) {
    const k = e.turn ?? e.request ?? "";
    const last = out[out.length - 1];
    if (last && last.key === k) last.events.push(e);
    else out.push({ key: k, request: e.request, events: [e], replies: [] });
  }
  const byTurn = new Map(out.map((t) => [t.key, t]));
  for (const r of [...replies].sort((a, b) => +new Date(a.at) - +new Date(b.at))) {
    // A turn answered in words has no actions, so its reply starts it.
    const key = r.turn ?? `reply-${r.id ?? r.at}`;
    let t = byTurn.get(key);
    if (!t) {
      t = { key, request: null, events: [], replies: [] };
      byTurn.set(key, t);
      out.push(t);
    }
    t.replies.push(r);
    t.request = fuller(t.request, r.request);
  }
  return out.sort((a, b) => startOf(a) - startOf(b));
}

type Sent = { job: string; text: string; createdAt: string };

function dayOf(at: number): string {
  const d = new Date(at), now = new Date();
  const days = Math.round((new Date(now).setHours(0, 0, 0, 0) - new Date(d).setHours(0, 0, 0, 0)) / 86_400_000);
  return days === 0 ? "Today" : days === 1 ? "Yesterday" : d.toLocaleDateString([], { weekday: "short", day: "numeric", month: "short" });
}

// Which conversation turn a sent message became. The whole message, or the
// start of it, as the words can gain a note about attached images on the way:
// anywhere inside would pin a short "ok" to the wrong turn. The clock on the
// laptop can differ from the server's by a little. If the hook cleaned the words
// this misses, and the message shows twice, which beats showing it nowhere.
const SKEW_MS = 10_000;
function turnOf(list: Turn[], text: string | null, createdAt: string) {
  if (!text) return undefined;
  const from = +new Date(createdAt) - SKEW_MS;
  return list.find((t) => startOf(t) >= from && (t.request === text || !!t.request?.startsWith(`${text}\n`)));
}

export default function Session() {
  const { projects } = useApp();
  const [id, setId] = useState("");
  const [view, setView] = useState<SessionView | null>(null);
  const [events, setEvents] = useState<FeedEvent[]>([]);
  const [replies, setReplies] = useState<Reply[]>([]);
  const [error, setError] = useState("");
  const [sent, setSent] = useState<Sent[]>([]);
  const [held, setHeld] = useState<{ text: string; images: Attachment[] } | null>(null);
  const [heldError, setHeldError] = useState("");
  const [heldBusy, setHeldBusy] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [more, setMore] = useState(false);
  const [menu, setMenu] = useState(false);
  const [who, setWho] = useState(false);
  const [unreadAt, setUnreadAt] = useState<number | null>(null);
  // Decisions made here, by the action they were about.
  const [decided, setDecided] = useState<Record<string, "allow" | "deny">>({});
  const [deciding, setDeciding] = useState("");
  const [decideError, setDecideError] = useState("");
  const cursor = useRef<string | null>(null);
  const { scrollRef, contentRef, isAtBottom, scrollToBottom, stopScroll } = useChatScroll();
  const passkeys = usePasskeys();
  const line = useRef<HTMLDivElement>(null);
  const top = useRef<HTMLButtonElement>(null);
  const loadingEarlier = useRef(false);
  // The height before older messages went in, so you stay on what you were reading.
  const before = useRef<number | null>(null);

  useEffect(() => {
    const sid = new URLSearchParams(window.location.search).get("id") ?? "";
    setId(sid);
    const waiting = readDraft(`held:${sid}`);
    if (waiting) setHeld({ text: waiting, images: [] });
    let busy = false;
    const load = async () => {
      if (busy) return;
      busy = true;
      try {
        const after = cursor.current ? `?after=${encodeURIComponent(cursor.current)}` : "";
        const r = await api<SessionView>(`/app/sessions/${encodeURIComponent(sid)}${after}`);
        // The laptop can come and go while this page stays open for days, so
        // where a reply would go is read fresh every time.
        if (!cursor.current) {
          setView(r);
          setMore(!!r.more);
          // Away since the last visit: open where the new messages start.
          const seen = readAt(sid);
          const first = r.replies.map((x) => +new Date(x.at)).filter((t) => seen !== null && t > seen).sort((a, b) => a - b)[0];
          if (first) setUnreadAt(first);
        } else {
          setView((v) => (v ? { ...v, session: r.session, jobs: r.jobs ?? v.jobs, continueWith: r.continueWith === undefined ? v.continueWith : r.continueWith } : r));
        }
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
    return whileQuiet(load, 2500, 15_000);
  }, []);

  useLayoutEffect(() => {
    if (unreadAt === null || !line.current || !scrollRef.current) return;
    const el = scrollRef.current;
    // After the page's own jump to the bottom, which this replaces.
    const place = () => {
      stopScroll();
      el.scrollTop = line.current ? line.current.offsetTop - el.offsetTop - 56 : el.scrollTop;
    };
    place();
    const t = setTimeout(place, 120);
    return () => clearTimeout(t);
    // Once, for the line as it opened.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [unreadAt]);

  // What you have reached counts as read; what arrives while you read back up
  // does not.
  const newest = replies.reduce((m, r) => Math.max(m, +new Date(r.at)), 0);
  useEffect(() => {
    if (id && newest && isAtBottom && document.visibilityState === "visible") markRead(id, newest);
  }, [id, newest, isAtBottom]);

  const earlier = useCallback(async () => {
    if (loadingEarlier.current) return;
    const oldest = [...events, ...replies].reduce((m, x) => Math.min(m, +new Date(x.at)), Infinity);
    if (!Number.isFinite(oldest)) return;
    loadingEarlier.current = true;
    try {
      const r = await api<SessionView>(`/app/sessions/${encodeURIComponent(id)}?before=${encodeURIComponent(new Date(oldest).toISOString())}`);
      before.current = scrollRef.current?.scrollHeight ?? null;
      setEvents((old) => [...new Map([...r.events, ...old].map((e) => [e.id, e])).values()]);
      setReplies((old) => [...new Map([...r.replies, ...old].map((x) => [x.id ?? x.at, x])).values()]);
      setMore(!!r.more);
    } catch { /* the button stays, and a tap tries again */ } finally {
      loadingEarlier.current = false;
    }
  }, [events, replies, id, scrollRef]);

  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (before.current === null || !el) return;
    el.scrollTop += el.scrollHeight - before.current;
    before.current = null;
  }, [events, replies, scrollRef]);

  useEffect(() => {
    const el = top.current;
    if (!el || !more) return;
    const seen = new IntersectionObserver((e) => { if (e[0].isIntersecting) earlier(); }, { root: scrollRef.current, rootMargin: "200px 0px 0px 0px" });
    seen.observe(el);
    return () => seen.disconnect();
  }, [more, earlier, scrollRef]);

  const s = view?.session;
  const cont = view?.continueWith ?? null;
  const siblings = (projects ?? []).flatMap((p) => p.sessions.map((x) => ({ ...x, project: p.name })))
    .filter((x) => !s || x.host === s.host).sort((a, b) => +new Date(b.lastAt) - +new Date(a.lastAt));
  const list = turns(events, replies);
  // From the server, so every device agrees. A message sent here counts as
  // queued until the next poll has it.
  const known = view?.jobs ?? [];
  const jobs: SessionJob[] = [
    ...known,
    ...sent.filter((x) => !known.some((j) => j.id === x.job)).map((x) => ({ id: x.job, kind: "prompt", status: "queued", code: null, reason: null, createdAt: x.createdAt, stopRequestedAt: null, asked: null, permission: null, saying: null })),
  ];
  const running = jobs.find((j) => active(j.status));
  const working = s?.light === "working" || !!running;
  const textOf = (j: SessionJob) => sent.find((x) => x.job === j.id)?.text ?? recall(j.id) ?? null;
  const asked = !working ? jobs[jobs.length - 1]?.asked ?? null : null;

  const blocked = !s ? "Loading…"
    : !cont ? "One step left. On your laptop, run: trackline remote enable"
    : cont.machine.stopped ? `Stopped on ${cont.machine.name}. To start again, run there: trackline remote enable`
    : passkeys.missing ? "Add a passkey to send."
    : undefined;

  // Allow once, or keep blocked: signed here, decided on the laptop, and the
  // agent is told in this same session.
  const decide = async (e: FeedEvent, kind: "allow" | "deny") => {
    const f = e.findings[0];
    if (!cont || !f?.target) return;
    setDeciding(e.id); setDecideError("");
    try {
      const body = await signDecision(cont.machine.id, cont.project, cont.agent, cont.session, kind, f.check, f.target, await passkeys.get());
      const r = await api<{ id: string }>("/app/remote/jobs", { method: "POST", body: JSON.stringify(body) });
      setDecided((d) => ({ ...d, [e.id]: kind }));
      setSent((x) => [...x, { job: r.id, text: kind === "allow" ? `Allow once: ${f.target}` : `Keep blocked: ${f.target}`, createdAt: new Date().toISOString() }]);
    } catch (err) {
      setDecideError((err as Error).message);
    } finally {
      setDeciding("");
    }
  };

  const deliver = async (text: string, images: Attachment[] = []) => {
    if (!cont) return;
    const body = await signPrompt(cont.machine.id, cont.project, cont.agent, text, await passkeys.get(), cont.session, images, !cont.machine.online);
    const r = await api<{ id: string }>("/app/remote/jobs", { method: "POST", body: JSON.stringify(body) });
    remember(r.id, text);
    rememberImages(r.id, images);
    setSent((x) => [...x, { job: r.id, text, createdAt: new Date().toISOString() }]);
  };

  // The laptop runs one prompt per project and refuses a second, and a signed
  // message cannot outlast the agent's work. So a message written while it
  // works waits here, and Face ID sends it when it has finished.
  const keep = (next: { text: string; images: Attachment[] } | null) => {
    setHeld(next);
    saveDraft(`held:${id}`, next?.text ?? "");
  };
  const send = async (text: string, images: Attachment[] = []) => {
    if (!working) return deliver(text, images);
    keep({ text: held ? `${held.text}\n\n${text}` : text, images: [...(held?.images ?? []), ...images] });
  };
  const sendHeld = async () => {
    if (!held) return;
    setHeldBusy(true); setHeldError("");
    try {
      await deliver(held.text, held.images);
      keep(null);
    } catch (e) {
      setHeldError((e as Error).message);
    } finally {
      setHeldBusy(false);
    }
  };
  const refresh = () => { api<SessionView>(`/app/sessions/${encodeURIComponent(id)}?after=${encodeURIComponent(cursor.current ?? "")}`).then((r) => setView((v) => (v ? { ...v, session: r.session } : r))).catch(() => {}); };
  const stop = async () => {
    if (!running) return;
    setStopping(true);
    try { await api(`/app/remote/jobs/${encodeURIComponent(running.id)}/stop`, { method: "POST", body: "{}" }); } catch { /* the button stays, and a tap tries again */ } finally { setStopping(false); }
  };

  // Measured: a second process on one session keeps the file sound but forks
  // the conversation, so the agent open at the desk never learns of this one.
  const who_ = s?.agentName ?? agentName(s?.host);
  const atDesk = !!s && known.length === 0 && Date.now() - +new Date(s.lastAt) < 30 * 60_000;
  const lastTurn = list[list.length - 1]?.key;
  const jobFor = new Map(jobs.flatMap((j) => { const t = turnOf(list, textOf(j), j.createdAt); return t ? [[t.key, j] as const] : []; }));
  const landed = new Set([...jobFor.values()].map((j) => j.id));
  // A message stays where it was sent. It moves into the conversation once the
  // laptop reports it, and it is never just taken away: without sharing it has
  // nowhere to move to. A decision shows on the finding it answered.
  const loose = jobs.filter((j) => !landed.has(j.id) && !(j.kind !== "prompt" && j.status === "done") && (textOf(j) || j.status !== "done"));
  const timeline = [
    ...list.map((t) => ({ at: startOf(t), turn: t, job: null })),
    ...loose.map((j) => ({ at: +new Date(j.createdAt), turn: null, job: j })),
  ].sort((a, b) => a.at - b.at);
  const firstNew = unreadAt === null ? null : list.find((t) => t.replies.some((r) => +new Date(r.at) >= unreadAt))?.key ?? null;

  return (
    <div className="chat-page">
      <aside className="chat-list" aria-label={`${agentName(s?.host)} chats`}>
        <div className="chat-list-head">
          {s && <AgentLogo host={s.host} size={28} />}
          <h2>{agentName(s?.host)}</h2>
        </div>
        {siblings.map((x) => <SessionRow key={x.id} s={x} project={x.project} current={x.id === id} />)}
      </aside>

      <section className="chat-pane">
        <header className="chat-head">
          <Link href={s ? `/app/sessions?agent=${s.host}` : "/app/sessions"} className="app-icon-btn chat-back" aria-label="Back to chats"><IconBack size={24} /></Link>
          {s && <AgentLogo host={s.host} size={32} />}
          <div style={{ display: "grid", gap: 2, minWidth: 0, flexGrow: 1 }}>
            <h1>{s ? <button type="button" className="chat-who" onClick={() => setWho(true)}>{who_}</button> : error ? "Chat" : "Loading…"}</h1>
            <span className="row-meta">{s ? [sessionName(s, agentName(s.host), s.project.name), s.project.name, cont?.machine.name].filter(Boolean).join(" · ") : ""}</span>
          </div>
          {running ? (
            <button className="btn-plain chat-stop" disabled={stopping || !!running.stopRequestedAt} onClick={stop}><IconStop size={12} />{stopping || running.stopRequestedAt ? "Stopping…" : "Stop"}</button>
          ) : s && <span className={`pill ${s.light}`}>{s.light === "working" && <span className="dot working" />}{LIGHT_LABEL[s.light]}</span>}
          {s && <button type="button" className="app-icon-btn" aria-label="Chat options" onClick={() => setMenu(true)}><IconMore size={22} /></button>}
        </header>
        {who && s && <AgentSheet chat={s.id} host={s.host} name={who_} model={s.model} agentId={s.agentId} project={s.project.id} onClose={() => setWho(false)} onChanged={refresh} />}
        {menu && s && (
          <SessionMenu session={{ id: s.id, name: sessionName(s, agentName(s.host), s.project.name), pinned: !!s.pinnedAt, archived: !!s.archivedAt }}
            onClose={() => setMenu(false)} onChanged={refresh}
            onDelete={() => window.location.assign(`/app/sessions?deleted=${encodeURIComponent(s.id)}`)}
            onCleared={() => { setEvents([]); setReplies([]); setMore(false); refresh(); }} />
        )}

        <div className="chat-scroll-wrap">
        <div className="chat-scroll" ref={scrollRef}>
          <div className="convo" ref={contentRef}>
            {error && <p className="composer-note error">{error}</p>}
            {!view && !error && [0, 1, 2].map((i) => <div key={i} className="skel" style={{ height: i === 1 ? 120 : 56 }} />)}
            {more && <button ref={top} type="button" className="load-earlier" onClick={earlier}>Load earlier</button>}
            {s && <p className="chat-divider">{scoreLine(s.score.session)}</p>}
            {view && timeline.length === 0 && <p className="chat-divider">No actions in the last 30 days.</p>}
            {timeline.map((item, n) => {
              const day = dayOf(item.at);
              const divider = n === 0 || day !== dayOf(timeline[n - 1].at) ? <p key={`day-${item.at}`} className="chat-day">{day}</p> : null;
              if (item.job) {
                const j = item.job;
                const text = textOf(j);
                const again = (j.status === "expired" || (j.status === "refused" && (j.code === "busy" || j.code === "in-use"))) && !!text;
                return (
                  <div key={j.id} style={{ display: "contents" }}>
                    {divider}
                    <MyMessage text={text ?? "A message you sent"} images={previewsFor(j.id)} note={j.status === "done" ? undefined : `${cont?.machine.name ?? "Computer"} · ${jobState(j)}`} />
                    {j.permission && <Finding tone="block" title="Action needed on Mac">{j.permission}</Finding>}
                    {again && (
                      <Finding tone="warn" title="Not delivered" actions={<SendAgain onSend={() => deliver(text, imagesFor(j.id))} />}>
                        {j.status === "expired" ? `${cont?.machine.name ?? "Your computer"} was offline, so this never reached it. Wake it, then send this again.` : j.code === "in-use" ? "This chat is open on your computer, so this was not sent. Close it there and send this again, or start a new chat." : "An agent was already working in this project, so this never started."}
                      </Finding>
                    )}
                  </div>
                );
              }
              const t = item.turn!;
              const last = t.key === lastTurn;
              const stops = t.events.filter((e) => e.findings.length && (e.blocked || e.severity));
              const job = jobFor.get(t.key);
              return (
                <div key={t.key} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
                  {divider}
                  {firstNew === t.key && <div ref={line} className="new-line">New messages</div>}
                  {t.request && <MyMessage text={requestLabel(t.request)!} images={job ? previewsFor(job.id) : undefined} note={job && active(job.status) ? `${cont?.machine.name ?? "Computer"} · ${jobState(job)}` : undefined} />}
                  {job?.permission && <Finding tone="block" title="Action needed on Mac">{job.permission}</Finding>}
                  <Activity lines={t.events.map(toLine)} live={last && working && !t.replies.length} />
                  <HelperLine name={who_} helpers={[...new Set(t.events.flatMap((e) => (e.helper ? [e.helper] : [])))]} />
                  {stops.slice(0, 3).map((e) => {
                    const blocking = e.blocked || e.severity === "block";
                    // Only the latest turn's stops are still open to a decision,
                    // and only where a laptop can carry it out.
                    const open = last && blocking && !!cont && !cont.machine.stopped && !!e.findings[0].target && !working;
                    const choice = decided[e.id];
                    return (
                      <Finding key={e.id} tone={blocking ? "block" : "warn"} title={e.findings[0].summary}
                        actions={choice ? <span className="decided">{choice === "allow" ? "Allowed once. It is trying again." : "Kept blocked. It has been told."}</span>
                          : open ? (
                            <>
                              <button className="btn-act" disabled={!!deciding} onClick={() => decide(e, "allow")}>{deciding === e.id ? `Waiting for ${proof()}…` : "Allow once"}</button>
                              <button className="btn-plain" disabled={!!deciding} onClick={() => decide(e, "deny")}>Keep blocked</button>
                            </>
                          ) : undefined}>
                        {e.findings[0].suggestion ?? (e.blocked ? `${who_} was told why and carried on.` : undefined)}
                        {decideError && open && !choice ? <><br /><span style={{ color: "var(--red-ink)" }}>{decideError}</span></> : null}
                      </Finding>
                    );
                  })}
                  {/* A reply that is only the question is shown once, in the card below. */}
                  {t.replies.filter((r) => !(asked && last && plain(r.text) === asked.question)).map((r) => <AgentReply key={r.id ?? r.at} host={s?.host ?? null} text={r.text} cut={r.cut} name={who_} model={r.model ?? s?.model} onName={() => setWho(true)} />)}
                </div>
              );
            })}
            {running?.saying && !replies.some((r) => r.text === running.saying) && <AgentReply host={s?.host ?? null} text={running.saying} name={who_} model={s?.model} onName={() => setWho(true)} />}
            {asked && <Question host={s?.host ?? null} question={asked.question} options={asked.options} onAnswer={send} />}
            {held && (
              <>
                <MyMessage text={held.text} note={heldError || (working ? `Waiting for ${who_} to finish` : "Ready to send")}
                  actions={<>
                    <button type="button" className="btn-plain" disabled={heldBusy} onClick={() => keep(null)}>Remove</button>
                    <button type="button" className="btn-act" disabled={working || heldBusy || !!blocked} onClick={sendHeld}>{heldBusy ? `Waiting for ${proof()}…` : "Send now"}</button>
                  </>} />
              </>
            )}
            {cont && !working && known.length > 0 && <ResumeHere agent={cont.agent} session={cont.session} />}
          </div>
        </div>
        {!isAtBottom && <JumpToNewest onClick={() => scrollToBottom("instant")} />}
        </div>

        <div className="chat-compose">
          <LaptopNotice machine={cont?.machine} />
          {passkeys.missing && <PasskeyNeeded onAdded={passkeys.reload} />}
          <Composer
            chat={id || undefined}
            placeholder={`Reply to ${who_}…`}
            blocked={blocked}
            note={working ? `${who_} is working. A message waits here until it finishes.` : atDesk ? "Open on your computer? Close it there first, or this won't be sent." : cont ? `${proof() === "Face ID" ? "Face ID" : "Your passkey"} · continues this chat on ${cont.machine.name}` : undefined}
            onSend={send}
          />
        </div>
      </section>
    </div>
  );
}
