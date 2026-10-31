"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { useStickToBottom } from "use-stick-to-bottom";
import { api, readDraft, saveDraft } from "@/lib/account";
import { addPasskey, withStepUp } from "@/lib/security";
import ReplyText, { LinkedText } from "@/components/ReplyText";
import CopyButton from "@/components/CopyButton";
import AgentLogo from "./AgentLogo";
import { Cmd } from "./FirstRun";
import { IconAlert, IconClip, IconClose, IconDown, IconShield, IconTerminal, IconUp } from "./icons";
import { type Attachment, MAX_ATTACHMENTS, type Passkey, prepareImage } from "@/lib/remote";

// The pieces a conversation with an agent is drawn from, whichever way it
// arrived: from trackline's hook in the terminal, or from a task sent here.

/**
 * A conversation opens on its newest message and follows what arrives, until
 * you scroll up to read; scrolling back down picks it up again. Instant, so a
 * long chat is never seen sliding past on the way to the bottom.
 */
export function useChatScroll() {
  useKeyboard();
  return useStickToBottom({ initial: "instant", resize: "instant" });
}

/**
 * The height left above the keyboard. iOS ignores interactive-widget and leaves
 * dvh alone, and in a Home Screen app it can also scroll the page up behind the
 * keyboard, so the chat takes its height from the visual viewport itself.
 */
function useKeyboard() {
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const root = document.documentElement;
    const fit = () => {
      root.style.setProperty("--vvh", `${vv.height}px`);
      if (window.scrollY) window.scrollTo(0, 0);
    };
    fit();
    vv.addEventListener("resize", fit);
    vv.addEventListener("scroll", fit);
    return () => {
      vv.removeEventListener("resize", fit);
      vv.removeEventListener("scroll", fit);
      root.style.removeProperty("--vvh");
    };
  }, []);
}

/** Back to the newest message, when you have scrolled away from it. */
export function JumpToNewest({ onClick }: { onClick: () => void }) {
  return <button type="button" className="jump-newest" aria-label="Jump to the newest message" onClick={onClick}><IconDown size={18} /></button>;
}

/**
 * This account's passkeys, ready before you tap send: on an iPhone a network
 * call between the tap and Face ID can cost the tap its right to ask for it.
 */
export function usePasskeys() {
  const keys = useRef<Passkey[] | null>(null);
  const [missing, setMissing] = useState(false);
  const load = async () => {
    keys.current = (await api<{ passkeys: Passkey[] }>("/app/remote/passkeys")).passkeys;
    setMissing(keys.current.length === 0);
    return keys.current;
  };
  useEffect(() => { load().catch(() => {}); }, []);
  const get = async () => {
    const k = keys.current?.length ? keys.current : await load();
    if (!k.length) throw new Error("Add a passkey to send from here.");
    return k;
  };
  return { get, missing, reload: load };
}

/** The first passkey, added where it is needed rather than on another page. */
export function AddPasskey({ onAdded }: { onAdded: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <>
      <button type="button" className="btn-act" disabled={busy} onClick={async () => {
        setBusy(true); setError("");
        try {
          await withStepUp(() => addPasskey(/iPhone|Android/i.test(navigator.userAgent) ? "Phone" : "Laptop"));
          onAdded();
        } catch (e) {
          setError((e as Error).message);
        } finally {
          setBusy(false);
        }
      }}>{busy ? "Waiting for your passkey…" : "Add a passkey"}</button>
      {error && <span className="composer-note error">{error}</span>}
    </>
  );
}

export function PasskeyNeeded({ onAdded }: { onAdded: () => void }) {
  return <div className="passkey-needed"><span>Add a passkey to send from this device.</span><AddPasskey onAdded={onAdded} /></div>;
}

const RESUME: Record<string, string> = { claude: "claude --resume", codex: "codex resume", cursor: "cursor-agent --resume" };

/**
 * The VS Code panels of Claude Code and Codex list only sessions started
 * there, by design, so a conversation started here is picked up on the laptop
 * from a terminal.
 */
export function ResumeHere({ agent, session }: { agent: string | null; session: string }) {
  if (!agent || !RESUME[agent]) return null;
  return (
    <div className="resume-here">
      <span>Carry on at your laptop, in a terminal in this project:</span>
      <Cmd text={`${RESUME[agent]} ${session}`} />
    </div>
  );
}

export type Line = { key: string; time?: string; verb: string; text: string; tone?: "ok" | "warn" | "block" | "dim"; why?: string };

export function MyMessage({ text, note, images, actions }: { text: string; note?: string; images?: string[]; actions?: ReactNode }) {
  return (
    <div className="bubble-me rise">
      {images && images.length > 0 && (
        <div className="bubble-images">{images.map((u) => <img key={u} src={u} alt="Attached image" />)}</div>
      )}
      <div><LinkedText text={text} /></div>
      {note && <span>{note}</span>}
      {actions && <div className="bubble-actions">{actions}</div>}
    </div>
  );
}

