"use client";

import Link from "next/link";
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, cached, clearSession, keepCached, listen, readAt, rememberNext, session, type User } from "@/lib/account";
import { line, type Project, whileQuiet } from "@/lib/dashboard";
import type { Overview as Remote } from "@/lib/remote";
import { forgetThisDevice } from "@/lib/alerts";
import Mark from "@/components/Mark";
import InstallHint from "./InstallHint";
import PairingPrompt from "./PairingPrompt";
import { IconChat, IconGear, IconHome, IconPlus, IconTarget, IconTeam } from "./icons";
import "./app.css";

// The frame every signed-in page sits in, and what they share: who is signed
// in, their projects and sessions, and their laptops. Fetched here once and
// kept fresh, so moving between pages is instant.

export type Section = "home" | "chats" | "team" | "ontrack" | "production" | "settings" | "account" | "new";

type AppData = {
  user: User | null;
  projects: Project[] | null;
  /** The newest trackline release, or null while unknown. */
  latest: string | null;
  remote: Remote | null;
  error: string;
  refresh: () => void;
};

const Ctx = createContext<AppData>({ user: null, projects: null, latest: null, remote: null, error: "", refresh: () => {} });
export const useApp = () => useContext(Ctx);

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

export async function signOut() {
  // A signed-out phone must stop getting the words of replies on its lock screen.
  await forgetThisDevice().catch(() => {});
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
  const [latest, setLatest] = useState<string | null>(null);
  const [offline, setOffline] = useState(false);
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
  agent = agent ?? (section === "chats" ? urlAgent : undefined);

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
    api<{ projects: Project[]; latest?: string | null }>("/app/overview")
      .then((r) => { setProjects(r.projects); setLatest(r.latest ?? null); setError(""); setOffline(false); keepCached("overview", r.projects); })
      .catch((e) => {
        if (gone(e)) return;
        // No answer at all is no signal, which is worth saying differently, and
        // the last list is still worth reading.
        if ((e as { status?: number }).status === undefined) {
          setOffline(true);
          setProjects((p) => p ?? cached<Project[]>("overview"));
        } else setError((e as Error).message);
      });
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
    return whileQuiet(load, 8000, 30_000);
  }, [load, gone]);

  // One line for the whole app: pages listen for what it hears. A dropped line
  // is normal on a phone, and polling covers the gap until it reopens.
  useEffect(() => {
    if (!session()) return;
    const stop = new AbortController();
    (async () => {
      while (!stop.signal.aborted) {
        if (document.visibilityState === "visible") {
          await listen(() => window.dispatchEvent(new Event("app:change")), () => { line.open = true; }, stop.signal).catch(() => {});
          line.open = false;
        }
        await new Promise((r) => setTimeout(r, 5000));
      }
    })();
    return () => stop.abort();
  }, []);

  const sessions = projects?.flatMap((p) => p.sessions) ?? [];
  const needsYou = sessions.filter((s) => s.light === "needs-you").length;
  const unread = sessions.filter((s) => s.reply && +new Date(s.reply.at) > (readAt(s.id) ?? Infinity)).length;
  useEffect(() => {
    if (!projects || !("setAppBadge" in navigator)) return;
    const n = needsYou + unread;
    (n ? navigator.setAppBadge(n) : navigator.clearAppBadge()).catch(() => {});
  }, [projects, needsYou, unread]);

  const here = (s: Section) => (section === s ? "page" : undefined);

  return (
    <Ctx.Provider value={{ user, projects, latest, remote, error, refresh: load }}>
      <div className={`app-shell ${chat ? "chat-mode" : ""}`}>
        <nav className="app-side" aria-label="Main">
          <Link href="/app" className="app-brand"><Mark />trackline</Link>
          <div className="app-nav">
            <Link className="app-link" href="/app" aria-current={here("home")}><IconHome />Home</Link>
            <Link className="app-link" href="/app/sessions" aria-current={here("chats")}><IconChat />Chats{needsYou > 0 ? <span className="badge">{needsYou}</span> : null}</Link>
            <Link className="app-link" href="/app/team" aria-current={here("team")}><IconTeam />Team</Link>
            <Link className="app-link" href="/app/ontrack" aria-current={here("ontrack")}><IconTarget />On track</Link>
            <Link className="app-link" href="/app/settings" aria-current={here("settings")}><IconGear />Settings</Link>
          </div>
          <Link href="/app/remote" className="app-new">New chat</Link>
        </nav>

        <main className={`app-main ${flush || chat ? "flush" : ""}`}>
          {offline && <p className="offline-bar" role="status">No connection. Showing what was last seen.</p>}
          {!chat && <InstallHint />}{children}
        </main>

        <nav className="app-tabs" aria-label="Main">
          <Link className="app-tab" href="/app" aria-current={section === "home" ? "page" : undefined}><IconHome />Home</Link>
          <Link className="app-tab" href="/app/sessions" aria-current={section === "chats" ? "page" : undefined}><IconChat />Chats{needsYou > 0 && <span className="badge">{needsYou}</span>}</Link>
          <Link className="app-tab-new" href="/app/remote" aria-label="New chat"><IconPlus /></Link>
          <Link className="app-tab" href="/app/team" aria-current={section === "team" ? "page" : undefined}><IconTeam />Team</Link>
          <Link className="app-tab" href="/app/settings" aria-current={section === "settings" || section === "account" || section === "ontrack" || section === "production" ? "page" : undefined}><IconGear />Settings</Link>
        </nav>

        <PairingPrompt />
      </div>
    </Ctx.Provider>
  );
}
