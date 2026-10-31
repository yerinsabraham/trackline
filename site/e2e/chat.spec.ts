import { expect, test, type Page } from "@playwright/test";
import { blankIcons, fakeAccount, REMOTE_ON } from "./fake-api";

const atBottom = (page: Page) => page.locator(".chat-scroll").evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight);

async function openChat(page: Page) {
  await page.goto("/app/session?id=s1");
  await page.locator(".bubble-agent").first().waitFor();
}

test("no icon is blank, on any page a phone opens", async ({ page, context }) => {
  await fakeAccount(context);
  for (const path of ["/app", "/app/sessions", "/app/session?id=s1", "/app/remote", "/app/alerts", "/app/machines", "/account"]) {
    await page.goto(path);
    await page.waitForLoadState("networkidle");
    expect(await blankIcons(page), path).toEqual([]);
  }
});

test("the back and send buttons show their arrows", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  for (const button of [page.locator(".chat-back svg"), page.locator(".composer-send svg")]) {
    const box = await button.boundingBox();
    expect(box?.width).toBeGreaterThanOrEqual(18);
    expect(box?.height).toBeGreaterThanOrEqual(18);
  }
});

test("a chat opens on its newest message, not the top", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  await expect.poll(() => atBottom(page)).toBeLessThan(4);
  await expect(page.getByRole("table")).toBeInViewport();
});

test("a new reply is followed when you are at the bottom", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  await expect.poll(() => atBottom(page)).toBeLessThan(4);
  fake.extra = [{ id: "new", at: new Date().toISOString(), turn: "new", text: "The newest reply" }];
  await expect(page.getByText("The newest reply")).toBeInViewport({ timeout: 8000 });
});

// As a finger does: many small moves. One jump of the whole distance is a single
// scroll event, which the follower can lose if it lands as the page changes height.
const swipeUp = (page: Page, steps: number) => page.locator(".chat-scroll").evaluate(async (el, n) => {
  for (let i = 0; i < n; i++) {
    el.scrollTop -= 60;
    await new Promise((r) => requestAnimationFrame(r));
  }
}, steps);
const NEW_REPLY = () => [{ id: "new", at: new Date().toISOString(), turn: "new", text: "The newest reply" }];
const position = (page: Page) => page.locator(".chat-scroll").evaluate((el) => el.scrollTop);

test("scrolling up to read is not undone by a new reply", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  await expect.poll(() => atBottom(page)).toBeLessThan(4);
  await swipeUp(page, 30);
  await page.waitForTimeout(100);
  const reading = await position(page);
  fake.extra = NEW_REPLY();
  await page.getByText("The newest reply").waitFor({ state: "attached", timeout: 8000 });
  await page.waitForTimeout(300);
  expect(await position(page)).toBe(reading);
});

test("a reply landing in the middle of a swipe does not pull you back", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  await expect.poll(() => atBottom(page)).toBeLessThan(4);
  fake.extra = NEW_REPLY();
  // The next poll arrives while the finger is moving.
  for (let i = 0; i < 12; i++) await swipeUp(page, 15);
  await page.getByText("The newest reply").waitFor({ state: "attached", timeout: 8000 });
  const reading = await position(page);
  await page.waitForTimeout(300);
  expect(await position(page)).toBe(reading);
  expect(await atBottom(page)).toBeGreaterThan(400);
});

