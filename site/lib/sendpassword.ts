// The send password, the browser's half (engine/internal/relay/password.go is
// the laptop's). The key is made here from what was typed and never leaves
// this page; the laptop made the same key when the password was set, and is
// the only other place that can check a job signed with it.

const utf8 = (s: string) => new TextEncoder().encode(s);

/** Slow on purpose: a guess costs this much for anyone who tries one offline. */
export async function passwordKey(machineId: string, salt: string, rounds: number, password: string): Promise<CryptoKey> {
  const base = await crypto.subtle.importKey("raw", utf8(password), "PBKDF2", false, ["deriveBits"]);
  const bits = await crypto.subtle.deriveBits(
    { name: "PBKDF2", hash: "SHA-256", salt: utf8(`trackline send v1\n${machineId}\n${salt}`), iterations: rounds },
    base, 256,
  );
  return crypto.subtle.importKey("raw", bits, { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
}

/** What the laptop checks: the job's challenge, keyed with the slow key. */
export async function passwordMAC(key: CryptoKey, job: Uint8Array): Promise<Uint8Array> {
  const purpose = utf8("trackline job v1\n");
  const all = new Uint8Array(purpose.length + job.length);
  all.set(purpose); all.set(job, purpose.length);
  return challengeMAC(key, new Uint8Array(await crypto.subtle.digest("SHA-256", all)));
}

/** The same, from the job's challenge where only that is at hand. */
export async function challengeMAC(key: CryptoKey, challenge: Uint8Array<ArrayBuffer>): Promise<Uint8Array> {
  return new Uint8Array(await crypto.subtle.sign("HMAC", key, challenge));
}

// Kept for this visit only, in memory, so a person types it once rather than
// for every message. Closing the page forgets it.
const kept = new Map<string, { id: string; key: CryptoKey }>();
export const keptPassword = (machine: string, id: string) => (kept.get(machine)?.id === id ? kept.get(machine)!.key : null);
export const keepPassword = (machine: string, id: string, key: CryptoKey) => { kept.set(machine, { id, key }); };
export const forgetPassword = (machine: string) => { kept.delete(machine); };
