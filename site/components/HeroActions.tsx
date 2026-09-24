"use client";

import Link from "next/link";
import { useState } from "react";
import { AGENT_PROMPT } from "@/lib/site";

// Most people install by asking their agent, so that is the primary action. The
// line underneath doubles as the confirmation, where the eye already is.
export default function HeroActions() {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(AGENT_PROMPT);
      setState("copied");
      setTimeout(() => setState("idle"), 4000);
    } catch {
      // Clipboard access can be refused; the prompt is shown in full further down.
      setState("failed");
    }
  };

  return (
    <>
      <div className="actions">
        <span className="frame">
          <Link className="btn btn-quiet" href="/docs/install"><span className="plus">+</span>Install guide</Link>
        </span>
        <span className="frame">
          <button className="btn btn-signal" type="button" onClick={copy}>
            <span className="plus">+</span>Copy prompt for your agent
          </button>
        </span>
      </div>
      <p className={state === "copied" ? "install-line done" : "install-line"} aria-live="polite">
        {state === "copied" && "Copied. Paste it into Claude Code, Codex or Cursor."}
        {state === "failed" && <>Could not copy. The prompt is under <a href="#install">Install</a>.</>}
        {state === "idle" && "Paste it into Claude Code, Codex or Cursor, and it installs and checks itself."}
      </p>
    </>
  );
}