test("a table renders as a table and does not widen the page", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  await expect(page.getByRole("cell", { name: "288 KB" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0);
});

test("you can always type, and a chat left open learns the laptop came online", async ({ page, context }) => {
  const fake = await fakeAccount(context, { continueWith: null });
  await openChat(page);
  const box = page.locator("textarea");
  await expect(box).toBeEnabled();
  await box.fill("written before the laptop was ready");
  await expect(page.locator(".composer-send")).toBeDisabled();
  await expect(page.getByText("trackline remote enable")).toBeVisible();
  fake.continueWith = REMOTE_ON;
  await expect(page.locator(".composer-send")).toBeEnabled({ timeout: 8000 });
  await expect(box).toHaveValue("written before the laptop was ready");
});

test("links in a reply and in your own message open in a new tab", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  const reply = page.getByRole("link", { name: "the docs" });
  await expect(reply).toHaveAttribute("href", "https://example.com/docs");
  await expect(reply).toHaveAttribute("target", "_blank");
  await expect(reply).toHaveAttribute("rel", /noopener/);
  const mine = page.locator(".bubble-me a").last();
  await expect(mine).toHaveAttribute("href", "https://example.com/ticket");
  await expect(mine).toHaveAttribute("target", "_blank");
});

test("a passkey can be added from the chat, without leaving it", async ({ page, context }) => {
  await fakeAccount(context, { passkeys: [] });
  await openChat(page);
  await expect(page.getByRole("button", { name: "Add a passkey" })).toBeVisible();
  expect(new URL(page.url()).pathname).toBe("/app/session");
});

test("the send button is visible before and after typing", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  const send = page.locator(".composer-send");
  await expect(send).toBeDisabled();
  expect(await send.evaluate((el) => getComputedStyle(el).boxShadow)).not.toBe("none");
  await page.locator("textarea").fill("hello");
  await expect(send).toBeEnabled();
  await expect.poll(() => send.evaluate((el) => getComputedStyle(el).backgroundColor)).toBe("rgb(207, 54, 14)");
});

test("a half-written message is still there when you come back", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  await page.locator("textarea").fill("half a thought");
  await page.reload();
  await expect(page.locator("textarea")).toHaveValue("half a thought");
});

test("passkeys are ready before you tap send", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  expect(fake.passkeyCalls).toBeGreaterThan(0);
});