const SAID: [RegExp, string, string][] = [[/^Edit/, "Edited", "file"], [/^Write/, "Wrote", "file"], [/^Delete/, "Deleted", "file"], [/^Read/, "Read", "file"], [/^Run/, "Ran", "command"]];

/** A turn's work in a line, for people reading the conversation and not the log. */
export function summarise(lines: Line[]): string {
  const counts = new Map<string, { n: number; noun: string }>();
  let other = 0;
  for (const l of lines) {
    const hit = SAID.find(([re]) => re.test(l.verb));
    if (!hit) { other++; continue; }
    const c = counts.get(hit[1]) ?? { n: 0, noun: hit[2] };
    c.n++;
    counts.set(hit[1], c);
  }
  const parts = [...counts].map(([verb, { n, noun }]) => `${verb} ${n} ${noun}${n === 1 ? "" : "s"}`);
  if (other) parts.push(`${other} other step${other === 1 ? "" : "s"}`);
  return parts.join(", ");
}

/** What the agent did: one line, opening to the steps, as a terminal showed them. */
export function Activity({ lines, live }: { lines: Line[]; live?: boolean }) {
  if (!lines.length && !live) return null;
  return (
    <details className="aterm rise" open={live}>
      <summary className="term-head">
        <IconTerminal size={14} />
        <span>{summarise(lines) || "Working"}</span>
        {live && <span className="live">live</span>}
      </summary>
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
    </details>
  );
}

export function Finding({ tone, title, children, actions }: { tone: "block" | "warn"; title: ReactNode; children?: ReactNode; actions?: ReactNode }) {
  return (
    <div className={`finding ${tone === "warn" ? "warn" : ""}`}>
      <span style={{ flex: "none", color: tone === "warn" ? "var(--amber-ink)" : "var(--red-ink)", display: "flex" }}>
        {tone === "warn" ? <IconAlert size={20} /> : <IconShield size={20} />}
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
      <IconAlert size={16} />
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
export function AgentReply({ host, text, cut }: { host: string | null; text: string; cut?: boolean }) {
  return (
    <div className="bubble-agent">
      <AgentLogo host={host} size={30} />
      <div className="body">
        <ReplyText text={text} />
        {cut && <p className="reply-cut">Cut short. The rest is on your laptop.</p>}
        <div className="reply-tools"><CopyButton text={text} /></div>
      </div>
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
        <IconClip size={20} />
      </button>
    </>
  );
  const strip = items.length > 0 || busy ? (
    <div className="attach-strip">
      {items.map((it) => (
        <span key={it.sha256} className="attach-thumb">
          <img src={it.preview} alt="Attached image" />
          <button type="button" aria-label="Remove image" onClick={() => setItems((x) => x.filter((y) => y.sha256 !== it.sha256))}>
            <IconClose size={12} />
          </button>
        </span>
      ))}
      {busy && <span className="attach-thumb skel" />}
    </div>
  ) : null;
  return { items, clear: () => setItems([]), button, strip, error };
}

/**
 * Where you type to the agent. You can always type: a thought is worth keeping
 * whatever the laptop is doing, and the draft waits here. When it cannot be
 * sent yet, the line underneath says what would let it.
 */
export function Composer({ chat, placeholder, onSend, blocked, note }: {
  /** Which conversation this is, so a half-written message waits for you there. */
  chat?: string;
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

  useEffect(() => {
    if (!chat) return;
    setText(readDraft(chat));
    requestAnimationFrame(grow);
  }, [chat]);

  const send = async () => {
    // A picture alone is a message too; the agent is told to look at it.
    const t = text.trim() || (attach.items.length ? "See the attached image." : "");
    if (!t || busy || blocked) return;
    setBusy(true); setError("");
    try {
      await onSend(t, attach.items);
      setText("");
      if (chat) saveDraft(chat, "");
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
        {attach.button(busy)}
        <label htmlFor="composer" style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)" }}>{placeholder}</label>
        <textarea
          id="composer" ref={box} rows={1} value={text} placeholder={placeholder}
          // Read-only rather than disabled while sending: a disabled box loses
          // focus, and the phone's keyboard goes with it.
          readOnly={busy}
          onChange={(e) => { setText(e.target.value); if (chat) saveDraft(chat, e.target.value); grow(); }}
          onKeyDown={(e) => {
            // Enter sends on a keyboard with one; a phone keeps Enter for new lines.
            if (e.key === "Enter" && !e.shiftKey && window.matchMedia("(min-width: 900px)").matches) { e.preventDefault(); send(); }
          }}
        />
        <button type="submit" className={`composer-send ${busy ? "sending" : ""}`} aria-label={busy ? "Sending" : "Sign and send"} disabled={(!text.trim() && !attach.items.length) || busy || !!blocked}>
          {busy ? <span className="composer-spinner" aria-hidden="true" /> : <IconUp size={20} />}
        </button>
      </div>
      <span className={`composer-note ${error ? "error" : ""}`}>
        {error || attach.error || blocked || (busy ? "Waiting for your passkey…" : note)}
      </span>
    </form>
  );
}
