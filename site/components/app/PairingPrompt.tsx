"use client";

import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/account";
import { withStepUp } from "@/lib/security";
import { normalizeCode, type Pairing, type Passkey, passkeysSupported, signPairing } from "@/lib/remote";
import { useApp } from "./AppShell";
import { IconClose, IconFace, IconLaptop, IconShield } from "./icons";

// A laptop asking to pair, wherever you are in the app: a notice, then the
// code boxes. The phone is also sent a push notification by the server, and
// tapping it opens the app, which shows this.

const LEN = 8;

export default function PairingPrompt() {
  const { remote, refresh } = useApp();
  const [dismissed, setDismissed] = useState<string[]>([]);
  const [open, setOpen] = useState<Pairing | null>(null);
  const waiting = remote?.pairings.find((p) => !dismissed.includes(p.id)) ?? null;

  useEffect(() => {
    if (waiting && !open) setOpen(waiting);
  }, [waiting, open]);

  if (!open) return null;
  const close = () => { setDismissed((d) => [...d, open.id]); setOpen(null); refresh(); };
  return (
    <>
      <div role="status" className="toast">
        <span className="agent-logo" style={{ width: 34, height: 34, background: "#fbe3d9", color: "#9e2a0a" }}><IconLaptop /></span>
        <span style={{ display: "grid", gap: 2 }}>
          <strong style={{ fontSize: 14 }}>Pairing request</strong>
          <span style={{ fontSize: 13, color: "var(--ink-2)" }}>{open.machine} wants to take tasks from you.</span>
        </span>
      </div>
      <PairSheet pairing={open} onClose={close} />
    </>
  );
}

function PairSheet({ pairing, onClose }: { pairing: Pairing; onClose: () => void }) {
  const [chars, setChars] = useState<string[]>(Array(LEN).fill(""));
  const [keys, setKeys] = useState<Passkey[] | null>(null);
  const [state, setState] = useState<"idle" | "signing" | "checking" | "paired" | "refused">("idle");
  const [note, setNote] = useState("");
  const boxes = useRef<(HTMLInputElement | null)[]>([]);

  useEffect(() => {
    api<{ passkeys: Passkey[] }>("/app/remote/passkeys").then((r) => setKeys(r.passkeys)).catch(() => setKeys([]));
    boxes.current[0]?.focus();
  }, []);

  const put = (at: number, raw: string) => {
    const typed = normalizeCode(raw).replace(/[^0-9A-Z]/g, "");
    if (!typed) { setChars((c) => c.map((x, i) => (i === at ? "" : x))); return; }
    setChars((c) => {
      const next = [...c];
      for (let k = 0; k < typed.length && at + k < LEN; k++) next[at + k] = typed[k];
      return next;
    });
    boxes.current[Math.min(at + typed.length, LEN - 1)]?.focus();
  };

  const code = chars.join("");
  const pair = async () => {
    if (!keys?.length) return;
    setNote("");
    try {
      setState("signing");
      // Signed once: if the server asks to confirm it is you first, only the
      // sending is repeated.
      const answer = await signPairing(pairing.machineId, code, keys);
      await withStepUp(() => api(`/app/remote/pairings/${pairing.id}/answer`, { method: "POST", body: JSON.stringify(answer) }));
      setState("checking");
      for (let i = 0; i < 40; i++) {
        await new Promise((r) => setTimeout(r, 750));
        const s = await api<{ status: string; reason: string | null }>(`/app/remote/pairings/${pairing.id}`);
        if (s.status === "trusted") { setState("paired"); return; }
        if (s.status === "refused") {
          setState("refused");
          setNote(`The laptop refused it: ${s.reason ?? "the code did not match"}. Run trackline remote enable again for a new code.`);
          return;
        }
      }
      setState("idle");
      setNote("The laptop has not answered. Check it is still waiting.");
    } catch (e) {
      setState("idle");
      setNote((e as Error).message);
    }
  };

  const done = state === "paired" || state === "refused";
  return (
    <>
      <div className="scrim" onClick={onClose} />
      <div role="dialog" aria-modal="true" aria-labelledby="pair-title" className="sheet">
        <span className="sheet-grab" aria-hidden="true" />
        <div style={{ display: "flex", gap: 14, alignItems: "flex-start" }}>
          <span className="agent-logo" style={{ width: 46, height: 46, background: "#f1f1ef" }}><IconLaptop /></span>
          <div style={{ flexGrow: 1 }}>
            <h2 id="pair-title" style={{ margin: 0, fontSize: 21, letterSpacing: "-0.02em" }}>Pair {pairing.machine}</h2>
            <p style={{ margin: "4px 0 0", color: "var(--ink-2)", fontSize: 14.5 }}>
              {state === "paired" ? "Paired. Tasks you send now run on it." : "Enter the code your laptop shows."}
            </p>
          </div>
          <button className="app-icon-btn" aria-label="Close" onClick={onClose}><IconClose /></button>
        </div>

        {!done && (
          <div className="code-boxes" role="group" aria-label="Pairing code">
            {chars.map((c, i) => (
              <span key={i} style={{ display: "contents" }}>
                {i === LEN / 2 && <span className="dash" aria-hidden="true" />}
                <input
                  ref={(el) => { boxes.current[i] = el; }}
                  aria-label={`Character ${i + 1}`}
                  className={c ? "filled" : ""}
                  value={c}
                  inputMode="text"
                  autoCapitalize="characters"
                  autoComplete="one-time-code"
                  spellCheck={false}
                  disabled={state !== "idle"}
                  onChange={(e) => put(i, e.target.value.slice(c ? 1 : 0) || e.target.value)}
                  onPaste={(e) => { e.preventDefault(); put(i, e.clipboardData.getData("text")); }}
                  onKeyDown={(e) => { if (e.key === "Backspace" && !c && i > 0) boxes.current[i - 1]?.focus(); }}
                />
              </span>
            ))}
          </div>
        )}

        {state === "idle" && (
          <p style={{ margin: 0, display: "flex", gap: 10, padding: "12px 14px", background: "#f5f5f3", borderRadius: 12, fontSize: 13, color: "var(--ink-2)", lineHeight: 1.45 }}>
            <span style={{ width: 18, flex: "none" }}><IconShield /></span>
            Only pair a laptop you are in front of. The code proves this screen and that laptop belong to the same person.
          </p>
        )}
        {note && <p className="composer-note error" style={{ textAlign: "left" }}>{note}</p>}
        {keys?.length === 0 && <p className="composer-note error" style={{ textAlign: "left" }}>Add a passkey on your account page first: pairing is signed with it.</p>}
        {!passkeysSupported() && <p className="composer-note error" style={{ textAlign: "left" }}>This browser cannot use passkeys.</p>}

        <div style={{ display: "grid", gap: 8 }}>
          {done ? (
            <button className="btn-act" style={{ height: 48 }} onClick={onClose}>Done</button>
          ) : (
            <button className="btn-act" style={{ height: 48 }} disabled={code.length !== LEN || !keys?.length || state !== "idle"} onClick={pair}>
              <span style={{ width: 18, display: "flex" }}><IconFace /></span>
              {state === "signing" ? "Waiting for your passkey…" : state === "checking" ? "The laptop is checking…" : "Pair with your passkey"}
            </button>
          )}
          {!done && <a className="btn-danger-text" style={{ textAlign: "center" }} href="/account">This wasn’t me</a>}
        </div>
      </div>
    </>
  );
}