test("text boxes do not trigger zoom, and buttons fit a finger", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  expect(parseFloat(await page.locator("textarea").evaluate((el) => getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(16);
  for (const sel of [".chat-back", ".composer-send", ".composer-attach"]) {
    const box = await page.locator(sel).boundingBox();
    expect(box?.height, sel).toBeGreaterThanOrEqual(44);
    expect(box?.width, sel).toBeGreaterThanOrEqual(44);
  }
});

test("the page is set up for the iPhone's safe areas", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app");
  expect(await page.locator('meta[name="viewport"]').getAttribute("content")).toContain("viewport-fit=cover");
});

const at = (mins: number) => new Date(Date.now() - mins * 60_000).toISOString();
const job = (id: string, status: string, over = {}) => ({ id, kind: "prompt", status, code: null, reason: null, createdAt: at(1), stopRequestedAt: null, asked: null, permission: null, ...over });
const remembered = (page: Page, id: string, text: string) => page.addInitScript((sent) => localStorage.setItem("trackline.sent", sent), JSON.stringify([[id, text]]));

test("a turn answered in words shows what was asked, with every reply in it", async ({ page, context }) => {
  await fakeAccount(context, { events: [], replies: [
    { id: "a", at: at(5), turn: "t1", text: "First part.", request: "What does the hook do?" },
    { id: "b", at: at(4), turn: "t1", text: "Second part.", request: "What does the hook do?" },
  ] });
  await page.goto("/app/session?id=s1");
  await expect(page.locator(".bubble-me")).toHaveText("What does the hook do?");
  await expect(page.locator(".bubble-agent")).toHaveCount(2);
});

test("a reply that was cut says so", async ({ page, context }) => {
  await fakeAccount(context, { events: [], replies: [{ id: "a", at: at(5), turn: "t1", text: "A long report", cut: true }] });
  await page.goto("/app/session?id=s1");
  await expect(page.locator(".reply-cut")).toBeVisible();
});

test("a message stays after its job is done, even with nothing to replace it", async ({ page, context }) => {
  await fakeAccount(context, { events: [], replies: [{ id: "a", at: at(0), turn: "t1", text: "Done." }], jobs: [job("job_1", "done")] });
  await remembered(page, "job_1", "Keep this message");
  await page.goto("/app/session?id=s1");
  await expect(page.locator(".bubble-me")).toHaveText("Keep this message");
});

test("a message the laptop reported back shows once", async ({ page, context }) => {
  await fakeAccount(context, { events: [], replies: [{ id: "a", at: at(0), turn: "t1", text: "Done.", request: "Fix the footer" }], jobs: [job("job_1", "done")] });
  await remembered(page, "job_1", "Fix the footer");
  await page.goto("/app/session?id=s1");
  await page.locator(".bubble-agent").first().waitFor();
  await expect(page.locator(".bubble-me")).toHaveCount(1);
});

test("a running task can be stopped from the chat", async ({ page, context }) => {
  const fake = await fakeAccount(context, { light: "working", jobs: [job("job_1", "delivered")] });
  await openChat(page);
  await page.getByRole("button", { name: "Stop" }).click();
  await expect.poll(() => fake.stops.length).toBe(1);
  expect(fake.stops[0]).toContain("job_1");
});

test("a message written while the agent works waits, and sends when it finishes", async ({ page, context }) => {
  const fake = await fakeAccount(context, { light: "working" });
  await openChat(page);
  await page.fill("textarea", "Also fix the footer");
  await page.locator(".composer-send").click();
  const held = page.locator(".bubble-me", { hasText: "Also fix the footer" });
  await expect(held).toContainText("to finish");
  await expect(held.getByRole("button", { name: "Send now" })).toBeDisabled();
  await expect(page.locator("textarea")).toHaveValue("");

  await page.reload();
  await expect(held).toBeVisible();

  fake.light = "done";
  await expect(held.getByRole("button", { name: "Send now" })).toBeEnabled();
  await held.getByRole("button", { name: "Remove" }).click();
  await expect(held).toHaveCount(0);
});

test("the agent's question is answered in the chat", async ({ page, context }) => {
  await fakeAccount(context, { jobs: [job("job_1", "done", { asked: { question: "Which colour?", options: ["Red", "Blue"] } })] });
  await openChat(page);
  await expect(page.getByRole("group", { name: "The agent is asking you" })).toContainText("Which colour?");
  await expect(page.getByRole("button", { name: "Red" })).toBeVisible();
});

test("scrolling to the top loads earlier messages and keeps your place", async ({ page, context }) => {
  await fakeAccount(context, { more: true, older: Array.from({ length: 30 }, (_, i) => ({ id: `old${i}`, at: at(2000 - i), turn: `o${i}`, text: `Earlier reply ${i}, with enough words to take up some of the screen when it is read on a phone.` })) });
  await openChat(page);
  const reading = page.locator(".bubble-agent", { hasText: "Reply number 0," });
  // Kept at the top, as a finger is: one jump can land between two frames.
  await expect(async () => {
    const earlier = page.locator(".bubble-agent", { hasText: "Earlier reply 0" });
    // Only until they arrive: scrolling up after that would lose the place being tested.
    if (!(await earlier.count())) await page.locator(".chat-scroll").evaluate((el) => { el.scrollTop = 0; });
    await expect(earlier).toHaveCount(1, { timeout: 800 });
  }).toPass();
  await expect(page.getByRole("button", { name: "Load earlier" })).toHaveCount(0);
  await expect(reading).toBeInViewport();
});

test("coming back opens at the new messages, with a way to the newest", async ({ page, context }) => {
  await fakeAccount(context);
  // The last visit read up to the reply about 80 minutes before the fake's newest.
  await page.addInitScript((t) => localStorage.setItem("trackline.read.s1", String(t)), Date.now() - 100 * 60_000);
  await page.goto("/app/session?id=s1");
  const line = page.locator(".new-line");
  await expect(line).toBeInViewport();
  const jump = page.getByRole("button", { name: "Jump to the newest message" });
  await expect(jump).toBeVisible();
  await jump.click();
  await expect.poll(() => atBottom(page)).toBeLessThan(3);
  await expect(jump).toHaveCount(0);
});

test("a task whose session is known opens as that conversation", async ({ page, context }) => {
  await fakeAccount(context, { job: { id: "job_1", kind: "prompt", agent: "claude", status: "done", code: null, reason: null, output: null, session: "abc", createdAt: at(1), finishedAt: at(0), dashboardSession: "s1" } });
  await page.goto("/app/remote?job=job_1");
  await page.waitForURL(/\/app\/session\?id=s1/);
  await page.locator(".bubble-agent").first().waitFor();
});

test("a chat shows the session's name, and it can be renamed", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  await page.getByRole("button", { name: "Chat options" }).click();
  await page.getByRole("button", { name: "Rename" }).click();
  await page.fill("#session-name", "Login work");
  await page.getByRole("button", { name: "Save" }).click();
  await expect.poll(() => fake.calls.find((c) => c.method === "PATCH")?.body).toContain("Login work");
});

