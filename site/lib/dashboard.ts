// What the dashboard reads from the API, and how it words it.

export type Light = "needs-you" | "drifting" | "working" | "done" | "idle";
export type Score = { checked: number; aligned: number; notCheckable: number; percent: number | null };
export type Status = { light: Light; request: string | null; score: { request: Score; session: Score } };

export type Reply = { id?: string; at: string; turn: string | null; text: string; request?: string | null; cut?: boolean; model?: string | null };
/** Who a chat is with. agentId is set once the agent is on the team. */
export type Who = { agentName?: string; agentTitle?: string | null; agentId?: string | null; onTeam?: boolean; model?: string | null };
export type Position = { title: string; description: string | null };
export type SessionSummary = Status & Who & { id: string; host: string; startedAt: string; lastAt: string; reply: Reply | null; title?: string | null; pinnedAt?: string | null; archivedAt?: string | null };
export type Agent = { host: string; wired: boolean; reported: boolean; lastAt: string | null };
export type Project = { id: string; name: string; lastSeenAt: string; agents: Agent[]; sessions: SessionSummary[] };

export type Finding = { check: string; severity: string; summary: string; target?: string; suggestion?: string };
export type FeedEvent = {
  id: string; at: string; turn: string | null; request: string | null; helper?: string | null;
  action: string; tool: string | null; paths: string[]; installs: string[];
  pathsUnknown: boolean; commandUnderstood: boolean | null; blocked: boolean;
  severity: "block" | "warn" | null; checked: boolean;
  findings: Finding[]; unmeasured: { check: string; reason: string }[];
};
import type { Continuation } from "./remote";

export type SessionView = {
  session: Status & Who & { position?: Position | null; id: string; host: string; startedAt: string; lastAt: string; title?: string | null; pinnedAt?: string | null; archivedAt?: string | null; project: { id: string; name: string } };
  events: FeedEvent[];
  replies: (Reply & { id: string })[];
  /** Older messages exist above this page. */
  more?: boolean;
  cursor: string | null;
  // Where a reply would go. Null: no laptop can take it.
  continueWith?: Continuation | null;
  /** Messages sent to this conversation from any device, oldest first. */
  jobs?: SessionJob[];
};

export type SessionJob = {
  id: string; kind: string; status: string; code: string | null; reason: string | null; createdAt: string;
  stopRequestedAt: string | null;
  /** What the agent stopped to ask. Only on the latest job. */
  asked: { question: string; options: string[] } | null;
  /** A macOS prompt the agent may be waiting behind. */
  permission: string | null;
  /** The last thing it said while still working. */
  saying?: string | null;
};

export const LIGHT_LABEL: Record<Light, string> = {
  "needs-you": "Needs your OK",
  drifting: "Drifting",
  working: "Working",
  done: "Done",
  idle: "Ready",
};

export type Member = { id: string; name: string; title: string | null; description: string | null; host: string; createdAt: string; project: { id: string; name: string; clientId: string }; model: string | null; lastAt: string | null; chat: (SessionSummary & { project: { name: string } }) | null };
export type Profile = {
  agent: Omit<Member, "model" | "lastAt" | "chat">;
  model: string | null;
  chats: (SessionSummary & { project: { name: string } })[];
  record: { days: number; chats: number; steps: number; replies: number; findings: number; blocked: number; score: Score; partial: boolean };
};

/** "CTO · monkey-post-3b", or the name alone with no position. */
export const titled = (name: string, title?: string | null) => (title ? `${title} · ${name}` : name);

/** A position as the agent is told it. */
export const positionText = (p: { title: string | null; description: string | null } | null | undefined) =>
  p?.title ? [p.title, p.description].filter(Boolean).join(": ") : undefined;

/** claude-opus-5-5 as "Opus 5.5", gpt-6-astra as "GPT-6 Astra". Unknown ones stand as they are. */
export function modelLabel(m: string | null | undefined): string | null {
  if (!m) return null;
  const claude = m.match(/^claude-([a-z]+)-(\d+)(?:-(\d+))?/);
  if (claude) return `${claude[1][0].toUpperCase()}${claude[1].slice(1)} ${claude[2]}${claude[3] ? `.${claude[3]}` : ""}`;
  const gpt = m.match(/^gpt-([\d.]+)(?:-(.+))?$/);
  if (gpt) return `GPT-${gpt[1]}${gpt[2] ? ` ${gpt[2][0].toUpperCase()}${gpt[2].slice(1)}` : ""}`;
  return m;
}

