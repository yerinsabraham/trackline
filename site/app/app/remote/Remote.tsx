"use client";

import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/account";
import { whileVisible } from "@/lib/dashboard";
import {
  active, type AgentEvent, type Job, jobState, type Machine, type Passkey, passkeysSupported, protectedMacArea, signPrompt,
} from "@/lib/remote";
import AgentLogo, { agentName } from "@/components/app/AgentLogo";
import AppShell, { useApp } from "@/components/app/AppShell";
import { Activity, AgentReply, Composer, Finding, type Line, MyMessage } from "@/components/app/Chat";
import { IconBack, IconFace, IconLaptop, IconStop } from "@/components/app/icons";

// A task for an agent on your laptop: choose where, say what, sign and send.
// Then the same conversation view as a session, fed by the laptop as it goes.

const ASKED = "trackline.asked.";
const remember = (job: string, text: string) => { try { sessionStorage.setItem(ASKED + job, text); } catch { /* private mode */ } };
const recall = (job: string) => { try { return sessionStorage.getItem(ASKED + job) ?? undefined; } catch { return undefined; } };

export default function Remote() {
  const [job, setJob] = useState<string | null | undefined>(undefined);
  useEffect(() => {
    const read = () => setJob(new URLSearchParams(window.location.search).get("job"));
    read();
    window.addEventListener("popstate", read);
    return () => window.removeEventListener("popstate", read);
  }, []);
  const open = (id: string) => { window.history.pushState(null, "", `/app/remote?job=${encodeURIComponent(id)}`); setJob(id); };

  if (job === undefined) return <AppShell section="new"><div className="skel" style={{ height: 320 }} /></AppShell>;
  return job
    ? <AppShell section="new" chat><JobChat id={job} onNew={() => { window.history.pushState(null, "", "/app/remote"); setJob(null); }} open={open} /></AppShell>
    : <AppShell section="new"><NewTask open={open} /></AppShell>;
}

