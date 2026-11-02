"use client";

import Link from "next/link";
import { useApp } from "@/components/app/AppShell";
import { IconBell, IconLaptop, IconServer, IconTarget, IconUser } from "@/components/app/icons";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import { WAKE } from "@/lib/dashboard";
import UpdateCard from "@/components/app/UpdateCard";

// Everything that is not a chat or a teammate.

const PLACES = [
  { href: "/app/machines", label: "Computers", icon: <IconLaptop /> },
  { href: "/app/alerts", label: "Alerts", icon: <IconBell /> },
  { href: "/app/ontrack", label: "On track", icon: <IconTarget /> },
  { href: "/app/production", label: "Production", icon: <IconServer /> },
  { href: "/account", label: "Account", icon: <IconUser /> },
];

export default function Settings() {
  const { projects } = useApp();
  // Set up and never heard from looks exactly like nothing to say, and Codex
  // runs a hook only once it is trusted, so a project can be silently unwatched.
  const silent = (projects ?? []).flatMap((p) => p.agents.filter((a) => a.wired && !a.reported).map((a) => ({ project: p.name, host: a.host })));
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 860 }}>
      <header className="rise"><h1 className="app-h1">Settings</h1></header>
      <nav className="card rise" aria-label="Settings">
        {PLACES.map((p) => <Link key={p.href} href={p.href} className="row-link settings-row">{p.icon}<strong>{p.label}</strong></Link>)}
      </nav>
      <UpdateCard />
      {silent.length > 0 && (
        <section className="card rise" aria-labelledby="silent">
          <div className="card-head"><h2 id="silent">Not watched yet</h2></div>
          {silent.map((a) => (
            <div key={`${a.project}-${a.host}`} className="row-link">
              <AgentLogo host={a.host} size={30} />
              <span className="row-main">
                <strong>{agentName(a.host)} in {a.project}</strong>
                <span className="row-meta">{WAKE[a.host]?.text}</span>
              </span>
            </div>
          ))}
        </section>
      )}
    </div>
  );
}
