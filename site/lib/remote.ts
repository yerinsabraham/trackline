// Remote prompts: the phone's side. What the phone signs, and how.
//
// A job is signed here, by a passkey, over exactly the bytes that are sent.
// The laptop checks that signature against keys it paired itself, so the
// server that carries the job cannot write one. Every signature is over a
// hash with its purpose in front ("trackline job v1", "trackline pair v2"),
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
  /** It stopped to ask the person something, and nothing has answered it yet. */
  asking?: boolean;
  /** The agent's session it continues, when it continues one. */
  session?: string | null;
};
export type Overview = { machines: Machine[]; pairings: Pairing[]; jobs: JobSummary[] };
export type Job = {
  id: string; kind: string; agent: string | null; status: string; code: string | null; reason: string | null;
  output: string | null; session: string | null; createdAt: string; finishedAt: string | null;
  machine?: string; project?: string; dashboardSession?: string | null;
  /** What the agent asked at the end of its turn, with the answers it offered. */
  asked?: { question: string; options: string[] } | null;
};

/** Where a reply goes: the laptop, project and agent, and the session to continue. */
export type Continuation = {
  machine: { id: string; name: string; online: boolean; stopped: boolean };
  project: string; agent: string; session: string;
};
export type AgentEvent = { seq: number; kind: string; tool: string | null; text: string | null; createdAt: string };
type Assertion = { authenticatorData: string; clientDataJSON: string; signature: string };

export const AGENT_LABEL: Record<string, string> = { claude: "Claude Code", codex: "Codex" };

