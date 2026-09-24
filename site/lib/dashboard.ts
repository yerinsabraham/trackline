// What the dashboard reads from the API, and how it words it.

export type Light = "on-track" | "drifting" | "needs-you" | "quiet";
export type Score = { checked: number; aligned: number; notCheckable: number; percent: number | null };
export type Status = { light: Light; request: string | null; score: { request: Score; session: Score } };

export type SessionSummary = Status & { id: string; host: string; startedAt: string; lastAt: string };
export type Project = { id: string; name: string; lastSeenAt: string; sessions: SessionSummary[] };

export type Finding = { check: string; severity: string; summary: string; target?: string; suggestion?: string };
export type FeedEvent = {
  id: string; at: string; turn: string | null; request: string | null;
  action: string; tool: string | null; paths: string[]; installs: string[];
  pathsUnknown: boolean; commandUnderstood: boolean | null; blocked: boolean;
  severity: "block" | "warn" | null; checked: boolean;
  findings: Finding[]; unmeasured: { check: string; reason: string }[];
};
export type SessionView = {
  session: Status & { id: string; host: string; startedAt: string; lastAt: string; project: { id: string; name: string } };
  events: FeedEvent[];
  cursor: string | null;
};

export const LIGHT_LABEL: Record<Light, string> = {
  "on-track": "On track",
  drifting: "Drifting",
  "needs-you": "Needs you",
  quiet: "Quiet",
};

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
export function whileVisible(fn: () => void, ms: number): () => void {
  let timer: ReturnType<typeof setInterval> | null = null;
  const start = () => { if (!timer) timer = setInterval(fn, ms); };
  const stop = () => { if (timer) { clearInterval(timer); timer = null; } };
  const onChange = () => { if (document.hidden) stop(); else { fn(); start(); } };
  document.addEventListener("visibilitychange", onChange);
  if (!document.hidden) start();
  return () => { stop(); document.removeEventListener("visibilitychange", onChange); };
}
