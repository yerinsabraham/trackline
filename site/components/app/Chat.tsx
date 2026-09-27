"use client";

import { useRef, useState, type ReactNode } from "react";
import ReplyText from "@/components/ReplyText";
import AgentLogo from "./AgentLogo";
import { IconAlert, IconClip, IconClose, IconShield, IconTerminal, IconUp } from "./icons";
import { type Attachment, MAX_ATTACHMENTS, prepareImage } from "@/lib/remote";

// The pieces a conversation with an agent is drawn from, whichever way it
// arrived: from trackline's hook in the terminal, or from a task sent here.

export type Line = { key: string; time?: string; verb: string; text: string; tone?: "ok" | "warn" | "block" | "dim"; why?: string };

export function MyMessage({ text, note, images }: { text: string; note?: string; images?: string[] }) {
  return (
    <div className="bubble-me rise">
      {images && images.length > 0 && (
        <div className="bubble-images">{images.map((u) => <img key={u} src={u} alt="Attached image" />)}</div>
      )}
      <div>{text}</div>
      {note && <span>{note}</span>}
    </div>
  );
}

/** What the agent did, as a terminal would show it. */
export function Activity({ lines, live }: { lines: Line[]; live?: boolean }) {
  if (!lines.length && !live) return null;
  return (
    <div className="aterm rise">
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

export function Finding({ tone, title, children, actions }: { tone: "block" | "warn"; title: ReactNode; children?: ReactNode; actions?: ReactNode }) {
  return (
    <div className={`finding ${tone === "warn" ? "warn" : ""}`}>
      <span style={{ width: 20, flex: "none", color: tone === "warn" ? "#9a6700" : "#c8321a", display: "flex" }}>
        {tone === "warn" ? <IconAlert /> : <IconShield />}
      </span>
      <div style={{ flexGrow: 1, minWidth: 0 }}>
        <strong>{title}</strong>{children && <p>{children}</p>}
        {actions && <div className="finding-actions">{actions}</div>}
      </div>
    </div>
  );
}

/**
 * The agent stopped to ask something. Its answers are one tap each; anything
 * else goes in the box below. Either way the reply resumes the same session.
 */
export function Question({ host, question, options, onAnswer }: {
  host: string | null; question: string; options: string[]; onAnswer: (text: string) => Promise<void>;
}) {
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  return (
    <div className="ask rise" role="group" aria-label="The agent is asking you">
      <div className="ask-head"><AgentLogo host={host} size={24} /><strong>Asking you</strong></div>
      <p className="ask-q">{question}</p>
      {options.length > 0 && (
        <div className="ask-options">
          {options.map((o) => (
            <button key={o} type="button" className="btn-plain" disabled={!!busy} onClick={async () => {
              setBusy(o); setError("");
              try { await onAnswer(o); } catch (e) { setError((e as Error).message); } finally { setBusy(""); }
            }}>{busy === o ? "Sending…" : o}</button>
          ))}
        </div>
      )}
      <span className={`ask-note ${error ? "error" : ""}`}>{error || (options.length ? "Or write your own answer below." : "Answer below.")}</span>
    </div>
  );
}

/**
 * Said before anything is sent, where the eye is: a laptop that is asleep,
 * offline or stopped will not get the message, and silence afterwards is
 * the worst way to find out.
 */
export function LaptopNotice({ machine }: { machine: { name: string; online: boolean; stopped: boolean; lastSeenAt?: string | null } | null | undefined }) {
  if (!machine || (machine.online && !machine.stopped)) return null;
  return (
    <div className="laptop-notice" role="status">
      <span style={{ width: 16, display: "flex", flex: "none" }}><IconAlert /></span>
      <span>
        {machine.stopped
          ? <>Remote is stopped on <strong>{machine.name}</strong>. Nothing sent now will run until you turn it on there: <code>trackline remote enable</code></>
          : <><strong>{machine.name}</strong> is asleep or offline{machine.lastSeenAt ? ` (last seen ${seen(machine.lastSeenAt)})` : ""}. A message waits 3 minutes for it, then is not delivered. Wake the laptop first.</>}
      </span>
    </div>
  );
}

function seen(iso: string): string {
  const m = Math.round((Date.now() - new Date(iso).getTime()) / 60000);
  return m < 1 ? "just now" : m < 60 ? `${m} min ago` : m < 48 * 60 ? `${Math.round(m / 60)} h ago` : `${Math.round(m / 1440)} days ago`;
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
 * Images to send with a message: the button that picks them, and what was
 * picked, each removable. Shrunk and hashed as they are chosen, so sending
 * is not held up by it.
 */
export function useAttachments() {
  const [items, setItems] = useState<Attachment[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const add = async (files: FileList | null) => {
    if (!files?.length) return;
    setError("");
    const room = MAX_ATTACHMENTS - items.length;
    if (files.length > room) setError(`Up to ${MAX_ATTACHMENTS} images a message.`);
    setBusy(true);
    try {
      const ready: Attachment[] = [];
      for (const f of [...files].slice(0, Math.max(0, room))) ready.push(await prepareImage(f));
      setItems((x) => [...x, ...ready].slice(0, MAX_ATTACHMENTS));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
      if (input.current) input.current.value = "";
    }
  };
  const button = (disabled?: boolean) => (
    <>
      <input ref={input} type="file" accept="image/*" multiple hidden onChange={(e) => add(e.target.files)} />
      <button type="button" className="composer-attach" aria-label="Attach images" disabled={disabled || busy || items.length >= MAX_ATTACHMENTS} onClick={() => input.current?.click()}>
        <span style={{ width: 20, display: "flex" }}><IconClip /></span>
      </button>
    </>
  );
  const strip = items.length > 0 || busy ? (
    <div className="attach-strip">
      {items.map((it) => (
        <span key={it.sha256} className="attach-thumb">
          <img src={it.preview} alt="Attached image" />
          <button type="button" aria-label="Remove image" onClick={() => setItems((x) => x.filter((y) => y.sha256 !== it.sha256))}>
            <span style={{ width: 12, display: "flex" }}><IconClose /></span>
          </button>
        </span>
      ))}
      {busy && <span className="attach-thumb skel" />}
    </div>
  ) : null;
  return { items, clear: () => setItems([]), button, strip, error };
}

/**
 * Where you type to the agent. When nothing can be sent, the box says why
 * rather than taking text that goes nowhere.
 */
export function Composer({ placeholder, onSend, blocked, note }: {
  placeholder: string;
  onSend: (text: string, images: Attachment[]) => Promise<void>;
  blocked?: string;
  note?: string;
}) {
  const attach = useAttachments();
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
    // A picture alone is a message too; the agent is told to look at it.
    const t = text.trim() || (attach.items.length ? "See the attached image." : "");
    if (!t || busy || blocked) return;
    setBusy(true); setError("");
    try {
      await onSend(t, attach.items);
      setText("");
      attach.clear();
      requestAnimationFrame(grow);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="composer" onSubmit={(e) => { e.preventDefault(); send(); }}>
      {attach.strip}
      <div className="composer-box">
        {attach.button(!!blocked || busy)}
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
        <button type="submit" className="composer-send" aria-label="Sign and send" disabled={(!text.trim() && !attach.items.length) || busy || !!blocked}>
          <span style={{ width: 18, display: "flex" }}><IconUp /></span>
        </button>
      </div>
      <span className={`composer-note ${error || blocked ? "error" : ""}`}>
        {error || attach.error || blocked || (busy ? "Waiting for your passkey…" : note)}
      </span>
    </form>
  );
}
