"use client";

import { useEffect, useRef, useState } from "react";
import { api, type User } from "@/lib/account";
import { accountConfirmed, confirmWith } from "@/lib/confirm";
import { googleButton } from "@/lib/google";
import { ACCOUNT_SIGNED, b64u, type Machine, type Passkey, passkeySigned, type Signed } from "@/lib/remote";
import { passkeyStepUp, setIdentitySheet } from "@/lib/security";
import { challengeMAC, keepPassword, passwordKey } from "@/lib/sendpassword";
import { AddPasskey } from "./Chat";
import { Cmd } from "./FirstRun";

// Every way to confirm it is you, in one place, offered by what works here.
// A phone with no passkey and no authenticator app used to meet a QR code, then
// "no passkey found", then a loop (2026-11-03). Now it can sign in with Google
// again or take a code from its email, and the computer that runs the message
// checks what it can itself: Google's token, a passkey, the send password.

type JobAsk = { machine?: Machine; keys: Passkey[]; missed: boolean; challenge: Uint8Array<ArrayBuffer>; done: (s: Signed | null) => void };
type IdentityAsk = { done: (ok: boolean) => void };

let showJob: ((a: JobAsk | null) => void) | null = null;
let showIdentity: ((a: IdentityAsk | null) => void) | null = null;

/** Opens the sheet for one message, and resolves with how it was confirmed. */
export function chooseSigner(machine: Machine | undefined, keys: Passkey[], missed: boolean, challenge: Uint8Array<ArrayBuffer>): Promise<Signed> {
  return new Promise((resolve, reject) => {
    if (!showJob) return reject(new Error("Nothing was sent."));
    showJob({ machine, keys, missed, challenge, done: (s) => { showJob?.(null); if (s) resolve(s); else reject(new Error("Nothing was sent.")); } });
  });
}

/** Confirms it is you to the account, for a change that needs it. */
function confirmIdentity(): Promise<void> {
  return new Promise((resolve, reject) => {
    if (!showIdentity) return reject(new Error("Confirmation cancelled."));
    showIdentity({ done: (ok) => { showIdentity?.(null); if (ok) resolve(); else reject(new Error("Confirmation cancelled.")); } });
  });
}

export default function ConfirmSheet() {
  const [job, setJob] = useState<JobAsk | null>(null);
  const [identity, setIdentity] = useState<IdentityAsk | null>(null);
  useEffect(() => {
    showJob = setJob;
    showIdentity = setIdentity;
    setIdentitySheet(confirmIdentity);
    return () => { showJob = null; showIdentity = null; setIdentitySheet(null); };
  }, []);
  return (
    <>
      {job && <JobSheet ask={job} />}
      {identity && <IdentitySheet ask={identity} />}
    </>
  );
}

function Sheet({ label, note, onClose, children, top }: { label: string; note: string; onClose: () => void; children: React.ReactNode; top?: boolean }) {
  return (
    <div className={`sheet-back ${top ? "sheet-top" : ""}`} onClick={onClose}>
      <div className="sheet confirm-sheet" role="dialog" aria-label={label} onClick={(e) => e.stopPropagation()}>
        <span className="sheet-grab" aria-hidden />
        <h2 className="confirm-title">{label}</h2>
        <p className="sheet-note">{note}</p>
        {children}
        <button type="button" className="sheet-item" onClick={onClose}>Cancel</button>
      </div>
    </div>
  );
}

function JobSheet({ ask }: { ask: JobAsk }) {
  const [error, setError] = useState("");
  const [setup, setSetup] = useState(false);
  const [added, setAdded] = useState(false);
  const m = ask.machine;
  const where = m?.name ?? "your computer";
  const nonce = b64u(ask.challenge);
  const fail = (e: unknown) => setError((e as Error).message);
  const none = !m?.google && !m?.emailCodes && !m?.password;

  return (
    <Sheet
      label="Confirm it's you" onClose={() => ask.done(null)}
      note={ask.missed ? "Your passkey wasn't used on this device." : `${where} checks this before it runs anything.`}
    >
      {m?.google && <GoogleOption nonce={nonce} onToken={(t) => ask.done({ key: "google", assertion: { authenticatorData: "", clientDataJSON: "", signature: t } })} onError={fail} />}
      {m?.emailCodes && <EmailOption onConfirmed={() => { accountConfirmed(); ask.done(ACCOUNT_SIGNED); }} onError={fail} />}
      {ask.keys.length > 0 && (
        <button type="button" className={m?.google || m?.emailCodes ? "btn-plain sheet-wide" : "btn-act sheet-wide"} onClick={async () => {
          setError("");
          try { const s = await passkeySigned(ask.challenge, ask.keys); confirmWith("passkey"); ask.done(s); } catch (e) { fail(e); }
        }}>Use a passkey</button>
      )}
      {m?.password && <PasswordOption machine={m} where={where} onKey={async (key) => ask.done({ key: m.password!.id, assertion: { authenticatorData: "", clientDataJSON: "", signature: b64u(await challengeMAC(key, ask.challenge)) } })} />}
      {none && (
        <div className="confirm-step">
          <p className="sheet-note">To confirm without a passkey, run this once on {where}. It lets you sign in with Google, or get a code by email, from here.</p>
          <Cmd text="trackline remote enable" />
        </div>
      )}
      {error && <p className="composer-note error" role="alert">{error}</p>}
      {!setup ? (
        <button type="button" className="sheet-item" onClick={() => setSetup(true)}>Set up this device instead</button>
      ) : (
        <div className="confirm-step">
          {!added ? <><p className="sheet-note">1. Add a passkey on this device.</p><AddPasskey onAdded={() => setAdded(true)} /></>
            : <p className="sheet-note">1. Passkey added.</p>}
          <p className="sheet-note">2. On {where}, run this, and enter the code it shows here.</p>
          <Cmd text="trackline remote enable --pair" />
        </div>
      )}
    </Sheet>
  );
}

