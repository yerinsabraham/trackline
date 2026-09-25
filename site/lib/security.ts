"use client";

import { startAuthentication, startRegistration } from "@simplewebauthn/browser";
import { api } from "@/lib/account";

type StepUpError = Error & { status?: number; stepUp?: boolean };

function isStepUp(e: unknown): e is StepUpError {
  return Boolean((e as StepUpError)?.status === 403 && (e as StepUpError)?.stepUp);
}

// A passkey never signs a challenge exactly as the server chose it. Remote
// jobs are signed with the same passkeys, so a server that could pick what a
// step-up signs could pass off a job as a sign-in. Each purpose hashes its own
// prefix; the server checks this one, the laptop only ever accepts "job".
async function purposeChallenge(purpose: string, serverChallenge: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(`trackline ${purpose} v1\n${serverChallenge}`));
  let bin = "";
  for (const b of new Uint8Array(digest)) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export async function stepUp(): Promise<void> {
  try {
    const options = await api<{ challenge: string }>("/auth/step-up/passkey/options", { method: "POST", body: "{}" });
    const challenge = await purposeChallenge("step-up", options.challenge);
    const response = await startAuthentication({ optionsJSON: { ...options, challenge } as never });
    await api("/auth/step-up", { method: "POST", body: JSON.stringify({ method: "passkey", response }) });
    return;
  } catch {
    // Fall back to a code below. Browsers can refuse passkeys for many normal
    // reasons: no passkey enrolled here, not on HTTPS locally, user cancelled.
  }

  const code = window.prompt("Enter your authenticator or recovery code");
  if (!code) throw new Error("Confirmation cancelled.");
  await api("/auth/step-up", {
    method: "POST",
    body: JSON.stringify({ method: code.includes("-") ? "recovery" : "totp", code }),
  });
}

export async function withStepUp<T>(action: () => Promise<T>): Promise<T> {
  try {
    return await action();
  } catch (e) {
    if (!isStepUp(e)) throw e;
    await stepUp();
    return action();
  }
}

export async function addPasskey(name: string): Promise<void> {
  const options = await api<unknown>("/auth/passkeys/options", { method: "POST", body: JSON.stringify({ name }) });
  const response = await startRegistration({ optionsJSON: options as never });
  await api("/auth/passkeys/verify", { method: "POST", body: JSON.stringify({ response }) });
}