test("clearing a chat asks once, then empties it", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  await page.getByRole("button", { name: "Chat options" }).click();
  await page.getByRole("button", { name: "Clear chat" }).click();
  await expect(page.getByText("can't be undone")).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Clear chat" }).click();
  await expect(page.locator(".bubble-agent")).toHaveCount(0);
  expect(fake.calls.some((c) => c.path.endsWith("/clear"))).toBe(true);
});

test("a deleted session can be undone, and nothing is sent", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.getByRole("button", { name: "Chat options" }).first().click();
  await page.getByRole("button", { name: "Delete" }).click();
  await expect(page.locator(".swipe")).toHaveCount(2);
  await page.getByRole("button", { name: "Undo" }).click();
  await expect(page.locator(".swipe")).toHaveCount(3);
  await page.waitForTimeout(6500);
  expect(fake.calls).toEqual([]);
});

test("a deleted session is really deleted when the Undo runs out", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.getByRole("button", { name: "Chat options" }).first().click();
  await page.getByRole("button", { name: "Delete" }).click();
  await expect.poll(() => fake.calls.length, { timeout: 9000 }).toBe(1);
  expect(fake.calls[0].body).toContain('"delete"');
  await expect(page.getByRole("button", { name: "Undo" })).toHaveCount(0);
});

test("leaving the page does not lose a delete", async ({ page, context, browserName }) => {
  test.skip(browserName === "chromium", "Playwright does not see Chromium's keepalive request during unload");
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.getByRole("button", { name: "Chat options" }).first().click();
  await page.getByRole("button", { name: "Delete" }).click();
  await page.goto("/app");
  await expect.poll(() => fake.calls.length).toBe(1);
});

test("several sessions can be chosen and deleted", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.getByRole("button", { name: "Chat options" }).first().click();
  await page.getByRole("button", { name: "Select several" }).click();
  await page.getByRole("checkbox").nth(0).check();
  await page.getByRole("checkbox").nth(1).check();
  await expect(page.getByRole("toolbar")).toContainText("2 chosen");
  await page.getByRole("toolbar").getByRole("button", { name: "Delete" }).click();
  await expect(page.locator(".swipe")).toHaveCount(1);
  await expect.poll(() => fake.calls.length, { timeout: 9000 }).toBe(1);
});

test("swiping a row left shows Delete", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app/sessions");
  const row = page.locator(".swipe-row").first();
  await row.waitFor();
  const box = (await row.boundingBox())!;
  await row.evaluate((el, b) => {
    // Built by hand: WebKit on a Mac has no Touch to construct.
    const fire = (type: string, x: number) => {
      const e = new Event(type, { bubbles: true });
      Object.defineProperty(e, "touches", { value: type === "touchend" ? [] : [{ clientX: x, clientY: b.y + 20 }] });
      el.dispatchEvent(e);
    };
    fire("touchstart", b.x + 300); fire("touchmove", b.x + 240); fire("touchmove", b.x + 180); fire("touchend", b.x + 180);
  }, box);
  await expect(page.locator(".swipe-delete").first()).toBeVisible();
  await expect.poll(() => row.evaluate((el) => el.style.transform)).toContain("-96");
});

