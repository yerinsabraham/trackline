"use client";

import { useEffect, useState } from "react";
import AgentChecklist from "@/components/AgentChecklist";
import type { Project } from "@/lib/dashboard";
import { api, clearSession, rememberNext, session } from "@/lib/account";
import { withStepUp } from "@/lib/security";

type Pending = { deviceName: string; expiresAt: string };

export default function Connect() {
  const [code, setCode] = useState("");
  const [pending, setPending] = useState<Pending | null>(null);
  const [state, setState] = useState<"entering" | "checking" | "confirm" | "approved" | "declined">("entering");
  const [error, setError] = useState("");

  const look = async (c: string) => {
    setError("");
    setState("checking");
    try {
      setPending(await api<Pending>(`/devices/code/${encodeURIComponent(c)}`));
      setState("confirm");
    } catch (e) {
      if ((e as { status?: number }).status === 401) {
        clearSession();
        rememberNext(`/connect?code=${encodeURIComponent(c)}`);
        return window.location.replace("/signin");
      }
      setError((e as Error).message);
      setState("entering");
    }
  };

  useEffect(() => {
    const c = new URLSearchParams(window.location.search).get("code") ?? "";
    setCode(c);
    if (!session()) {
      rememberNext(`/connect${c ? `?code=${encodeURIComponent(c)}` : ""}`);
      window.location.replace("/signin");
      return;
    }
    if (c) look(c);
  }, []);

  const decide = async (approve: boolean) => {
    setError("");
    try {
      // An account with a second factor confirms it is you before a machine
      // joins it; one without is not asked.
      await withStepUp(() => api(`/devices/code/${encodeURIComponent(code)}/${approve ? "approve" : "deny"}`, { method: "POST", body: "{}" }));
      setState(approve ? "approved" : "declined");
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <div className="wrap account-page">
      <div className="account-card">
        <p className="eyebrow">trackline account</p>

        {(state === "entering" || state === "checking") && (
          <>
            <h1>Connect a machine</h1>
            <p className="account-sub">Enter the code <code>trackline connect</code> showed in your terminal.</p>
            <form
              className="connect-form"
              onSubmit={(e) => { e.preventDefault(); if (code.trim()) look(code.trim()); }}
            >
              <input
                className="code-input"
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
                placeholder="BCDF-GHJK"
                autoComplete="off"
                spellCheck={false}
                aria-label="Code"
              />
              <button className="btn btn-dark" type="submit" disabled={state === "checking"}>Continue</button>
            </form>
          </>
        )}

        {state === "confirm" && pending && (
          <>
            <h1>Connect this machine?</h1>
            <div className="connect-device">
              <span className="connect-name">{pending.deviceName}</span>
              <span className="connect-code">{code.toUpperCase()}</span>
            </div>
            <p className="account-sub">
              Only approve if <strong>you</strong> just ran <code>trackline connect</code> on this machine, and the code
              matches the one in your terminal. It will be able to send what trackline notices to your account.
            </p>
            <div className="account-actions">
              <button className="btn btn-signal" onClick={() => decide(true)}>Approve</button>
              <button className="btn btn-quiet" onClick={() => decide(false)}>Decline</button>
            </div>
          </>
        )}

        {state === "approved" && <Connected />}

        {state === "declined" && (
          <>
            <h1>Declined</h1>
            <p className="account-sub">That machine was not connected.</p>
          </>
        )}

        {error && <p className="account-error" role="alert">{error}</p>}
      </div>
    </div>
  );
}

// After approving: the project that was just set up, and each of its agents,
// ready or with the one step left. The terminal sends this a moment after
// approval, so it is asked for a few times.
function Connected() {
  const [project, setProject] = useState<Project | null>(null);
  useEffect(() => {
    const started = Date.now();
    let stop = false;
    const look = async () => {
      try {
        const r = await api<{ projects: Project[] }>("/app/overview");
        const recent = r.projects.find((p) => Date.now() - new Date(p.lastSeenAt).getTime() < 2 * 60_000 && p.agents.length);
        if (recent) { setProject(recent); if (recent.agents.every((a) => a.reported)) stop = true; }
      } catch { /* the page still says what to do next */ }
      if (!stop && Date.now() - started < 5 * 60_000) setTimeout(look, 3000);
    };
    look();
    return () => { stop = true; };
  }, []);
  return (
    <>
      <h1>Connected</h1>
      {!project && <p className="account-sub">Go back to your terminal. It finishes on its own in a few seconds.</p>}
      {project && (
        <>
          <p className="account-sub"><strong>{project.name}</strong> is connected. Your agents there:</p>
          <AgentChecklist agents={project.agents} />
        </>
      )}
      <div className="account-actions">
        <a className="btn btn-signal" href="/app">Open your dashboard</a>
      </div>
    </>
  );
}
