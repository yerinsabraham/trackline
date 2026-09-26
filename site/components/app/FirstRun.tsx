"use client";

import { useState } from "react";
import { AGENT_PROMPT } from "@/lib/site";
import { WAKE, type Project } from "@/lib/dashboard";
import { agentName } from "./AgentLogo";
import AgentLogo from "./AgentLogo";
import { IconLaptop } from "./icons";

// What a new account sees instead of an empty dashboard. Each step ticks
// itself off from what the laptop reports, so there is nothing to click
// through: the page moves on when the work is done.

function Copy({ text, label = "Copy" }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button" className="btn-plain fr-copy"
      onClick={async () => {
        try { await navigator.clipboard.writeText(text); setDone(true); setTimeout(() => setDone(false), 1800); }
        catch { /* the text is on screen to copy by hand */ }
      }}
    >{done ? "Copied" : label}</button>
  );
}

function Cmd({ text }: { text: string }) {
  return <div className="fr-cmd"><code>{text}</code><Copy text={text} /></div>;
}

export default function FirstRun({ projects }: { projects: Project[] }) {
  const connected = projects.length > 0;
  const [prompted, setPrompted] = useState(false);
  // An agent that is wired but has never reported is the one place people
  // get stuck after connecting, and each host is woken differently.
  const waiting = projects.flatMap((p) => p.agents).filter((a) => a.wired && !a.reported);
  const step = connected ? 3 : 1;

  return (
    <section className="card fr rise rise-1" aria-labelledby="fr-title">
      <div className="fr-head">
        <h2 id="fr-title">Set up trackline</h2>
        <p className="fr-laptop"><span style={{ width: 16, display: "flex" }}><IconLaptop /></span>Do this on your laptop.</p>
      </div>

      <ol className="fr-steps">
        <li className={`fr-step ${connected ? "done" : "now"}`}>
          <span className="fr-n">{connected ? "✓" : 1}</span>
          <div className="fr-body">
            <strong>Install it in a project</strong>
            {!connected && <>
              <p>Paste this into Claude Code, Codex or Cursor.</p>
              <div className="fr-actions">
                <button type="button" className="btn-act" onClick={async () => {
                  try { await navigator.clipboard.writeText(AGENT_PROMPT); setPrompted(true); } catch { /* shown below */ }
                }}>{prompted ? "Copied" : "Copy prompt for your agent"}</button>
              </div>
              <details className="fr-more">
                <summary>Or run it yourself</summary>
                <Cmd text="npm install -g trackline" />
                <Cmd text="trackline init" />
              </details>
            </>}
          </div>
        </li>

        <li className={`fr-step ${connected ? "done" : "now"}`}>
          <span className="fr-n">{connected ? "✓" : 2}</span>
          <div className="fr-body">
            <strong>Connect it to this account</strong>
            {connected
              ? <p>{projects.map((p) => p.name).join(", ")}</p>
              : <>
                  <p>In the same project. It opens this site to approve.</p>
                  <Cmd text="trackline connect" />
                  <p className="fr-wait"><span className="dot working" />Waiting for your laptop</p>
                </>}
          </div>
        </li>

        <li className={`fr-step ${step === 3 ? "now" : ""}`}>
          <span className="fr-n">3</span>
          <div className="fr-body">
            <strong>Give your agent a task</strong>
            {step === 3 && <>
              <p>Its first session shows up here as it happens.</p>
              {waiting.map((a) => (
                <p key={a.host} className="fr-hint"><AgentLogo host={a.host} size={22} />{WAKE[a.host]?.text ?? `Send a message in ${agentName(a.host)}.`}</p>
              ))}
              <p className="fr-wait"><span className="dot working" />Waiting for the first session</p>
            </>}
          </div>
        </li>
      </ol>
    </section>
  );
}