test("search narrows the list", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app/sessions");
  // Typed after the page is live, as a person would.
  await expect(page.locator(".swipe")).toHaveCount(3);
  await page.fill(".list-search", "dark");
  await expect(page.locator(".swipe")).toHaveCount(1);
  await expect(page.locator(".swipe")).toContainText("Add dark mode");
});

test("a turn's work reads as one line, and opens to the steps", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  const work = page.locator(".aterm").last();
  await expect(work.locator("summary")).toContainText("Edited 1 file");
  await expect(work.locator(".term-line").first()).toBeHidden();
  await work.locator("summary").click();
  await expect(work.locator(".term-line").first()).toBeVisible();
});

test("moving between tabs keeps the frame and its data", async ({ page, context }) => {
  await fakeAccount(context);
  // Tapped before this arrives, a static export falls back to a full page load.
  const ready = page.waitForResponse(/app\/team\/__next\.app\.team/);
  await page.goto("/app");
  await page.locator(".swipe .row-link").first().waitFor();
  await ready;
  await page.evaluate(() => { (window as unknown as { same: boolean }).same = true; });
  await page.locator(".app-tab", { hasText: "Team" }).click();
  await page.waitForURL(/\/app\/team/);
  await page.getByText("Reviewer · reviewer-3b").waitFor();
  await page.locator(".app-tab", { hasText: "Chats" }).click();
  await expect(page.locator(".swipe .row-link").first()).toBeVisible({ timeout: 500 });
  expect(await page.evaluate(() => (window as unknown as { same?: boolean }).same)).toBe(true);
});

test("a phone opens on its chats, with what needs you first", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app");
  await expect(page.locator(".swipe .row-link").first()).toContainText("Needs your OK");
});

test("with no connection the app says so and shows the last list", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app");
  await page.locator(".swipe .row-link").first().waitFor();
  await context.unroute("https://api.creovine.com/**");
  await context.route("https://api.creovine.com/**", (r) => r.abort());
  await page.reload();
  await expect(page.locator(".offline-bar")).toBeVisible({ timeout: 12000 });
  await expect(page.locator(".swipe .row-link").first()).toBeVisible();
});

