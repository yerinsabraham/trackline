"use client";

import CopyButton from "@/components/CopyButton";
import { ago, type Agent, HOST_LABEL, WAKE } from "@/lib/dashboard";
import "@/app/app/dashboard.css";

// Each agent in a project: ready once it has reported, otherwise the one step
// that wakes it. A hook that is set up but never fires looks like one with
// nothing to say, so the difference is shown rather than left to be guessed.
export default function AgentChecklist({ agents }: { agents: Agent[] }) {
  if (agents.length === 0) return null;
  return (
    <ul className="agent-list">
      {agents.map((a) => (
        <li key={a.host} className={a.reported ? "ready" : "waiting"}>
          <span className="agent-mark" aria-hidden>{a.reported ? "✓" : "…"}</span>
          <div>
            <strong>{HOST_LABEL[a.host] ?? a.host}</strong>
            {a.reported ? (
              <span className="agent-note">Ready{a.lastAt ? ` · last active ${ago(a.lastAt)}` : ""}</span>
            ) : (
              <span className="agent-note">
                {WAKE[a.host]?.text ?? "Not heard from yet."}
                {WAKE[a.host]?.copy && (
                  <span className="agent-copy"><code>{WAKE[a.host].copy}</code><CopyButton text={WAKE[a.host].copy!} /></span>
                )}
              </span>
            )}
          </div>
        </li>
      ))}
    </ul>
  );
}