function IdentitySheet({ ask }: { ask: IdentityAsk }) {
  const [user, setUser] = useState<User | null>(null);
  const [nonce, setNonce] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const fail = (e: unknown) => setError((e as Error).message);
  useEffect(() => {
    api<{ user: User }>("/auth/me").then((r) => {
      setUser(r.user);
      if (r.user.signInMethods.includes("google")) api<{ nonce: string }>("/auth/step-up/google/options", { method: "POST", body: "{}" }).then((x) => setNonce(x.nonce)).catch(fail);
    }).catch(fail);
  }, []);
  const confirmed = () => { accountConfirmed(); ask.done(true); };

  return (
    <Sheet label="Confirm it's you" note="This change needs you to confirm it's you." onClose={() => ask.done(false)} top>
      {nonce && <GoogleOption nonce={nonce} onError={fail} onToken={async (credential) => {
        try { await api("/auth/step-up", { method: "POST", body: JSON.stringify({ method: "google", credential }) }); confirmed(); } catch (e) { fail(e); }
      }} />}
      {user?.email && <EmailOption onConfirmed={confirmed} onError={fail} />}
      <button type="button" className="btn-plain sheet-wide" onClick={async () => {
        setError("");
        try { await passkeyStepUp(); confirmed(); } catch { setError("Your passkey wasn't used. Try another way."); }
      }}>Use a passkey</button>
      <form className="confirm-password" onSubmit={async (e) => {
        e.preventDefault(); setError("");
        try { await api("/auth/step-up", { method: "POST", body: JSON.stringify({ method: code.includes("-") ? "recovery" : "totp", code: code.trim() }) }); confirmed(); } catch (err) { fail(err); }
      }}>
        <label className="sheet-label" htmlFor="auth-code">Or a code from your authenticator app, or a recovery code</label>
        <input id="auth-code" className="sheet-input" inputMode="text" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />
        <button type="submit" className="btn-plain sheet-wide" disabled={code.trim().length < 6}>Use this code</button>
      </form>
      {error && <p className="composer-note error" role="alert">{error}</p>}
    </Sheet>
  );
}

function GoogleOption({ nonce, onToken, onError }: { nonce: string; onToken: (token: string) => void; onError: (e: unknown) => void }) {
  const el = useRef<HTMLDivElement>(null);
  useEffect(() => { if (el.current) googleButton(el.current, nonce, onToken).catch(onError); }, [nonce]); // eslint-disable-line react-hooks/exhaustive-deps
  return <div className="confirm-google" ref={el} aria-label="Continue with Google" />;
}

function EmailOption({ onConfirmed, onError }: { onConfirmed: () => void; onError: (e: unknown) => void }) {
  const [to, setTo] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  if (!to) return (
    <button type="button" className="btn-plain sheet-wide" disabled={busy} onClick={async () => {
      setBusy(true);
      try { setTo((await api<{ to: string }>("/auth/step-up/email/send", { method: "POST", body: "{}" })).to); } catch (e) { onError(e); } finally { setBusy(false); }
    }}>{busy ? "Sending…" : "Email me a code"}</button>
  );
  return (
    <form className="confirm-password" onSubmit={async (e) => {
      e.preventDefault(); setBusy(true);
      try { await api("/auth/step-up", { method: "POST", body: JSON.stringify({ method: "email", code: code.trim() }) }); onConfirmed(); } catch (err) { onError(err); setBusy(false); }
    }}>
      <label className="sheet-label" htmlFor="email-code">Code sent to {to}</label>
      <input id="email-code" className="sheet-input" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))} />
      <button type="submit" className="btn-act sheet-wide" disabled={code.length !== 6 || busy}>{busy ? "Confirming…" : "Confirm"}</button>
    </form>
  );
}

function PasswordOption({ machine, where, onKey }: { machine: Machine; where: string; onKey: (key: CryptoKey) => Promise<void> }) {
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const pw = machine.password!;
  return (
    <form className="confirm-password" onSubmit={async (e) => {
      e.preventDefault();
      if (!password) return;
      setBusy(true);
      // The key is made here; nothing typed is sent. A wrong password is only
      // known when the computer refuses the message.
      const key = await passwordKey(machine.id, pw.salt, pw.rounds, password);
      keepPassword(machine.id, pw.id, key);
      await onKey(key);
    }}>
      <label className="sheet-label" htmlFor="send-password">Or your send password for {where}</label>
      <input id="send-password" className="sheet-input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      <button type="submit" className="btn-plain sheet-wide" disabled={!password || busy}>{busy ? "Confirming…" : "Send with password"}</button>
    </form>
  );
}