export function protectedMacArea(text: string): string | null {
  const t = text.toLowerCase();
  if (/\bicloud drive\b|mobile documents|clouddocs/.test(t)) return "iCloud Drive";
  if (/(^|[^a-z])downloads?($|[^a-z])/.test(t)) return "Downloads";
  if (/(^|[^a-z])desktop($|[^a-z])/.test(t)) return "Desktop";
  if (/(^|[^a-z])documents?($|[^a-z])/.test(t)) return "Documents";
  if (/\/volumes\//.test(t) || /\bexternal (drive|disk|volume)\b|\bnetwork (drive|volume|share)\b/.test(t)) return "an external or network volume";
  return null;
}

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

const PAIR = "trackline pair v2\n";

/**
 * The code, made slow before it is signed. The answer passes through the
 * server, and a plain hash of an 8-character code could be searched in
 * minutes; this takes years (engine/internal/relay/webauthn.go, PairRounds).
 */
export async function pairChallenge(machineId: string, code: string): Promise<Uint8Array<ArrayBuffer>> {
  const base = await crypto.subtle.importKey("raw", utf8(normalizeCode(code)), "PBKDF2", false, ["deriveBits"]);
  const slow = await crypto.subtle.deriveBits({ name: "PBKDF2", hash: "SHA-256", salt: utf8(PAIR + machineId), iterations: 600_000 }, base, 256);
  return sha256(utf8(PAIR), new Uint8Array(slow));
}

/**
 * Pairs a passkey with a laptop by signing the code on its screen. Only
 * someone who can see that screen can sign the right code, which is how the
 * laptop knows the key is the person's and not one the server slipped in.
 */
export async function signPairing(machineId: string, code: string, keys: Passkey[]) {
  const challenge = await pairChallenge(machineId, code);
  const { key, assertion } = await sign(challenge, keys);
  return { key: { id: key.id, publicKey: key.publicKey, name: key.name }, ...assertion };
}

function jobId(): string {
  return `job_${b64u(crypto.getRandomValues(new Uint8Array(16)))}`;
}

/**
 * Builds and signs a prompt for one project on one laptop. With a session,
 * the agent continues that conversation instead of starting one.
 */
export async function signPrompt(machine: string, project: string, agent: string, text: string, keys: Passkey[], session?: string, images: Attachment[] = []) {
  const now = Date.now();
  const bytes = utf8(JSON.stringify({
    v: 1, id: jobId(), machine, project, kind: "prompt", agent, text, ...(session ? { session } : {}),
    // The hashes are signed; the bytes travel beside the job, and the laptop
    // takes only images that match.
    ...(images.length ? { images: images.map((i) => ({ type: i.type, size: i.size, sha256: i.sha256 })) } : {}),
    issuedAt: now, expiresAt: now + JOB_LIFE_MS,
  }));
  const { key, assertion } = await sign(await sha256(utf8("trackline job v1\n"), bytes), keys);
  return {
    machine, envelope: { job: b64u(bytes), key: key.id, ...assertion },
    ...(images.length ? { images: images.map((i) => ({ sha256: i.sha256, data: i.data })) } : {}),
  };
}

/** An image ready to send: shrunk, hashed, and a preview for the page. */
export type Attachment = { type: string; size: number; sha256: string; data: string; preview: string };

export const MAX_ATTACHMENTS = 4;
const MAX_SIDE = 2048;
const MAX_BYTES = 3 * 1024 * 1024;

/**
 * A photo as it will be sent: at most 2048 pixels on its longest side, as
 * JPEG, which every agent reads and which keeps a phone photo to a few
 * hundred kilobytes. Screenshots stay legible at this size.
 */
export async function prepareImage(file: File): Promise<Attachment> {
  if (!file.type.startsWith("image/")) throw new Error(`${file.name} is not an image.`);
  const bitmap = await createImageBitmap(file).catch(() => { throw new Error(`${file.name} could not be read as an image.`); });
  const scale = Math.min(1, MAX_SIDE / Math.max(bitmap.width, bitmap.height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.round(bitmap.width * scale);
  canvas.height = Math.round(bitmap.height * scale);
  const ctx = canvas.getContext("2d")!;
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
  let blob: Blob | null = null;
  for (const q of [0.88, 0.75, 0.6]) {
    blob = await new Promise<Blob | null>((r) => canvas.toBlob(r, "image/jpeg", q));
    if (blob && blob.size <= MAX_BYTES) break;
  }
  if (!blob || blob.size > MAX_BYTES) throw new Error(`${file.name} is too large to send, even shrunk.`);
  const bytes = new Uint8Array(await blob.arrayBuffer());
  const hash = new Uint8Array(await crypto.subtle.digest("SHA-256", bytes));
  let bin = "";
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return {
    type: "image/jpeg", size: bytes.length,
    sha256: [...hash].map((b) => b.toString(16).padStart(2, "0")).join(""),
    data: btoa(bin), preview: URL.createObjectURL(blob),
  };
}

// Previews of what was sent, for this page only: the images themselves are
// not kept anywhere once the laptop has them.
const sentPreviews = new Map<string, string[]>();
export const rememberPreviews = (job: string, urls: string[]) => { if (urls.length) sentPreviews.set(job, urls); };
export const previewsFor = (job: string) => sentPreviews.get(job) ?? [];

/**
 * Signs a decision about something trackline stopped: allow it once, or keep
 * it blocked. It names the check and the target only; the laptop writes the
 * words the agent is told.
 */
export async function signDecision(
  machine: string, project: string, agent: string, session: string,
  kind: "allow" | "deny", check: string, target: string, keys: Passkey[],
) {
  const now = Date.now();
  const bytes = utf8(JSON.stringify({
    v: 1, id: jobId(), machine, project, kind, agent, text: "", session, check, target,
    issuedAt: now, expiresAt: now + JOB_LIFE_MS,
  }));
  const { key, assertion } = await sign(await sha256(utf8("trackline job v1\n"), bytes), keys);
  return { machine, envelope: { job: b64u(bytes), key: key.id, ...assertion } };
}

/** What a job's status means, in words. */
export function jobState(j: { status: string; code: string | null; reason: string | null; asked?: unknown; asking?: boolean }): string {
  if (j.status === "done" && (j.asked || j.asking)) return "Waiting for your answer";
  switch (j.status) {
    case "queued": return "Waiting for the laptop";
    case "delivered": return "Working";
    case "done": return "Done";
    case "expired": return "Not delivered: the laptop was asleep or offline";
    case "cancelled": return "Stopped before it started";
    case "refused": return `The laptop refused it: ${j.reason ?? j.code}`;
    case "failed": return j.code === "stopped" ? "Stopped" : `Failed: ${j.reason ?? "no reason given"}`;
  }
  return j.status;
}

export const active = (status: string) => status === "queued" || status === "delivered";

// What was sent, by job id, on this device: the server keeps a job's text
// only until the laptop has it, and a page that closed before then would
// otherwise have nothing to show. The last 50 are kept.
const SENT = "trackline.sent";

export function remember(job: string, text: string) {
  try {
    const all = JSON.parse(localStorage.getItem(SENT) ?? "[]") as [string, string][];
    localStorage.setItem(SENT, JSON.stringify([...all.filter(([id]) => id !== job), [job, text]].slice(-50)));
  } catch { /* private mode: the page shows it while open */ }
}

export function recall(job: string): string | undefined {
  try {
    return (JSON.parse(localStorage.getItem(SENT) ?? "[]") as [string, string][]).find(([id]) => id === job)?.[1];
  } catch { return undefined; }
}
