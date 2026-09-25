"use client";

import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, clearSession, rememberNext, session, type User } from "@/lib/account";
import { type Project, whileVisible } from "@/lib/dashboard";
import type { Overview as Remote } from "@/lib/remote";
import Mark from "@/components/Mark";
import AgentLogo from "./AgentLogo";
import PairingPrompt from "./PairingPrompt";
import { IconBell, IconChat, IconHome, IconLaptop, IconPlus, IconSignOut, IconUser } from "./icons";
import "./app.css";

// The frame every signed-in page sits in, and what they share: who is signed
// in, their projects and sessions, and their laptops. Fetched here once and
// kept fresh, so moving between pages is instant.

export type Section = "overview" | "sessions" | "machines" | "alerts" | "account" | "new";

type AppData = {
  user: User | null;
  projects: Project[] | null;
  remote: Remote | null;
  error: string;
  refresh: () => void;
};

const Ctx = createContext<AppData>({ user: null, projects: null, remote: null, error: "", refresh: () => {} });
export const useApp = () => useContext(Ctx);

const HOSTS = ["claude-code", "codex", "cursor"] as const;

export function initials(u: User | null): string {
  const from = u?.name ?? u?.email ?? "";
  const parts = from.replace(/@.*/, "").split(/[\s._-]+/).filter(Boolean);
  return (parts.slice(0, 2).map((p) => p[0]!.toUpperCase()).join("") || "·");
}

export function Avatar({ user, size = 34 }: { user: User | null; size?: number }) {
  return (
    <span className="app-avatar" style={{ width: size, height: size, fontSize: size * 0.38 }}>
      {user?.avatarUrl ? <img src={user.avatarUrl} alt="" referrerPolicy="no-referrer" /> : initials(user)}
    </span>
  );
}

export function signOut() {
  clearSession();
  window.location.replace("/signin");
}

export default function AppShell({ section, agent, flush, chat, children }: {
  section: Section; agent?: string; flush?: boolean;
  // A conversation fills the screen: on a phone the reply box takes the tab bar's place.
  chat?: boolean;
  children: ReactNode;
}) {
  const [user, setUser] = useState<User | null>(null);
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [remote, setRemote] = useState<Remote | null>(null);
  const [error, setError] = useState("");
  // Sessions filtered to one agent say so in the address; the sidebar lights it.
  const [urlAgent, setUrlAgent] = useState<string | undefined>();
  useEffect(() => {
    const sync = () => setUrlAgent(new URLSearchParams(window.location.search).get("agent") ?? undefined);
    sync();
    window.addEventListener("app:agent", sync);
    return () => window.removeEventListener("app:agent", sync);
  }, []);
  agent = agent ?? (section === "sessions" ? urlAgent : undefined);

  const gone = useCallback((e: unknown) => {
    if ((e as { status?: number }).status === 401) {
      clearSession();
      rememberNext(window.location.pathname + window.location.search);
      window.location.replace("/signin");
      return true;
    }
    return false;
  }, []);

  const load = useCallback(() => {
    api<{ projects: Project[] }>("/app/overview")
      .then((r) => { setProjects(r.projects); setError(""); })
      .catch((e) => { if (!gone(e)) setError((e as Error).message); });
    // Remote answers 404 while it is switched off for the account service.
    api<Remote>("/app/remote").then(setRemote).catch((e) => { if (!gone(e)) setRemote({ machines: [], pairings: [], jobs: [] }); });
  }, [gone]);

  useEffect(() => {
    if (!session()) {
      rememberNext(window.location.pathname + window.location.search);
      window.location.replace("/signin");
      return;
    }
    api<{ user: User }>("/auth/me").then((r) => setUser(r.user)).catch(gone);
    load();
    return whileVisible(load, 8000);
  }, [load, gone]);

  const sessions = projects?.flatMap((p) => p.sessions) ?? [];
  const count = (h: string) => sessions.filter((s) => s.host === h).length;
  const wired = (h: string) => !!projects?.some((p) => p.agents.some((a) => a.host === h && a.wired));
  const needsYou = sessions.filter((s) => s.light === "needs-you").length;

  const here = (s: Section) => (section === s && !agent ? "page" : undefined);

  return (
    <Ctx.Provider value={{ user, projects, remote, error, refresh: load }}>
      <div className={`app-shell ${chat ? "chat-mode" : ""}`}>
        <nav className="app-side" aria-label="Main">
          <a href="/app" className="app-brand"><Mark />trackline</a>
          <a href="/app/remote" className="app-new"><IconPlus />New task</a>
          <div className="app-nav">
            <a className="app-link" href="/app" aria-current={here("overview")}><IconHome />Overview</a>
            <a className="app-link" href="/app/sessions" aria-current={here("sessions")}><IconChat />All sessions<span className="count">{sessions.length || ""}</span></a>
            <a className="app-link" href="/app/machines" aria-current={here("machines")}><IconLaptop />Machines</a>
            <a className="app-link" href="/app/alerts" aria-current={here("alerts")}><IconBell />Alerts{needsYou > 0 && <span className="badge">{needsYou}</span>}</a>
          </div>
          <div className="app-nav">
            <div className="app-nav-label">Agents</div>
            {HOSTS.map((h) => (
              <a key={h} className="app-link" href={`/app/sessions?agent=${h}`} aria-current={agent === h ? "page" : undefined}>
                <AgentLogo host={h} size={22} />
                {h === "claude-code" ? "Claude Code" : h === "codex" ? "Codex" : "Cursor"}
                <span className="count">{count(h) || (projects && !wired(h) ? "Set up" : "")}</span>
              </a>
            ))}
          </div>
          <div className="app-me">
            <Avatar user={user} />
            <a href="/account" aria-current={here("account")}>
              <strong>{user?.name ?? "Your account"}</strong>
              <span>{user?.email ?? ""}</span>
            </a>
            <button className="app-icon-btn" aria-label="Sign out" onClick={signOut}><IconSignOut /></button>
          </div>
        </nav>

        <header className="app-top">
          <a href="/app" className="app-brand"><Mark />trackline</a>
          <a href="/account" className="app-avatar-link" aria-label="Account"><Avatar user={user} /></a>
        </header>

        <main className={`app-main ${flush || chat ? "flush" : ""}`}>{children}</main>

        <nav className="app-tabs" aria-label="Main">
          <a className="app-tab" href="/app" aria-current={section === "overview" ? "page" : undefined}><IconHome />Home</a>
          <a className="app-tab" href="/app/sessions" aria-current={section === "sessions" ? "page" : undefined}><IconChat />Sessions</a>
          <a className="app-tab-new" href="/app/remote" aria-label="New task"><IconPlus /></a>
          <a className="app-tab" href="/app/alerts" aria-current={section === "alerts" ? "page" : undefined}><IconBell />Alerts</a>
          <a className="app-tab" href="/account" aria-current={section === "account" ? "page" : undefined}><IconUser />Account</a>
        </nav>

        <PairingPrompt />
      </div>
    </Ctx.Provider>
  );
}
