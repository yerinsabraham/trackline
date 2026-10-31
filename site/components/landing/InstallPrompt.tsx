"use client";

import Link from "next/link";
import { useState } from "react";
import { AGENT_PROMPT } from "@/lib/site";

// Most people install by asking their agent, so that is the main action. The
// line underneath confirms it, where the eye already is.
export default function InstallPrompt({ second }: { second: { href: string; label: string } }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(AGENT_PROMPT);
      setState("copied");
      setTimeout(() => setState("idle"), 4000);
    } catch {
      setState("failed");
    }
  };
  return (
    <>
      <div className="land-actions">
        <button type="button" className="land-btn signal" onClick={copy}>{state === "copied" ? "Copied" : "Copy the install prompt"}</button>
        <Link className="land-btn quiet" href={second.href}>{second.label}</Link>
      </div>
      <p className="land-small" aria-live="polite">
        {state === "copied" && "Now paste it into Claude Code, Codex or Cursor."}
        {state === "failed" && <>Could not copy. <Link href="/docs/install">The install guide has the prompt.</Link></>}
      </p>
    </>
  );
}
