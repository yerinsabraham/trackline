"use client";

import { useCallback, useEffect, useRef, useState, type TouchEvent } from "react";
import Link from "next/link";
import { api, readAt } from "@/lib/account";
import { sessionName, type SessionSummary, whileVisible } from "@/lib/dashboard";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import { useApp } from "@/components/app/AppShell";
import AgentSheet from "@/components/app/AgentSheet";
import FirstRun from "@/components/app/FirstRun";
import HandoffCards from "@/components/app/HandoffCards";
import { IconMore } from "@/components/app/icons";
import SessionMenu from "@/components/app/SessionMenu";
import { SessionRow } from "@/components/app/SessionRow";

// Every chat, whatever waits on you first, then newest: the app's front page.
// Claude Code, Codex and Cursor each have their own list too, the way the
// sidebar names them.

const AGENTS: [string, string][] = [["", "All"], ["claude-code", "Claude Code"], ["codex", "Codex"], ["cursor", "Cursor"]];
type Filter = "all" | "working" | "needs-you" | "done";
const FILTERS: [Filter, string][] = [["all", "All"], ["working", "Working"], ["needs-you", "Needs your OK"], ["done", "Done"]];

type Row = SessionSummary & { project: { name: string }; projectId?: string };
const UNDO_MS = 6000;

/**
 * One row: tap to open, swipe left for Delete, hold to start choosing several,
 * or the three dots for everything. In choosing mode a tap ticks it instead.
 */
function ListRow({ s, choosing, chosen, onToggle, onMenu, onDelete, onHold }: {
  s: Row; choosing: boolean; chosen: boolean; onToggle: () => void; onMenu: () => void; onDelete: () => void; onHold: () => void;
}) {
  const [dx, setDx] = useState(0);
  const start = useRef<{ x: number; y: number; dx: number } | null>(null);
  const hold = useRef<ReturnType<typeof setTimeout> | null>(null);
  const held = useRef(false);
  const stopHold = () => { if (hold.current) clearTimeout(hold.current); hold.current = null; };
  const down = (e: TouchEvent) => {
    const t = e.touches[0];
    start.current = { x: t.clientX, y: t.clientY, dx };
    held.current = false;
    hold.current = setTimeout(() => { held.current = true; onHold(); }, 500);
  };
  const move = (e: TouchEvent) => {
    if (!start.current) return;
    const t = e.touches[0];
    const x = t.clientX - start.current.x;
    if (Math.abs(x) > 8 || Math.abs(t.clientY - start.current.y) > 8) stopHold();
    if (!choosing && Math.abs(x) > Math.abs(t.clientY - start.current.y)) setDx(Math.max(-96, Math.min(0, start.current.dx + x)));
  };
  const up = () => {
    stopHold();
    start.current = null;
    setDx((d) => (d < -48 ? -96 : 0));
  };
  return (
    <div className={`swipe ${s.archivedAt ? "archived" : ""}`}>
      <button type="button" className="swipe-delete" tabIndex={dx ? 0 : -1} aria-hidden={!dx} onClick={onDelete}>Delete</button>
      <div className="swipe-row" style={{ transform: dx ? `translateX(${dx}px)` : undefined }} onTouchStart={down} onTouchMove={move} onTouchEnd={up} onTouchCancel={up}>
        {choosing && <input type="checkbox" className="row-tick" checked={chosen} onChange={onToggle} aria-label={`Choose ${s.title ?? "chat"}`} />}
        <SessionRow s={s} project={s.project.name} unread={!!s.reply && +new Date(s.reply.at) > (readAt(s.id) ?? Infinity)} onClick={(e) => {
          if (dx) { e.preventDefault(); setDx(0); }
          else if (held.current || choosing) { e.preventDefault(); if (choosing && !held.current) onToggle(); held.current = false; }
        }} />
        {!choosing && <button type="button" className="row-more" aria-label="Chat options" onClick={onMenu}><IconMore size={20} /></button>}
      </div>
    </div>
  );
}

