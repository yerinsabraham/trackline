"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api, clearSession, rememberNext, session } from "@/lib/account";
import { ago, whileVisible } from "@/lib/dashboard";
import { withStepUp } from "@/lib/security";
import {
  active, AGENT_LABEL, type AgentEvent, type Job, jobState, type Machine, type Overview, type Passkey,
  type Pairing, passkeysSupported, signPairing, signPrompt,
} from "@/lib/remote";
import "../dashboard.css";

const HERE = "/app/remote";

function machineLine(m: Machine): string {
  if (m.stopped) return "Stopped. Turn it on again on the laptop: trackline remote enable";
  if (m.online) return "Ready";
  return m.lastSeenAt ? `Asleep or offline, last seen ${ago(m.lastSeenAt)}. A message waits 3 minutes.` : "Not seen yet.";
}

export default function Remote() {
  const [data, setData] = useState<Overview | null>(null);
  const [keys, setKeys] = useState<Passkey[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    Promise.all([api<Overview>("/app/remote"), api<{ passkeys: Passkey[] }>("/app/remote/passkeys")])
      .then(([o, p]) => { setData(o); setKeys(p.passkeys); })
      .catch((e) => {
        const status = (e as { status?: number }).status;
        if (status === 401) { clearSession(); rememberNext(HERE); window.location.replace("/signin"); }
        // The API answers 404 until remote is switched on for the account service.
        else if (status === 404) setError("Remote is not switched on yet.");
        else setError((e as Error).message);
      });
  }, []);

  useEffect(() => {
    if (!session()) { rememberNext(HERE); return window.location.replace("/signin"); }
    load();
    return whileVisible(load, 10_000);
  }, [load]);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true); setError("");
    try { await fn(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); }
  };

  const [job, setJob] = useState<string | null>(null);
  // What was asked, for a message sent from this page. The server drops the
  // signed message once the laptop reports, so older ones show without it.
  const [asked, setAsked] = useState<Record<string, string>>({});
  // A pairing leaves the server's waiting list the moment the phone answers,
  // but its card must stay to say whether the laptop trusted it.
  const [held, setHeld] = useState<Pairing[]>([]);
  const pairings = data ? [...data.pairings, ...held.filter((h) => !data.pairings.some((p) => p.id === h.id))] : [];

  return (
    <div className="wrap dash">
      <a href="/app" className="dash-back">← Dashboard</a>
      <div className="dash-head">
        <p className="eyebrow">Remote</p>
        <h1>Send your agent a task</h1>
      </div>
      {error && <p className="account-error" role="alert">{error}</p>}
      {!data && !error && <p className="dash-muted">Loading…</p>}

      {data && keys && (
        <>
          {!passkeysSupported() && <p className="dash-muted">This browser cannot use passkeys, which remote needs.</p>}
          {keys.length === 0 && (
            <div className="dash-empty">
              <p>Add a passkey first. Every message to your laptop is signed with it.</p>
              <p><a className="btn btn-signal" href="/account">Add a passkey</a></p>
            </div>
          )}

          {pairings.map((p) => (
            <PairCard key={p.id} pairing={p} keys={keys} busy={busy} act={act}
              hold={() => setHeld((h) => [...h, p])}
              release={() => { setHeld((h) => h.filter((x) => x.id !== p.id)); load(); }} />
          ))}

          {data.machines.length === 0 && data.pairings.length === 0 && (
            <div className="dash-empty">
              <p>No laptop has remote on.</p>
              <p className="dash-muted">On your laptop, in a project: <code>trackline remote enable</code></p>
            </div>
          )}

          {job ? (
            <JobView id={job} asked={asked[job]} onBack={() => { setJob(null); load(); }} />
          ) : (
            <>
              {data.machines.length > 0 && keys.length > 0 && (
                <Composer machines={data.machines} keys={keys} busy={busy} act={act}
                  sent={(id, text) => { setAsked((a) => ({ ...a, [id]: text })); setJob(id); }} />
              )}
              {data.jobs.length > 0 && (
                <section className="dash-project">
                  <h2>Recent</h2>
                  <ul className="dash-sessions">
                    {data.jobs.map((j) => (
                      <li key={j.id}>
                        <button className={`dash-session remote-job ${active(j.status) ? "light-working" : j.status === "done" ? "light-done" : "light-idle"}`} onClick={() => setJob(j.id)}>
                          <span className="dash-dot" aria-hidden />
                          <span className="dash-session-main">
                            <span className="dash-session-top">
                              <strong>{jobState(j)}</strong>
                              <span>{j.agent ? AGENT_LABEL[j.agent] ?? j.agent : "Test"} · {j.machine} · {ago(j.createdAt)}</span>
                            </span>
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                </section>
              )}
            </>
          )}

          {data.machines.some((m) => !m.stopped) && (
            <p className="dash-muted dash-help">
              <button className="link-danger" disabled={busy} onClick={() => act(async () => {
                if (!window.confirm("Stop remote on every laptop? Starting again takes the laptop.")) return;
                await api("/app/remote/stop", { method: "POST", body: "{}" });
                load();
              })}>Stop remote everywhere</button>
            </p>
          )}
        </>
      )}
    </div>
  );
}

type Act = (fn: () => Promise<void>) => Promise<void>;

function PairCard({ pairing, keys, busy, act, hold, release }: {
  pairing: Pairing; keys: Passkey[]; busy: boolean; act: Act; hold: () => void; release: () => void;
}) {
  const [code, setCode] = useState("");
  const [result, setResult] = useState("");
  const [finished, setFinished] = useState(false);

  const pair = () => act(async () => {
    hold();
    // Signed once: if the server first asks to confirm it is you, only the
    // sending is retried, not the signing.
    const answer = await signPairing(pairing.machineId, code, keys);
    await withStepUp(() => api(`/app/remote/pairings/${pairing.id}/answer`, { method: "POST", body: JSON.stringify(answer) }));
    setResult("Waiting for the laptop to check it…");
    for (let i = 0; i < 30; i++) {
      await new Promise((r) => setTimeout(r, 1000));
      const s = await api<{ status: string; reason: string | null }>(`/app/remote/pairings/${pairing.id}`);
      if (s.status === "trusted") { setResult("Paired."); setFinished(true); return; }
      if (s.status === "refused") { setResult(`The laptop refused it: ${s.reason ?? "the code did not match"}. Start again on the laptop.`); setFinished(true); return; }
    }
    setResult("The laptop has not answered. Check it is still waiting.");
    setFinished(true);
  });

  return (
    <section className="dash-empty remote-pair">
      <p><strong>{pairing.machine}</strong> wants to pair.</p>
      <p className="dash-muted">Enter the code it shows.</p>
      <input className="account-input remote-code" value={code} onChange={(e) => setCode(e.target.value)}
        placeholder="XXXX-XXXX" autoCapitalize="characters" autoComplete="off" spellCheck={false} />
      <button className="btn btn-signal" disabled={busy || code.replace(/[-\s]/g, "").length !== 8 || keys.length === 0} onClick={pair}>Pair</button>
      {result && <p className="account-note">{result}</p>}
      {finished && <button className="btn btn-quiet" onClick={release}>OK</button>}
    </section>
  );
}

function Composer({ machines, keys, busy, act, sent }: { machines: Machine[]; keys: Passkey[]; busy: boolean; act: Act; sent: (id: string, text: string) => void }) {
  const [machineId, setMachineId] = useState(machines[0].id);
  const machine = machines.find((m) => m.id === machineId) ?? machines[0];
  const [projectId, setProjectId] = useState(machine.projects[0]?.id ?? "");
  const project = machine.projects.find((p) => p.id === projectId) ?? machine.projects[0];
  const [agent, setAgent] = useState("");
  const [text, setText] = useState("");
  const agents = project?.agents ?? [];
  const chosen = agents.includes(agent) ? agent : agents[0] ?? "";

  const send = () => act(async () => {
    const body = await signPrompt(machine.id, project.id, chosen, text.trim(), keys);
    const r = await api<{ id: string }>("/app/remote/jobs", { method: "POST", body: JSON.stringify(body) });
    sent(r.id, text.trim());
    setText("");
  });

  return (
    <section className="remote-composer">
      {machines.length > 1 && (
        <select className="account-input" value={machine.id} onChange={(e) => { setMachineId(e.target.value); setProjectId(""); }}>
          {machines.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
        </select>
      )}
      <p className={`dash-muted small remote-machine ${machine.online && !machine.stopped ? "on" : ""}`}>
        {machines.length === 1 && <strong>{machine.name} · </strong>}{machineLine(machine)}
      </p>
      {machine.projects.length === 0 ? (
        <p className="dash-muted">No project has remote on. In one: <code>trackline remote enable</code></p>
      ) : (
        <>
          <select className="account-input" value={project.id} onChange={(e) => setProjectId(e.target.value)}>
            {machine.projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
          {agents.length === 0 ? (
            <p className="dash-muted small">trackline is not set up for an agent in {project.name}. There: <code>trackline init</code></p>
          ) : (
            <div className="remote-agents" role="radiogroup">
              {agents.map((a) => (
                <button key={a} role="radio" aria-checked={a === chosen} className={`btn ${a === chosen ? "btn-signal" : "btn-quiet"}`} onClick={() => setAgent(a)}>
                  {AGENT_LABEL[a] ?? a}
                </button>
              ))}
            </div>
          )}
          <textarea className="account-input remote-text" rows={5} value={text} onChange={(e) => setText(e.target.value)}
            placeholder={`What should ${AGENT_LABEL[chosen] ?? "the agent"} do in ${project.name}?`} />
          <button className="btn btn-signal remote-send" disabled={busy || !chosen || !text.trim() || machine.stopped} onClick={send}>
            {busy ? "Sending…" : "Sign and send"}
          </button>
        </>
      )}
    </section>
  );
}

function JobView({ id, asked, onBack }: { id: string; asked?: string; onBack: () => void }) {
  const [job, setJob] = useState<Job | null>(null);
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [stopping, setStopping] = useState(false);
  const after = useRef(-1);

  useEffect(() => {
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
      } catch { /* the next poll tries again */ } finally {
        busy = false;
      }
    };
    load();
    return whileVisible(load, 1500);
  }, [id]);

  // The final answer usually repeats the agent's last message; shown once.
  const shown = events.filter((e, i) => e.kind !== "session" && !(e.kind === "done" && events[i - 1]?.kind === "say" && events[i - 1].text === e.text));

  return (
    <section className="remote-job-view">
      <button className="dash-back link-plain" onClick={onBack}>← New message</button>
      {!job && <p className="dash-muted">Loading…</p>}
      {job && (
        <>
          <div className={`dash-status ${active(job.status) ? "light-working" : job.status === "done" ? "light-done" : "light-idle"}`}>
            <div className="dash-status-top">
              <span className="dash-dot" aria-hidden />
              <strong>{jobState(job)}</strong>
              {active(job.status) && <span className="dash-live">Live</span>}
            </div>
            <p className="dash-muted small">{job.agent ? AGENT_LABEL[job.agent] ?? job.agent : "Test"} · sent {ago(job.createdAt)}</p>
          </div>
          {asked && <div className="dash-asked"><p className="dash-muted small">You asked</p><p>{asked}</p></div>}
          <ol className="remote-events">
            {shown.map((e) => (
              <li key={e.seq} className={`remote-event remote-${e.kind}`}>
                {e.kind === "say" && <p>{e.text}</p>}
                {e.kind === "tool" && <code>{e.tool} {e.text}</code>}
                {e.kind === "denied" && <p><strong>Not allowed:</strong> <code>{e.tool} {e.text}</code></p>}
                {e.kind === "done" && <p>{e.text}</p>}
                {e.kind === "error" && <p><strong>Error:</strong> {e.text}</p>}
              </li>
            ))}
          </ol>
          {active(job.status) && (
            <button className="btn btn-danger" disabled={stopping} onClick={async () => {
              setStopping(true);
              try { await api(`/app/remote/jobs/${encodeURIComponent(id)}/stop`, { method: "POST", body: "{}" }); } finally { setStopping(false); }
            }}>Stop</button>
          )}
        </>
      )}
    </section>
  );
}
