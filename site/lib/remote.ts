// Remote prompts: the phone's side. What the phone signs, and how.
//
// A job is signed here, by a passkey, over exactly the bytes that are sent.
// The laptop checks that signature against keys it paired itself, so the
// server that carries the job cannot write one. Every signature is over a
// hash with its purpose in front ("trackline job v1", "trackline pair v1"),
// so a signature made for one purpose can never pass as another; the laptop
// accepts only its own two.

export type Passkey = { id: string; publicKey: string; name: string; transports: string[] };
export type RemoteProject = { id: string; name: string; agents: string[] };
export type Machine = {
  id: string; name: string; online: boolean; lastSeenAt: string | null; stopped: boolean;
  projects: RemoteProject[]; passkeys: { id: string; name: string }[];
};
export type Pairing = { id: string; machineId: string; machine: string; expiresAt: string };
export type JobSummary = {
  id: string; machine: string | null; clientId: string; kind: string; agent: string | null;
  status: string; code: string | null; reason: string | null; createdAt: string; finishedAt: string | null;
};
export type Overview = { machines: Machine[]; pairings: Pairing[]; jobs: JobSummary[] };
export type Job = {
  id: string; kind: string; agent: string | null; status: string; code: string | null; reason: string | null;
  output: string | null; session: string | null; createdAt: string; finishedAt: string | null;
};
export type AgentEvent = { seq: number; kind: string; tool: string | null; text: string | null; createdAt: string };
type Assertion = { authenticatorData: string; clientDataJSON: string; signature: string };

export const AGENT_LABEL: Record<string, string> = { claude: "Claude Code", codex: "Codex" };

/** How long a job may wait for the laptop. The laptop refuses anything older. */
const JOB_LIFE_MS = 3 * 60 * 1000;

const utf8 = (s: string) => new TextEncoder().encode(s);

export function b64u(bytes: ArrayBuffer | Uint8Array): string {
  const b = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  let bin = "";
  for (const x of b) bin += String.fromCharCode(x);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function unb64u(s: string): Uint8Array<ArrayBuffer> {
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((s.length + 3) % 4));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

async function sha256(...parts: Uint8Array[]): Promise<Uint8Array<ArrayBuffer>> {
  const all = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let at = 0;
  for (const p of parts) { all.set(p, at); at += p.length; }
  return new Uint8Array(await crypto.subtle.digest("SHA-256", all));
}

/** Reads a code as a person types it, the same way the laptop does. */
export function normalizeCode(s: string): string {
  return s.toUpperCase().replace(/[-\s]/g, "").replace(/O/g, "0").replace(/[IL]/g, "1");
}

export function passkeysSupported(): boolean {
  return typeof window !== "undefined" && "PublicKeyCredential" in window && !!navigator.credentials;
}

/** Asks one of the person's passkeys to sign, with Face ID, a fingerprint or a PIN. */
async function sign(challenge: Uint8Array<ArrayBuffer>, keys: Passkey[]): Promise<{ key: Passkey; assertion: Assertion }> {
  const cred = (await navigator.credentials.get({
    publicKey: {
      challenge,
      allowCredentials: keys.map((k) => ({ type: "public-key" as const, id: unb64u(k.id), transports: k.transports as AuthenticatorTransport[] })),
      userVerification: "required",
      timeout: 60_000,
    },
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("No passkey signed.");
  const r = cred.response as AuthenticatorAssertionResponse;
  const key = keys.find((k) => k.id === b64u(cred.rawId));
  if (!key) throw new Error("That passkey is not on this account.");
  return {
    key,
    assertion: { authenticatorData: b64u(r.authenticatorData), clientDataJSON: b64u(r.clientDataJSON), signature: b64u(r.signature) },
  };
}

/**
 * Pairs a passkey with a laptop by signing the code on its screen. Only
 * someone who can see that screen can sign the right code, which is how the
 * laptop knows the key is the person's and not one the server slipped in.
 */
export async function signPairing(machineId: string, code: string, keys: Passkey[]) {
  const challenge = await sha256(utf8(`trackline pair v1\n${machineId}\n${normalizeCode(code)}`));
  const { key, assertion } = await sign(challenge, keys);
  return { key: { id: key.id, publicKey: key.publicKey, name: key.name }, ...assertion };
}

function jobId(): string {
  return `job_${b64u(crypto.getRandomValues(new Uint8Array(16)))}`;
}

/** Builds and signs a prompt for one project on one laptop. */
export async function signPrompt(machine: string, project: string, agent: string, text: string, keys: Passkey[]) {
  const now = Date.now();
  const bytes = utf8(JSON.stringify({
    v: 1, id: jobId(), machine, project, kind: "prompt", agent, text, issuedAt: now, expiresAt: now + JOB_LIFE_MS,
  }));
  const { key, assertion } = await sign(await sha256(utf8("trackline job v1\n"), bytes), keys);
  return { machine, envelope: { job: b64u(bytes), key: key.id, ...assertion } };
}

/** What a job's status means, in words. */
export function jobState(j: { status: string; code: string | null; reason: string | null }): string {
  switch (j.status) {
    case "queued": return "Waiting for the laptop";
    case "delivered": return "Working";
    case "done": return "Done";
    case "expired": return "The laptop did not pick it up in time";
    case "cancelled": return "Stopped before it started";
    case "refused": return `The laptop refused it: ${j.reason ?? j.code}`;
    case "failed": return j.code === "stopped" ? "Stopped" : `Failed: ${j.reason ?? "no reason given"}`;
  }
  return j.status;
}

export const active = (status: string) => status === "queued" || status === "delivered";