export default function Sessions() {
  const { projects, remote, error } = useApp();
  const [onTrack, setOnTrack] = useState<number | null | undefined>(undefined);
  useEffect(() => {
    // On its own slow clock: this reads a whole day of actions.
    const load = () => { api<{ score: { percent: number | null } }>("/app/ontrack?days=1").then((r) => setOnTrack(r.score.percent)).catch(() => setOnTrack(null)); };
    load();
    return whileVisible(load, 60_000);
  }, []);
  const [agent, setAgent] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [q, setQ] = useState("");
  const [project, setProject] = useState("");
  const [archived, setArchived] = useState(false);
  const [rows, setRows] = useState<Row[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [total, setTotal] = useState(0);
  const [more, setMore] = useState(false);
  const [page, setPage] = useState(0);
  const [menu, setMenu] = useState<Row | null>(null);
  const [who, setWho] = useState<Row | null>(null);
  const [choosing, setChoosing] = useState(false);
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [undo, setUndo] = useState<{ ids: string[]; timer: ReturnType<typeof setTimeout> } | null>(null);
  const undoing = useRef(undo);
  undoing.current = undo;

  const asked = useRef(0);
  const pages = useRef(0);
  const read = useCallback(async (at: number) => {
    const mine = at === 0 ? ++asked.current : asked.current;
    const u = new URLSearchParams({ page: String(at) });
    if (q.trim()) u.set("q", q.trim());
    if (agent) u.set("host", agent);
    if (project) u.set("project", project);
    if (archived) u.set("archived", "1");
    const r = await api<{ sessions: Row[]; total: number; more: boolean }>(`/app/sessions?${u}`);
    // Typing outruns the network: only the latest question gets to answer.
    if (mine !== asked.current) return;
    setTotal(r.total);
    setMore(r.more);
    setRows((old) => {
      const fresh = new Set(r.sessions.map((x) => x.id));
      if (at > 0) return [...(old ?? []).filter((x) => !fresh.has(x.id)), ...r.sessions];
      // A refresh of the top, keeping any further pages already shown.
      return pages.current === 0 ? r.sessions : [...r.sessions, ...(old ?? []).slice(r.sessions.length).filter((x) => !fresh.has(x.id))];
    });
  }, [q, agent, project, archived]);

  // Asking again from the top whenever what is asked for changes.
  useEffect(() => {
    setRows(null); setPage(0); pages.current = 0;
    const t = setTimeout(() => { read(0).then(() => setFailed(false)).catch(() => { setRows([]); setFailed(true); }); }, q ? 250 : 0);
    return () => clearTimeout(t);
  }, [read, q]);
  useEffect(() => whileVisible(() => { read(0).catch(() => {}); }, 8000), [read]);

  const commit = (ids: string[]) => {
    const body = JSON.stringify({ ids, action: "delete" });
    return api("/app/sessions/bulk", { method: "POST", body, keepalive: true }).catch(() => {});
  };
  // Gone at once, and truly gone six seconds later, or as you leave the page.
  const remove = useCallback((ids: string[]) => {
    const was = undoing.current;
    if (was) { clearTimeout(was.timer); commit(was.ids); }
    const timer = setTimeout(() => { setUndo(null); commit(ids).then(() => read(0)); }, UNDO_MS);
    setUndo({ ids, timer });
    setChoosing(false); setChosen(new Set());
  }, [read]);
  useEffect(() => {
    const flush = () => { const u = undoing.current; if (u) { clearTimeout(u.timer); commit(u.ids); setUndo(null); } };
    window.addEventListener("pagehide", flush);
    return () => { window.removeEventListener("pagehide", flush); flush(); };
  }, []);

  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    setAgent(q.get("agent") ?? "");
    const st = q.get("status");
    if (st === "working" || st === "needs-you" || st === "done") setFilter(st);
    // Deleted from inside the chat: the Undo lives here, where the row was.
    const gone = q.get("deleted");
    if (gone) {
      window.history.replaceState(null, "", window.location.pathname);
      remove([gone]);
    }
  }, [remove]);
  const pick = (a: string) => {
    setAgent(a);
    window.history.replaceState(null, "", a ? `/app/sessions?agent=${a}` : "/app/sessions");
    window.dispatchEvent(new Event("app:agent"));
  };

  const hidden = new Set(undo?.ids ?? []);
  // What the shell already holds: read at once while the list loads, so a tab
  // opens with no skeleton, and still worth reading with no signal.
  const plainList = !q && !archived && !agent && !project;
  const last = (failed || !rows) && plainList ? (projects ?? []).flatMap((p) => p.sessions.map((s) => ({ ...s, project: { name: p.name } }))) : [];
  const all = (rows?.length ? rows : last).filter((s) => !hidden.has(s.id));
  const matches = (s: Row, f: Filter) =>
    f === "all" ? true : f === "done" ? s.light === "done" || s.light === "idle" : s.light === f;
  // What waits on you goes first, under anything you pinned to stay on top.
  const shown = all.filter((s) => matches(s, filter)).sort((a, b) => Number(!!b.pinnedAt) - Number(!!a.pinnedAt) || Number(b.light === "needs-you") - Number(a.light === "needs-you"));
  const toggle = (id: string) => setChosen((c) => { const n = new Set(c); if (!n.delete(id)) n.add(id); return n; });
  const archive = async () => {
    await api("/app/sessions/bulk", { method: "POST", body: JSON.stringify({ ids: [...chosen], action: "archive" }) }).catch(() => {});
    setChoosing(false); setChosen(new Set()); setRows(null); read(0).catch(() => {});
  };
  const wired = !agent || !!projects?.some((p) => p.agents.some((a) => a.host === agent && a.wired));

  const fresh = !!projects && !failed && projects.every((p) => p.sessions.length === 0) && rows?.length === 0 && !q && !archived;
  if (fresh) return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <header className="rise">
        <h1 className="app-h1">Chats</h1>
        <p className="app-sub">Three steps and your agents show up here.</p>
      </header>
      <FirstRun projects={projects} />
    </div>
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 860 }}>
      <header className="rise chats-head">
        <h1 className="app-h1">Chats</h1>
        {onTrack !== undefined && <Link href="/app/ontrack" className="on-track-link">{onTrack === null ? "Nothing checked today" : `${onTrack}% on track today`}</Link>}
      </header>
      {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}
      {!!remote?.machines.length && remote.machines.every((m) => !m.online) && <p className="offline-bar" role="status">Computer offline. Messages wait until it is back.</p>}
      <HandoffCards />
      <div className="seg rise rise-1" role="group" aria-label="Agent">
        {AGENTS.map(([a, label]) => (
          <button key={a || "all"} aria-pressed={agent === a} onClick={() => pick(a)}>
            {a && <AgentLogo host={a} size={20} />}
            <span className="seg-label">{label}</span>
          </button>
        ))}
      </div>
      <input type="search" className="list-search rise rise-2" aria-label="Search chats" placeholder="Search agents, names and replies" value={q} onChange={(e) => setQ(e.target.value)} />
      <div className="chips rise rise-2" role="group" aria-label="Status">
        {FILTERS.map(([f, label]) => (
          <button key={f} className="pick" aria-pressed={filter === f} onClick={() => setFilter(f)}>
            {label}{f === "all" && rows ? ` ${total}` : ""}
          </button>
        ))}
        <button className="pick" aria-pressed={archived} onClick={() => setArchived((a) => !a)}>Archived</button>
        {(projects?.length ?? 0) > 1 && (
          <select className="pick" aria-label="Project" value={project} onChange={(e) => setProject(e.target.value)}>
            <option value="">All projects</option>
            {projects!.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
        )}
      </div>

      {!rows && !all.length && [0, 1, 2, 3].map((i) => <div key={i} className="skel" style={{ height: 72 }} />)}
      {projects && !wired && (
        <div className="card empty">
          <AgentLogo host={agent} size={40} />
          <strong>trackline is not set up for {AGENTS.find(([a]) => a === agent)?.[1]} yet.</strong>
          <span>In a project on your computer: <code>trackline init --host {agent === "claude-code" ? "claude" : agent}</code></span>
        </div>
      )}
      {rows && wired && shown.length === 0 && !failed && <div className="card empty">{q ? "Nothing matches that." : archived ? "Nothing archived." : "No chats here yet."}</div>}
      {shown.length > 0 && (
        <div className="card rise">
          {shown.map((s) => (
            <ListRow key={s.id} s={s} choosing={choosing} chosen={chosen.has(s.id)} onToggle={() => toggle(s.id)} onMenu={() => setMenu(s)} onDelete={() => remove([s.id])}
              onHold={() => { setChoosing(true); toggle(s.id); }} />
          ))}
        </div>
      )}
      {more && <button type="button" className="btn-plain" onClick={() => { const next = page + 1; setPage(next); pages.current = next; read(next).catch(() => {}); }}>Show more</button>}

      {choosing && (
        <div className="select-bar" role="toolbar" aria-label="Chosen chats">
          <span>{chosen.size} chosen</span>
          <button type="button" className="btn-plain" onClick={() => { setChoosing(false); setChosen(new Set()); }}>Cancel</button>
          <button type="button" className="btn-plain" disabled={!chosen.size} onClick={archive}>Archive</button>
          <button type="button" className="btn-act" disabled={!chosen.size} onClick={() => remove([...chosen])}>Delete</button>
        </div>
      )}
      {undo && (
        <div className="undo-bar" role="status">
          <span>{undo.ids.length === 1 ? "Chat deleted" : `${undo.ids.length} chats deleted`}</span>
          <button type="button" onClick={() => { clearTimeout(undo.timer); setUndo(null); }}>Undo</button>
        </div>
      )}
      {menu && (
        <SessionMenu session={{ id: menu.id, name: sessionName(menu, agentName(menu.host), menu.project.name), pinned: !!menu.pinnedAt, archived: !!menu.archivedAt, onTeam: !!menu.agentId }}
          onClose={() => setMenu(null)} onChanged={() => { setRows(null); read(0).catch(() => {}); }} onDelete={() => remove([menu.id])}
          onSelect={() => setChoosing(true)} onAgent={() => setWho(menu)} />
      )}
      {who && <AgentSheet chat={who.id} host={who.host} name={who.agentName ?? agentName(who.host)} model={who.model} agentId={who.agentId} project={who.projectId ?? ""}
        onClose={() => setWho(null)} onChanged={() => { read(0).catch(() => {}); }} />}
    </div>
  );
}
