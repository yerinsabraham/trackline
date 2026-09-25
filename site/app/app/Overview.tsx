"use client";

import { useEffect, useState } from "react";
import { api, clearSession, rememberNext, session } from "@/lib/account";
import { ago, HOST_LABEL, LIGHT_LABEL, type Project, scoreLine, whileVisible } from "@/lib/dashboard";
import AgentChecklist from "@/components/AgentChecklist";
import { plain } from "@/components/ReplyText";
import "./dashboard.css";

export default function Overview() {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [error, setError] = useState("");
  const [, tick] = useState(0);

  useEffect(() => {
    if (!session()) { rememberNext("/app"); return window.location.replace("/signin"); }
    const load = () =>
      api<{ projects: Project[] }>("/app/overview")
        .then((r) => { setProjects(r.projects); setError(""); tick((n) => n + 1); })
        .catch((e) => {
          if ((e as { status?: number }).status === 401) { clearSession(); rememberNext("/app"); window.location.replace("/signin"); }
          else setError((e as Error).message);
        });
    load();
    return whileVisible(load, 10_000);
  }, []);

  return (
    <div className="wrap dash">
      <div className="dash-head">
        <p className="eyebrow">Dashboard</p>
        <h1>Your agents</h1>
      </div>
      {error && <p className="account-error" role="alert">{error}</p>}
      {projects === null && !error && <p className="dash-muted">Loading…</p>}
      {projects?.length === 0 && (
        <div className="dash-empty">
          <p>Nothing here yet.</p>
          <p className="dash-muted">Run <code>trackline connect</code> in a project. What your agent does there shows up here as it happens.</p>
          <p className="dash-muted">Using Codex? It only runs hooks you trust: open it in the project, type <code>/hooks</code>, and trust trackline.</p>
        </div>
      )}
      {projects?.map((p) => (
        <section key={p.id} className="dash-project">
          <h2>{p.name}</h2>
          <AgentChecklist agents={p.agents} />
          {p.sessions.length === 0 && p.agents.length === 0 && <p className="dash-muted">No sessions in the last 30 days.</p>}
          <ul className="dash-sessions">
            {p.sessions.map((s) => (
              <li key={s.id}>
                <a href={`/app/session?id=${encodeURIComponent(s.id)}`} className={`dash-session light-${s.light}`}>
                  <span className="dash-dot" aria-hidden />
                  <span className="dash-session-main">
                    <span className="dash-session-top">
                      <strong>{LIGHT_LABEL[s.light]}</strong>
                      <span>{HOST_LABEL[s.host] ?? s.host} · {ago(s.lastAt)}</span>
                    </span>
                    <span className="dash-request">{s.request ?? "Request not recorded"}</span>
                    {s.reply && <span className="dash-session-reply">{HOST_LABEL[s.host] ?? "Agent"}: {plain(s.reply.text)}</span>}
                    <span className="dash-score">{scoreLine(s.score.request)}</span>
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </section>
      ))}
      {projects && projects.length > 0 && (
        <p className="dash-muted dash-help">
          An agent missing? Run <code>trackline status</code> in the project. It says which agent has never reported, and why.
          {" "}<a href="/app/alerts">Alert settings</a>.
        </p>
      )}
    </div>
  );
}