export const HOST_LABEL: Record<string, string> = { "claude-code": "Claude Code", codex: "Codex", cursor: "Cursor" };

/** "92% aligned · 24 of 26 checked actions · 5 not checkable", or why there is no number. */
export function scoreLine(s: Score): string {
  const unchecked = s.notCheckable ? ` · ${s.notCheckable} not checkable` : "";
  if (s.percent === null) return `Nothing checkable yet${unchecked}`;
  return `${s.percent}% aligned · ${s.aligned} of ${s.checked} checked action${s.checked === 1 ? "" : "s"}${unchecked}`;
}

export function ago(iso: string, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000));
  if (s < 45) return "just now";
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} h ago`;
  return new Date(iso).toLocaleDateString();
}

/** What an action did, in a few words, never more than the upload carried. */
export function describe(e: FeedEvent): string {
  if (e.installs.length) return `Installed ${e.installs.join(", ")}`;
  const verb: Record<string, string> = {
    "write-file": "Wrote", "edit-file": "Edited", "delete-file": "Deleted", "read-file": "Read",
    "run-command": "Ran a command", "call-tool": "Called a tool", other: "Acted",
  };
  const v = verb[e.action] ?? "Acted";
  if (e.paths.length) return `${v} ${e.paths.slice(0, 2).join(", ")}${e.paths.length > 2 ? ` and ${e.paths.length - 2} more` : ""}`;
  return v;
}

/** Polls while the page is visible, and once more on coming back to it. */
/** Whether the server's line is open, so polling can be the slow backstop. */
export const line = { open: false };

/** Polls, slowing to `slow` while the line is open and doing the work itself. */
export function whileQuiet(fn: () => void, ms: number, slow: number): () => void {
  let last = Date.now();
  const run = () => { last = Date.now(); fn(); };
  const onChange = () => run();
  window.addEventListener("app:change", onChange);
  const stop = whileVisible(() => { if (!line.open || Date.now() - last >= slow) run(); }, ms);
  return () => { window.removeEventListener("app:change", onChange); stop(); };
}

export function whileVisible(fn: () => void, ms: number): () => void {
  let timer: ReturnType<typeof setInterval> | null = null;
  const start = () => { if (!timer) timer = setInterval(fn, ms); };
  const stop = () => { if (timer) { clearInterval(timer); timer = null; } };
  const onChange = () => { if (document.hidden) stop(); else { fn(); start(); } };
  document.addEventListener("visibilitychange", onChange);
  if (!document.hidden) start();
  return () => { stop(); document.removeEventListener("visibilitychange", onChange); };
}

/** What wakes an agent that is set up but has never reported. */
export const WAKE: Record<string, { text: string; copy?: string }> = {
  codex: { text: "Codex runs a hook only once you trust it. In Codex: Settings → Hooks → From Projects → Trust." },
  cursor: { text: "Send a message in Cursor's agent chat in this project." },
  "claude-code": { text: "Send a message in Claude Code in this project." },
};

/**
 * A request as a person would name it. A decision made on the phone reaches
 * the agent as a message the laptop writes (engine/cmd/trackline/remote_agent.go,
 * decisionText); shown whole it would read as something you typed.
 */
/** A session's name: what is known of it, or where and when it ran. */
export function sessionName(s: { title?: string | null; host: string; startedAt: string }, agent: string, project?: string): string {
  return s.title || [agent, project, new Date(s.startedAt).toLocaleDateString([], { day: "numeric", month: "short" })].filter(Boolean).join(" · ");
}

export function requestLabel(text: string | null): string | null {
  if (!text) return text;
  const allow = text.match(/^You were stopped by trackline \([^:]+: (.+?)\)\. The person you are working with has now approved/);
  if (allow) return `Allowed once from your phone: ${allow[1]}`;
  const deny = text.match(/^The person you are working with has decided, from their phone, to keep this blocked \([^:]+: (.+?)\)\./);
  if (deny) return `Kept blocked from your phone: ${deny[1]}`;
  return text;
}
