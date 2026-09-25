"use client";

import { useState } from "react";
import { api } from "@/lib/account";
import { ago } from "@/lib/dashboard";
import { useApp } from "@/components/app/AppShell";
import { IconKey, IconLaptop, IconStop } from "@/components/app/icons";

// The laptops that take tasks: whether each is awake, what it takes tasks
// for, which passkeys can send to it, and a stop for each.

export default function Machines() {
  const { remote, refresh } = useApp();
  const [busy, setBusy] = useState("");

  const stop = async (machine?: string) => {
    if (!window.confirm(machine ? "Stop remote on this laptop? Starting again takes the laptop." : "Stop remote on every laptop? Starting again takes the laptop.")) return;
    setBusy(machine ?? "all");
    try { await api("/app/remote/stop", { method: "POST", body: JSON.stringify(machine ? { machine } : {}) }); refresh(); } finally { setBusy(""); }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 18, maxWidth: 860 }}>
      <header className="rise">
        <h1 className="app-h1">Machines</h1>
        <p className="app-sub">Laptops that take tasks from you. Only the laptop can turn remote on.</p>
      </header>
      {!remote && <div className="skel" style={{ height: 160 }} />}
      {remote?.machines.length === 0 && (
        <div className="card empty">
          <span className="agent-logo" style={{ width: 44, height: 44 }}><span style={{ width: 22, display: "flex" }}><IconLaptop /></span></span>
          <strong>No laptop takes tasks yet.</strong>
          <span>On your laptop, in a project: <code>trackline remote enable</code>. It shows a code; this app asks for it.</span>
        </div>
      )}
      {remote?.machines.map((m, i) => (
        <section key={m.id} className={`card rise rise-${Math.min(i + 1, 4)}`} aria-labelledby={`m-${m.id}`}>
          <div className="card-head">
            <span className="agent-logo" style={{ width: 40, height: 40 }}><span style={{ width: 20, display: "flex" }}><IconLaptop /></span></span>
            <div style={{ display: "grid", gap: 2 }}>
              <h2 id={`m-${m.id}`}>{m.name}</h2>
              <span style={{ fontSize: 13, display: "flex", alignItems: "center", gap: 6, color: m.stopped ? "var(--red-ink)" : m.online ? "var(--ok-ink)" : "var(--muted)" }}>
                <span className={`dot ${m.online && !m.stopped ? "working" : ""}`} />
                {m.stopped ? "Stopped from here" : m.online ? "Online · ready for tasks" : m.lastSeenAt ? `Asleep or offline · seen ${ago(m.lastSeenAt)}` : "Not seen yet"}
              </span>
            </div>
            {!m.stopped && (
              <button className="btn-plain" style={{ marginLeft: "auto", height: 36 }} disabled={!!busy} onClick={() => stop(m.id)}>
                <span style={{ width: 12, display: "flex" }}><IconStop /></span>Stop
              </button>
            )}
          </div>
          <div className="card-body">
            <div style={{ display: "grid", gap: 6 }}>
              <span className="app-nav-label" style={{ padding: 0 }}>Takes tasks for</span>
              {m.projects.length === 0 ? <span className="app-muted" style={{ fontSize: 14 }}>No project</span> : (
                <div className="chips">{m.projects.map((p) => <span key={p.id} className="chip mono" style={{ display: "inline-flex", alignItems: "center" }}>{p.name}</span>)}</div>
              )}
            </div>
            <div style={{ display: "grid", gap: 6 }}>
              <span className="app-nav-label" style={{ padding: 0 }}>Passkeys that can send to it</span>
              {m.passkeys.length === 0 ? <span className="app-muted" style={{ fontSize: 14 }}>None paired</span> : m.passkeys.map((k) => (
                <span key={k.id} style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 14 }}><span style={{ width: 16, display: "flex" }}><IconKey /></span>{k.name}</span>
              ))}
            </div>
            {m.stopped && <p className="app-muted" style={{ margin: 0, fontSize: 13.5 }}>To start again, on the laptop: <code className="mono">trackline remote enable</code></p>}
          </div>
        </section>
      ))}
      {remote && remote.machines.some((m) => !m.stopped) && remote.machines.length > 1 && (
        <button className="btn-danger-text" style={{ alignSelf: "flex-start" }} disabled={!!busy} onClick={() => stop()}>Stop remote on every laptop</button>
      )}
    </div>
  );
}
