// The site's side of trackline accounts: where the API is, and the session.
//
// The session is a bearer token in localStorage, as the other Creovine
// products hold theirs. The site renders only its own content, and every
// session can be ended from the account page ("sign out everywhere").

export const API = process.env.NEXT_PUBLIC_TRACKLINE_API ?? "https://api.creovine.com/trackline/v1";
export const GOOGLE_CLIENT_ID = "266379412699-5t0tvcark8sfd16pp1u83oenq2ads23v.apps.googleusercontent.com";

const KEY = "trackline.session";
const STATE_KEY = "trackline.github.state";

export type User = { id: string; name: string | null; email: string | null; avatarUrl: string | null; signInMethods: string[] };

export function session(): string | null {
  try { return localStorage.getItem(KEY); } catch { return null; }
}
export function setSession(token: string) {
  try { localStorage.setItem(KEY, token); } catch { /* private mode: the session lasts this tab */ }
}
export function clearSession() {
  try { localStorage.removeItem(KEY); } catch { /* nothing to clear */ }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = session();
  const res = await fetch(`${API}${path}`, {
    ...init,
    headers: {
      ...(init.body ? { "content-type": "application/json" } : {}),
      ...(token ? { authorization: `Bearer ${token}` } : {}),
      ...init.headers,
    },
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw Object.assign(new Error(body.error ?? "Something went wrong."), { status: res.status });
  return body as T;
}

/** Where GitHub sends people back to: a page on this site, never the API. */
export function githubCallback(): string {
  return `${window.location.origin}/auth/github/callback`;
}

/**
 * The state is kept in this tab as well as signed by the API. On return it has
 * to match, which ties the callback to the tab that started it: a link someone
 * else started cannot sign you in to their account.
 */
export async function startGithub() {
  const { url, state } = await api<{ url: string; state: string }>(
    `/auth/github/start?redirect_uri=${encodeURIComponent(githubCallback())}`,
  );
  sessionStorage.setItem(STATE_KEY, state);
  window.location.assign(url);
}

export function takeGithubState(): string | null {
  const s = sessionStorage.getItem(STATE_KEY);
  sessionStorage.removeItem(STATE_KEY);
  return s;
}
