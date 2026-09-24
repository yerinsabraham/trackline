"use client";

import { useState } from "react";
import CopyButton from "@/components/CopyButton";
import { AGENT_PROMPT } from "@/lib/site";

const COMMANDS = "npm install -g trackline\ntrackline init";

export default function InstallTabs() {
  const [tab, setTab] = useState<"agent" | "terminal">("agent");
  return (
    <div className="term">
      <div className="term-bar">
        <div className="tabs" role="tablist" aria-label="How to install">
          <button type="button" role="tab" id="tab-agent" aria-selected={tab === "agent"} onClick={() => setTab("agent")}>Your agent</button>
          <button type="button" role="tab" id="tab-terminal" aria-selected={tab === "terminal"} onClick={() => setTab("terminal")}>Terminal</button>
        </div>
        <CopyButton text={tab === "agent" ? AGENT_PROMPT : COMMANDS} />
      </div>
      {tab === "agent" ? (
        <pre role="tabpanel" aria-labelledby="tab-agent" className="prompt">{AGENT_PROMPT}</pre>
      ) : (
        <pre role="tabpanel" aria-labelledby="tab-terminal">
          <span className="p">$ </span>npm install -g trackline{"\n"}
          <span className="p">$ </span>trackline init{"              "}<span className="c"># Claude Code; --host codex or --host cursor</span>
        </pre>
      )}
    </div>
  );
}