test("dark mode leaves nothing dark on dark", async ({ page, context }) => {
  await fakeAccount(context);
  await page.emulateMedia({ colorScheme: "dark" });
  await openChat(page);
  const look = () => page.evaluate(() => {
    const lum = (c: string) => { const [r, g, b] = c.match(/[\d.]+/g)!.map(Number); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
    const bg = (el: Element | null): string => {
      for (; el; el = el.parentElement) { const c = getComputedStyle(el).backgroundColor; if (!/rgba\(.*, 0\)|transparent/.test(c)) return c; }
      return "rgb(255,255,255)";
    };
    return [...document.querySelectorAll(".chat-head h1, .bubble-agent p, .bubble-me > div, .composer textarea, .row-meta")].map((el) => {
      const cs = getComputedStyle(el);
      return { what: el.className || el.tagName, gap: Math.abs(lum(cs.color) - lum(bg(el))) };
    }).filter((x) => x.gap < 90);
  });
  expect(await look()).toEqual([]);
  expect(await page.evaluate(() => getComputedStyle(document.querySelector(".chat-scroll")!.closest(".app-shell")!).colorScheme)).toBe("dark");
});

test("stopping a laptop asks in the app, not in a browser box", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app/machines");
  await page.getByRole("button", { name: /^Stop/ }).first().click();
  await expect(page.getByRole("dialog")).toContainText("Stop remote on this computer?");
  await page.getByRole("button", { name: "Cancel" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("what the agent says while it works shows as it arrives", async ({ page, context }) => {
  await fakeAccount(context, { light: "working", jobs: [job("job_1", "delivered", { saying: "Reading the login form now." })] });
  await openChat(page);
  await expect(page.locator(".bubble-agent", { hasText: "Reading the login form now." })).toBeVisible();
});

test("a ping from the server updates a chat at once", async ({ page, context }) => {
  const fake = await fakeAccount(context, { light: "working" });
  await openChat(page);
  fake.extra.push({ id: "live", at: new Date().toISOString(), turn: "t39", text: "Arrived by ping." });
  await page.evaluate(() => window.dispatchEvent(new Event("app:change")));
  await expect(page.locator(".bubble-agent", { hasText: "Arrived by ping." })).toBeVisible({ timeout: 1500 });
});

test("the tabs are Chats, Team and Settings", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app");
  await expect(page.locator(".app-tab")).toHaveText(["Chats1", "Team", "Settings"].map((x) => new RegExp(x.replace("1", "\\d?"))));
  await page.locator(".app-tab", { hasText: "Settings" }).click();
  await expect(page.getByRole("link", { name: "Computers" })).toBeVisible();
  await page.goto("/app/session?id=s1");
  await expect(page.locator(".chat-who")).toBeVisible();
});

test("every reply says who sent it, and the name opens the agent", async ({ page, context }) => {
  const fake = await fakeAccount(context, { events: [], replies: [{ id: "a", at: at(5), turn: "t1", text: "Done.", model: "claude-opus-5-5" }] });
  await page.goto("/app/session?id=s1");
  const from = page.locator(".reply-from");
  await expect(from.locator("strong")).toBeVisible();
  await expect(from).toContainText("Opus 5.5");
  await from.locator("button").click();
  await page.getByRole("button", { name: "Add to team" }).click();
  await expect.poll(() => fake.calls.some((c) => c.path === "/app/sessions/s1/team")).toBe(true);
});

test("a helper is one quiet line", async ({ page, context }) => {
  const step = { id: "h", at: at(5), turn: "t1", request: "Look into it", action: "read-file", tool: "Read", paths: ["a.ts"], installs: [], pathsUnknown: false, commandUnderstood: null, blocked: false, severity: null, checked: true, findings: [], unmeasured: [], helper: "Explore" };
  await fakeAccount(context, { events: [step, { ...step, id: "h2" }], replies: [{ id: "a", at: at(4), turn: "t1", text: "Found it." }] });
  await page.goto("/app/session?id=s1");
  await expect(page.locator(".helper-line")).toHaveCount(1);
  await expect(page.locator(".helper-line")).toContainText("used a helper: Explore");
});

test("a message refused because the chat is open says so and can go again", async ({ page, context }) => {
  await remembered(page, "j1", "carry on");
  await fakeAccount(context, { jobs: [job("j1", "refused", { code: "in-use", reason: "open" })] });
  await page.goto("/app/session?id=s1");
  await expect(page.getByText("This chat is open on your computer")).toBeVisible();
});

test("the team lists members, and a profile is their record", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/team");
  await page.getByRole("link", { name: /Reviewer/ }).click();
  await page.waitForURL(/\/app\/agent/);
  await expect(page.locator(".agent-badge")).toHaveText("AI agent");
  await expect(page.locator(".agent-record")).toContainText("100% aligned");
  await page.getByRole("button", { name: "Rename" }).click();
  await page.getByRole("textbox", { name: "Name" }).fill("CTO");
  await page.getByRole("button", { name: "Continue" }).click();
  await expect.poll(() => fake.calls.find((c) => c.method === "PATCH")?.body).toContain("CTO");
});

test("signing is called Face ID only on an iPhone", async ({ page, context, browserName }) => {
  await fakeAccount(context, { jobs: [job("j1", "done")] });
  await page.goto("/app/session?id=s1");
  await expect(page.locator(".composer-note").first()).toContainText(browserName === "webkit" ? "Face ID" : "Your passkey");
});

test("an agent that is set up but silent is said out loud", async ({ page, context }) => {
  await fakeAccount(context, { silent: true });
  await page.goto("/app/settings");
  await expect(page.getByText("Not watched yet")).toBeVisible();
  await expect(page.getByText("Codex in trackline")).toBeVisible();
});

test("a computer that has gone quiet is said plainly on the chats page", async ({ page, context }) => {
  await fakeAccount(context, { offline: true });
  await page.goto("/app");
  await expect(page.getByText("Computer offline.")).toBeVisible();
});

test("a position is set in the person's words and says what it reaches", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/agent?id=a1");
  await expect(page.locator(".agent-title")).toHaveText("Reviewer");
  await page.getByRole("button", { name: "Set position" }).click();
  await expect(page.getByRole("dialog")).toContainText("never changes what the agent is allowed to do");
  await page.getByLabel("Title").fill("CTO");
  await page.getByRole("button", { name: "Save" }).click();
  await expect.poll(() => fake.calls.find((c) => c.method === "PATCH")?.body).toContain("CTO");
});

test("a new chat with a member already knows who and where", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app/remote?with=a1");
  await expect(page.locator(".app-h1")).toHaveText("New chat with Reviewer · reviewer-3b");
  await expect(page.getByLabel("What should reviewer-3b do?")).toBeVisible();
});

