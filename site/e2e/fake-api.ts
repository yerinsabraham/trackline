import type { BrowserContext, Page } from "@playwright/test";

// A fake account answered in the browser: nothing reaches the real API.

const iso = (minsAgo: number) => new Date(Date.now() - minsAgo * 60_000).toISOString();

export const TABLE = "Here is the result:\n\n| Metric | Before | After |\n|---|---|---|\n| Bundle | 412 KB | 288 KB |\n\n```\nconst a = 1;\n```\n\nSee [the docs](https://example.com/docs).";

const event = (i: number, minsAgo: number) => ({
  id: `e${i}`, at: iso(minsAgo), turn: `t${i}`, request: `Request number ${i}: please do the next piece of the work, see https://example.com/ticket`,
  action: "edit-file", tool: "Edit", paths: ["site/app/page.tsx"], installs: [], pathsUnknown: false,
  commandUnderstood: null, blocked: false, severity: null, checked: true, findings: [], unmeasured: [],
});

const score = { checked: 40, aligned: 40, notCheckable: 0, percent: 100 };
const summary = (id: string, request: string | null, light: string) => ({
  id, host: "claude-code", startedAt: iso(900), lastAt: iso(2), light, request, score: { request: score, session: score },
  reply: { at: iso(2), turn: "t", text: "| Metric | Before |\n|---|---|\n| LCP | 2.9 s |" },
});

export const REMOTE_ON = { machine: { id: "m1", name: "MacBook", online: true, stopped: false }, project: "cp", agent: "claude", session: "abc" };

export type Fake = { continueWith: typeof REMOTE_ON | null; turns: number; passkeyCalls: number; passkeys: { id: string; publicKey: string; name: string; transports: string[] }[]; extra: { id: string; at: string; turn: string; text: string }[]; light: string; jobs: unknown[]; replies: unknown[] | null; events: unknown[] | null; more: boolean; older: unknown[]; stops: string[]; job: unknown };

export async function fakeAccount(context: BrowserContext, over: Partial<Fake> = {}): Promise<Fake> {
  const fake: Fake = { continueWith: REMOTE_ON, turns: 40, passkeyCalls: 0, passkeys: [{ id: "k", publicKey: "x", name: "Phone", transports: [] }], extra: [], light: "done", jobs: [], replies: null, events: null, more: false, older: [], stops: [], job: null, ...over };
  await context.addInitScript(() => localStorage.setItem("trackline.session", "fake"));
  await context.route("https://api.creovine.com/**", (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace("/trackline/v1", "");
    const json = (body: unknown) => route.fulfill({
      status: 200, contentType: "application/json",
      headers: { "access-control-allow-origin": "*", "access-control-allow-headers": "*", "access-control-allow-methods": "*" },
      body: JSON.stringify(body),
    });
    if (route.request().method() === "OPTIONS") return json({});
    if (path === "/auth/me") return json({ user: { id: "u", name: "Test Person", email: "t@example.com", avatarUrl: null, signInMethods: ["github"] } });
    if (path === "/app/overview") return json({ historyDays: 30, projects: [{ id: "p1", name: "trackline", lastSeenAt: iso(1), agents: [{ host: "claude-code", wired: true, reported: true, lastAt: iso(1) }], sessions: [summary("s1", "Fix the login page", "done"), summary("s2", "yes", "needs-you")] }] });
    if (path === "/app/remote") return json({ machines: [{ id: "m1", name: "MacBook", online: true, lastSeenAt: iso(0), stopped: false, projects: [{ id: "cp", name: "trackline", agents: ["claude"] }], passkeys: [{ id: "k", name: "Phone" }] }], pairings: [], jobs: [] });
    if (path === "/app/ontrack") return json({ score: { percent: 90 } });
    if (path === "/app/remote/passkeys") { fake.passkeyCalls++; return json({ passkeys: fake.passkeys }); }
    if (path === "/app/alerts") return json({ prefs: { red: true, finished: true, drifting: false }, devices: [], slack: false, vapidPublicKey: null });
    if (path.startsWith("/app/remote/jobs/") && path.endsWith("/stop")) { fake.stops.push(path); return json({ status: "delivered" }); }
    if (path.endsWith("/events") && path.startsWith("/app/remote/jobs/")) return json({ status: "done", events: [] });
    if (path.startsWith("/app/remote/jobs/")) return json(fake.job ?? {});
    if (path.startsWith("/app/sessions/")) {
      if (url.searchParams.get("before")) { const older = fake.older; fake.older = []; fake.more = false; return json({ events: [], replies: older, more: false }); }
      const after = url.searchParams.get("after");
      const events = (fake.events ?? Array.from({ length: fake.turns }, (_, i) => event(i, 400 - i * 8))) as ReturnType<typeof event>[];
      const replies = fake.replies ?? events.map((e, i) => ({ id: `r${i}`, at: new Date(+new Date(e.at) + 30_000).toISOString(), turn: e.turn, text: i === fake.turns - 1 ? TABLE : `Reply number ${i}, long enough to take a few lines on a phone so that the page scrolls a good way.` }));
      return json({
        session: { ...summary("s1", "Fix the login page", fake.light), project: { id: "p1", name: "trackline" } },
        events: after ? [] : events, replies: after ? fake.extra : replies, cursor: "c1", continueWith: fake.continueWith, jobs: fake.jobs, more: fake.more,
      });
    }
    return json({});
  });
  return fake;
}

/** Every drawn icon that has no size. */
export function blankIcons(page: Page) {
  return page.evaluate(() => [...document.querySelectorAll("svg")]
    .filter((s) => {
      const el = s.closest("[hidden], .app-side, .chat-list") as HTMLElement | null;
      if (el || getComputedStyle(s).display === "none" || s.getClientRects().length === 0 && !s.parentElement?.offsetParent) return false;
      const r = s.getBoundingClientRect();
      return r.width === 0 || r.height === 0;
    })
    .map((s) => s.parentElement?.outerHTML.slice(0, 120)));
}