function NewTask({ open }: { open: (id: string) => void }) {
  const { remote } = useApp();
  const [keys, setKeys] = useState<Passkey[] | null>(null);
  const [machineId, setMachineId] = useState("");
  const [projectId, setProjectId] = useState("");
  const [agent, setAgent] = useState("");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => { api<{ passkeys: Passkey[] }>("/app/remote/passkeys").then((r) => setKeys(r.passkeys)).catch(() => setKeys([])); }, []);

  const machines = remote?.machines ?? [];
  const machine: Machine | undefined = machines.find((m) => m.id === machineId) ?? machines.find((m) => m.online) ?? machines[0];
  const project = machine?.projects.find((p) => p.id === projectId) ?? machine?.projects[0];
  const agents = project?.agents ?? [];
  const chosen = agents.includes(agent) ? agent : agents[0] ?? "";
  const protectedArea = protectedMacArea(text);

  const send = async () => {
    if (!machine || !project || !chosen || !keys?.length || !text.trim()) return;
    setBusy(true); setError("");
    try {
      const body = await signPrompt(machine.id, project.id, chosen, text.trim(), keys);
      const r = await api<{ id: string }>("/app/remote/jobs", { method: "POST", body: JSON.stringify(body) });
      remember(r.id, text.trim());
      open(r.id);
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };

  if (!remote) return <div className="skel" style={{ height: 320 }} />;
  return (
    <div className="new-task">
      <header className="rise">
        <h1 className="app-h1">New task</h1>
        <p className="app-sub">It runs on your laptop, with your own agent, watched by trackline.</p>
      </header>

      {!passkeysSupported() && <p className="composer-note error" style={{ textAlign: "left" }}>This browser cannot use passkeys, which sending needs.</p>}
      {keys?.length === 0 && (
        <div className="card empty">
          <strong>Add a passkey first.</strong>
          <span>Every task is signed with it, so only you can send one.</span>
          <a className="btn-act" href="/account">Add a passkey</a>
        </div>
      )}
      {machines.length === 0 && (
        <div className="card empty">
          <span className="agent-logo" style={{ width: 44, height: 44 }}><span style={{ width: 22, display: "flex" }}><IconLaptop /></span></span>
          <strong>No laptop takes tasks yet.</strong>
          <span>On your laptop, in a project: <code>trackline remote enable</code></span>
        </div>
      )}

      {machine && (
        <form className="new-task-form rise rise-1" onSubmit={(e) => { e.preventDefault(); send(); }}>
          <div className="card" style={{ display: "flex", alignItems: "center", gap: 12, padding: 14 }}>
            <span className="agent-logo" style={{ width: 40, height: 40 }}><span style={{ width: 20, display: "flex" }}><IconLaptop /></span></span>
            <div style={{ display: "grid", gap: 2, flexGrow: 1, minWidth: 0 }}>
              <strong style={{ fontSize: 14.5 }}>{machine.name}</strong>
              <span style={{ fontSize: 12.5, display: "flex", alignItems: "center", gap: 6, color: machine.stopped ? "var(--red-ink)" : machine.online ? "var(--ok-ink)" : "var(--muted)" }}>
                <span className={`dot ${machine.online && !machine.stopped ? "working" : ""}`} />
                {machine.stopped ? "Stopped. Turn it on again on the laptop." : machine.online ? "Online" : "Asleep or offline. A task waits 3 minutes."}
              </span>
            </div>
            {machines.length > 1 && (
              <select aria-label="Laptop" className="chip" value={machine.id} onChange={(e) => { setMachineId(e.target.value); setProjectId(""); }}>
                {machines.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
              </select>
            )}
          </div>

          {machine.projects.length === 0 ? (
            <p className="app-muted" style={{ margin: 0 }}>No project has remote on. In one: <code className="mono">trackline remote enable</code></p>
          ) : (
            <>
              <fieldset className="nt-field">
                <legend>Project</legend>
                <div className="chips">
                  {machine.projects.map((p) => (
                    <button type="button" key={p.id} className="chip mono" aria-pressed={p.id === project?.id} onClick={() => setProjectId(p.id)}>{p.name}</button>
                  ))}
                </div>
              </fieldset>
              <fieldset className="nt-field">
                <legend>Agent</legend>
                {agents.length === 0 ? (
                  <p className="app-muted" style={{ margin: 0, fontSize: 14 }}>trackline is not set up for an agent in {project?.name}. There: <code className="mono">trackline init</code></p>
                ) : (
                  <div className="nt-agents">
                    {agents.map((a) => (
                      <button type="button" key={a} className="nt-agent" aria-pressed={a === chosen} onClick={() => setAgent(a)}>
                        <AgentLogo host={a} size={30} />{agentName(a)}
                      </button>
                    ))}
                  </div>
                )}
              </fieldset>
              <div className="nt-field">
                <label htmlFor="task">What should {agentName(chosen)} do?</label>
                <textarea id="task" className="nt-text" value={text} onChange={(e) => setText(e.target.value)}
                  placeholder="Make the contact form's email required and show the error under the field." />
              </div>
              {protectedArea && (
                <p className="composer-note warn" style={{ textAlign: "left" }}>
                  This may need local approval on your Mac to access {protectedArea}. If macOS asks, approve it there; the phone will show that it is waiting.
                </p>
              )}
              {error && <p className="composer-note error" style={{ textAlign: "left" }}>{error}</p>}
              <button type="submit" className="btn-act nt-send" disabled={busy || !text.trim() || !chosen || !keys?.length || machine.stopped}>
                <span style={{ width: 18, display: "flex" }}><IconFace /></span>{busy ? "Waiting for your passkey…" : "Sign and send"}
              </button>
              <p className="composer-note">Secrets and new dependencies stay blocked in tasks sent from here. For unattended work, keep files inside this project when you can.</p>
            </>
          )}
        </form>
      )}
    </div>
  );
}

type Item = { kind: "activity"; lines: Line[] } | { kind: "say"; text: string; key: string } | { kind: "permission"; text: string; key: string } | { kind: "error"; text: string; key: string };

function items(events: AgentEvent[]): Item[] {
  const out: Item[] = [];
  let lastSay = "";
  for (const e of events) {
    if (e.kind === "tool" || e.kind === "denied") {
      const l: Line = {
        key: String(e.seq), time: new Date(e.createdAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
        verb: e.kind === "denied" ? "Not allowed" : e.tool ?? "Tool", text: e.text ?? "", tone: e.kind === "denied" ? "block" : undefined,
      };
      const prev = out[out.length - 1];
      if (prev?.kind === "activity") prev.lines.push(l); else out.push({ kind: "activity", lines: [l] });
    } else if (e.kind === "permission" && e.text) {
      out.push({ kind: "permission", text: e.text, key: String(e.seq) });
    } else if (e.kind === "say" && e.text) {
      out.push({ kind: "say", text: e.text, key: String(e.seq) });
      lastSay = e.text;
    } else if (e.kind === "done" && e.text && e.text !== lastSay) {
      out.push({ kind: "say", text: e.text, key: String(e.seq) });
    } else if (e.kind === "error" && e.text) {
      out.push({ kind: "error", text: e.text, key: String(e.seq) });
    }
  }
  return out;
}

function JobChat({ id, onNew, open }: { id: string; onNew: () => void; open: (id: string) => void }) {
  const [job, setJob] = useState<Job | null>(null);
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [stopping, setStopping] = useState(false);
  const [missing, setMissing] = useState(false);
  const after = useRef(-1);
  const scroller = useRef<HTMLDivElement>(null);
  const keys = useRef<Passkey[] | null>(null);
  const { remote } = useApp();
  const asked = recall(id);

  useEffect(() => {
    after.current = -1;
    setEvents([]); setJob(null);
    let busy = false;
    const load = async () => {
      if (busy) return;
      busy = true;
      try {
        const [j, e] = await Promise.all([
          api<Job>(`/app/remote/jobs/${encodeURIComponent(id)}`),
          api<{ events: AgentEvent[] }>(`/app/remote/jobs/${encodeURIComponent(id)}/events?after=${after.current}`),
        ]);
        if (e.events.length) {
          after.current = e.events[e.events.length - 1].seq;
          setEvents((old) => [...old, ...e.events]);
        }
        setJob(j);
      } catch (err) {
        if ((err as { status?: number }).status === 404) setMissing(true);
      } finally {
        busy = false;
      }
    };
    load();
    return whileVisible(load, 1200);
  }, [id]);

  useEffect(() => {
    const el = scroller.current;
    if (el && el.scrollHeight - el.scrollTop - el.clientHeight < 240) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  }, [events.length, job?.status]);

  const machineName = remote?.machines.find((m) => m.id === job?.machine)?.name ?? "your laptop";
  const live = !!job && active(job.status);
  const list = items(events);
  const canContinue = job?.agent && job.session && job.machine && job.project;

  const send = async (text: string) => {
    if (!job || !canContinue) return;
    if (!keys.current) keys.current = (await api<{ passkeys: Passkey[] }>("/app/remote/passkeys")).passkeys;
    if (!keys.current.length) throw new Error("Add a passkey on your account page first.");
    const body = await signPrompt(job.machine!, job.project!, job.agent!, text, keys.current, job.session!);
    const r = await api<{ id: string }>("/app/remote/jobs", { method: "POST", body: JSON.stringify(body) });
    remember(r.id, text);
    open(r.id);
  };

  return (
    <div className="chat-page single">
      <section className="chat-pane">
        <header className="chat-head">
          <button className="app-icon-btn" aria-label="New task" onClick={onNew}><IconBack /></button>
          {job?.agent && <AgentLogo host={job.agent} size={32} />}
          <div style={{ display: "grid", gap: 2, minWidth: 0, flexGrow: 1 }}>
            <h1>{asked ?? (job ? `${agentName(job.agent)} task` : "Task")}</h1>
            <span className="row-meta">{job ? `${agentName(job.agent)} · ${machineName} · ${jobState(job)}` : ""}</span>
          </div>
          {live && (
            <button className="btn-plain" style={{ height: 36, padding: "0 12px" }} disabled={stopping} onClick={async () => {
              setStopping(true);
              try { await api(`/app/remote/jobs/${encodeURIComponent(id)}/stop`, { method: "POST", body: "{}" }); } finally { setStopping(false); }
            }}><span style={{ width: 12, display: "flex" }}><IconStop /></span>{stopping ? "Stopping…" : "Stop"}</button>
          )}
        </header>

        <div className="chat-scroll" ref={scroller}>
          <div className="chat">
            {missing && <p className="composer-note error">This task does not exist, or is not yours.</p>}
            {asked && <MyMessage text={asked} note={job ? `Sent to ${machineName}` : undefined} />}
            {!job && !missing && <div className="skel" style={{ height: 90 }} />}
            {job?.status === "queued" && <p className="chat-divider">Waiting for {machineName} to pick it up…</p>}
            {list.map((it, i) => it.kind === "activity"
              ? <Activity key={`a${i}`} lines={it.lines} live={live && i === list.length - 1} />
              : it.kind === "permission" ? <Finding key={it.key} tone="block" title="Action needed on Mac">{it.text}</Finding>
              : it.kind === "say" ? <AgentReply key={it.key} host={job?.agent ?? null} text={it.text} />
              : <Finding key={it.key} tone="block" title="The agent stopped with an error">{it.text}</Finding>)}
            {live && list[list.length - 1]?.kind !== "activity" && job?.status === "delivered" && <Activity lines={[]} live />}
            {job && !live && job.status !== "done" && (
              <Finding tone={job.status === "refused" ? "block" : "warn"} title={jobState(job)}>
                {job.reason && job.reason !== jobState(job) && job.status !== "refused" ? job.reason : undefined}
              </Finding>
            )}
            {job?.dashboardSession && !live && (
              <a className="chat-divider" href={`/app/session?id=${encodeURIComponent(job.dashboardSession)}`} style={{ textDecoration: "underline" }}>Open the full session</a>
            )}
          </div>
        </div>

        <div className="chat-compose">
          <Composer
            placeholder={`Reply to ${agentName(job?.agent)}…`}
            blocked={!job ? "Loading…" : live ? `${agentName(job.agent)} is still working. Reply when it has finished.` : !canContinue ? "This task has no session to continue." : undefined}
            note={`Signed with your passkey · continues this session on ${machineName}`}
            onSend={send}
          />
        </div>
      </section>
    </div>
  );
}
