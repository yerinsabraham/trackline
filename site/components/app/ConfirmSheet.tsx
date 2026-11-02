"use client";

import { useEffect, useState } from "react";
import type { Machine, Passkey, Signer } from "@/lib/remote";
import { keepPassword, passwordKey } from "@/lib/sendpassword";
import { AddPasskey } from "./Chat";
import { Cmd } from "./FirstRun";

// Every way to confirm a message, in one place. A browser asked for a passkey
// this device does not hold offers a QR code, then "no passkey found", and no
// way back (found on a Mac, 2026-11-02); a person with no passkey at all, or a
// device without Face ID, had nothing to try. So when a passkey is not going
// to work, this asks first, and names each way that can.

type Ask = { machine?: Machine; keys: Passkey[]; missed: boolean; done: (s: Signer | null) => void };

let show: ((a: Ask | null) => void) | null = null;

/** Opens the sheet, and resolves with how the person chose to sign. */
export function chooseSigner(machine: Machine | undefined, keys: Passkey[], missed: boolean): Promise<Signer> {
  return new Promise((resolve, reject) => {
    if (!show) return reject(new Error("Nothing was sent."));
    show({ machine, keys, missed, done: (s) => { show?.(null); if (s) resolve(s); else reject(new Error("Nothing was sent.")); } });
  });
}

export default function ConfirmSheet() {
  const [ask, setAsk] = useState<Ask | null>(null);
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [setup, setSetup] = useState(false);
  const [added, setAdded] = useState(false);
  useEffect(() => { show = setAsk; return () => { show = null; }; }, []);
  useEffect(() => { if (!ask) { setPassword(""); setBusy(false); setSetup(false); setAdded(false); } }, [ask]);
  if (!ask) return null;

  const pw = ask.machine?.password ?? null;
  const where = ask.machine?.name ?? "your computer";
  const usePassword = async () => {
    if (!pw || !ask.machine || !password) return;
    setBusy(true);
    // The key is made here; nothing typed is sent. A wrong password is only
    // known when the computer refuses the message.
    const key = await passwordKey(ask.machine.id, pw.salt, pw.rounds, password);
    keepPassword(ask.machine.id, pw.id, key);
    ask.done({ password: pw.id, key });
  };

  return (
    <div className="sheet-back" onClick={() => ask.done(null)}>
      <div className="sheet confirm-sheet" role="dialog" aria-label="Confirm it's you" onClick={(e) => e.stopPropagation()}>
        <span className="sheet-grab" aria-hidden />
        <h2 className="confirm-title">Confirm it&apos;s you</h2>
        <p className="sheet-note">{ask.missed ? "Your passkey wasn't used on this device." : `Your computer checks this before it runs anything.`}</p>

        {ask.keys.length > 0 && (
          <button type="button" className="btn-act sheet-wide" onClick={() => ask.done(ask.keys)}>Use a passkey</button>
        )}
        {ask.keys.length > 0 && <p className="sheet-note">Face ID, Touch ID or your device PIN. On a computer, you can scan a code with the phone that has it.</p>}

        {pw ? (
          <form className="confirm-password" onSubmit={(e) => { e.preventDefault(); usePassword(); }}>
            <label className="sheet-label" htmlFor="send-password">Or your send password for {where}</label>
            <input id="send-password" className="sheet-input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
            <button type="submit" className="btn-plain sheet-wide" disabled={!password || busy}>{busy ? "Confirming…" : "Send with password"}</button>
          </form>
        ) : (
          <div className="confirm-step">
            <p className="sheet-note">No passkey? Set a send password, on {where}:</p>
            <Cmd text="trackline remote password" />
          </div>
        )}

        {!setup ? (
          <button type="button" className="sheet-item" onClick={() => setSetup(true)}>Set up this device instead</button>
        ) : (
          <div className="confirm-step">
            {!added ? (
              <>
                <p className="sheet-note">1. Add a passkey on this device.</p>
                <AddPasskey onAdded={() => setAdded(true)} />
              </>
            ) : <p className="sheet-note">1. Passkey added.</p>}
            <p className="sheet-note">2. On {where}, run this, and enter the code it shows here.</p>
            <Cmd text="trackline remote enable --pair" />
          </div>
        )}
        <button type="button" className="sheet-item" onClick={() => ask.done(null)}>Cancel</button>
      </div>
    </div>
  );
}