const handoff = (over = {}) => ({
  id: "hf_abcdefgh12", number: 1, project: "trackline", task: "Review the login change.\n\nLook at the tests too.", status: "asked", reason: null, result: null, expiresAt: at(-60), createdAt: at(1), job: null,
  to: { id: "a1", name: "reviewer-3b", title: "Reviewer", host: "codex" },
  from: { name: "cto-1", title: "CTO", agent: "claude", session: "abc", working: false },
  sign: { machine: "m1", project: "cp", agent: "codex", session: null },
  ...over,
});

test("a handoff shows its whole task and asks plainly", async ({ page, context }) => {
  await fakeAccount(context, { handoffs: [handoff()] });
  await page.goto("/app");
  const card = page.locator(".handoff");
  await expect(card).toContainText("CTO · cto-1 asks Reviewer · reviewer-3b");
  await expect(card.locator(".handoff-task")).toHaveText("Review the login change.\n\nLook at the tests too.");
  await expect(card).toContainText("In trackline · a new chat");
  await expect(card.getByRole("button", { name: "Approve" })).toBeVisible();
});

test("a position signed into a message is on screen beside it", async ({ page, context }) => {
  await fakeAccount(context, { position: { title: "CTO", description: "Reviews every change." } });
  await page.goto("/app/session?id=s1");
  await expect(page.locator(".sends-position")).toHaveText("Sent with your position for reviewer-3b: CTO: Reviews every change.");
});

test("a handoff can be declined", async ({ page, context }) => {
  const fake = await fakeAccount(context, { handoffs: [handoff()] });
  await page.goto("/app");
  await page.getByRole("button", { name: "Decline" }).click();
  await expect.poll(() => fake.calls.some((c) => c.path.endsWith("/decline"))).toBe(true);
});

test("a handoff from an agent still working cannot be approved yet", async ({ page, context }) => {
  await fakeAccount(context, { handoffs: [handoff({ from: { name: "cto-1", title: "CTO", agent: "claude", session: null, working: true } })] });
  await page.goto("/app");
  await expect(page.locator(".handoff")).toContainText("still working");
  await expect(page.getByRole("button", { name: "Approve" })).toHaveCount(0);
});

test("one nobody approved says so", async ({ page, context }) => {
  await fakeAccount(context, { handoffs: [handoff({ status: "expired" })] });
  await page.goto("/app");
  await expect(page.getByText("Nobody approved this in time")).toBeVisible();
});

test("the three dots also reach add to team", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.getByRole("button", { name: "Chat options" }).first().click();
  await page.getByRole("button", { name: "Agent and team" }).click();
  await page.getByRole("button", { name: "Add to team" }).click();
  await expect.poll(() => fake.calls.some((c) => c.path.endsWith("/team"))).toBe(true);
});
