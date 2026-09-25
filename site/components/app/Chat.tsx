"use client";

import { useRef, useState, type ReactNode } from "react";
import ReplyText from "@/components/ReplyText";
import AgentLogo from "./AgentLogo";
import { IconAlert, IconShield, IconTerminal, IconUp } from "./icons";

// The pieces a conversation with an agent is drawn from, whichever way it
// arrived: from trackline's hook in the terminal, or from a task sent here.

export type Line = { key: string; time?: string; verb: string; text: string; tone?: "ok" | "warn" | "block" | "dim"; why?: string };

export function MyMessage({ text, note }: { text: string; note?: string }) {
  return (
    <div className="bubble-me rise">
      <div>{text}</div>
      {note && <span>{note}</span>}
    </div>
  );
}

/** What the agent did, as a terminal would show it. */
export function Activity({ lines, live }: { lines: Line[]; live?: boolean }) {
  if (!lines.length && !live) return null;
  return (
    <div className="term rise">
      <div className="term-head">
        <span style={{ width: 14, display: "flex" }}><IconTerminal /></span>
        <span>Activity · {lines.length} step{lines.length === 1 ? "" : "s"}</span>
        {live && <span className="live">live</span>}
      </div>
      {lines.map((l) => (
        <div key={l.key} className="term-line">
          {l.time && <span className="t">{l.time}</span>}
          <span className={`v ${l.tone ?? ""}`}>{l.verb}</span>
          <span className="x">{l.text}{l.why && <span className="why"> · {l.why}</span>}</span>
        </div>
      ))}
      {live && (
        <div className="term-line"><span className="v dim">Working</span><span className="term-caret" aria-hidden="true" /></div>
      )}
    </div>
  );
}

export function Finding({ tone, title, children }: { tone: "block" | "warn"; title: ReactNode; children?: ReactNode }) {
  return (
    <div className={`finding ${tone === "warn" ? "warn" : ""}`}>
      <span style={{ width: 20, flex: "none", color: tone === "warn" ? "#9a6700" : "#c8321a", display: "flex" }}>
        {tone === "warn" ? <IconAlert /> : <IconShield />}
      </span>
      <div><strong>{title}</strong>{children && <p>{children}</p>}</div>
    </div>
  );
}

/** The agent's words, rendered as the Markdown it wrote, never as raw marks. */
export function AgentReply({ host, text }: { host: string | null; text: string }) {
  return (
    <div className="bubble-agent">
      <AgentLogo host={host} size={30} />
      <div className="body"><ReplyText text={text} /></div>
    </div>
  );
}

/**
 * Where you type to the agent. When nothing can be sent, the box says why
 * rather than taking text that goes nowhere.
 */
export function Composer({ placeholder, onSend, blocked, note }: {
  placeholder: string;
  onSend: (text: string) => Promise<void>;
  blocked?: string;
  note?: string;
}) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const box = useRef<HTMLTextAreaElement>(null);

  const grow = () => {
    const el = box.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 180)}px`;
  };

  const send = async () => {
    const t = text.trim();
    if (!t || busy || blocked) return;
    setBusy(true); setError("");
    try {
      await onSend(t);
      setText("");
      requestAnimationFrame(grow);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="composer" onSubmit={(e) => { e.preventDefault(); send(); }}>
      <div className="composer-box">
        <label htmlFor="composer" style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)" }}>{placeholder}</label>
        <textarea
          id="composer" ref={box} rows={1} value={text} placeholder={blocked ? "Replying is not available here" : placeholder}
          disabled={!!blocked || busy}
          onChange={(e) => { setText(e.target.value); grow(); }}
          onKeyDown={(e) => {
            // Enter sends on a keyboard with one; a phone keeps Enter for new lines.
            if (e.key === "Enter" && !e.shiftKey && window.matchMedia("(min-width: 900px)").matches) { e.preventDefault(); send(); }
          }}
        />
        <button type="submit" className="composer-send" aria-label="Sign and send" disabled={!text.trim() || busy || !!blocked}>
          <span style={{ width: 18, display: "flex" }}><IconUp /></span>
        </button>
      </div>
      <span className={`composer-note ${error || blocked ? "error" : ""}`}>
        {error || blocked || (busy ? "Waiting for your passkey…" : note)}
      </span>
    </form>
  );
}
