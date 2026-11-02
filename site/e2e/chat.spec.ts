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
  await page.locator("textarea").fill("hello");
  await page.locator(".composer-send").click();
  const sheet = page.getByRole("dialog", { name: "Confirm it's you" });
  // No passkey to offer, so it is not offered.
  await expect(sheet.getByRole("button", { name: "Use a passkey" })).toHaveCount(0);
  await sheet.getByRole("button", { name: "Set up this device instead" }).click();
  await expect(sheet.getByRole("button", { name: "Add a passkey" })).toBeVisible();
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

/** A phone has no three dots on a row: holding it opens the same sheet. */
const hold = (page: Page, row = 0) => page.locator(".swipe-row").nth(row).evaluate(async (el) => {
  const e = new Event("touchstart", { bubbles: true });
  Object.defineProperty(e, "touches", { value: [{ clientX: 100, clientY: 100 }] });
  el.dispatchEvent(e);
  await new Promise((r) => setTimeout(r, 560));
  const up = new Event("touchend", { bubbles: true });
  Object.defineProperty(up, "touches", { value: [] });
  el.dispatchEvent(up);
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
  await page.getByRole("button", { name: "Rename chat" }).click();
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
  await page.locator(".swipe-row").first().waitFor();
  await hold(page);
  await page.getByRole("button", { name: "Delete chat" }).click();
  await expect(page.locator(".swipe")).toHaveCount(2);
  await page.getByRole("button", { name: "Undo" }).click();
  await expect(page.locator(".swipe")).toHaveCount(3);
  await page.waitForTimeout(6500);
  expect(fake.calls).toEqual([]);
});

test("a deleted session is really deleted when the Undo runs out", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.locator(".swipe-row").first().waitFor();
  await hold(page);
  await page.getByRole("button", { name: "Delete chat" }).click();
  await expect.poll(() => fake.calls.length, { timeout: 9000 }).toBe(1);
  expect(fake.calls[0].body).toContain('"delete"');
  await expect(page.getByRole("button", { name: "Undo" })).toHaveCount(0);
});

test("leaving the page does not lose a delete", async ({ page, context, browserName }) => {
  test.skip(browserName === "chromium", "Playwright does not see Chromium's keepalive request during unload");
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.locator(".swipe-row").first().waitFor();
  await hold(page);
  await page.getByRole("button", { name: "Delete chat" }).click();
  await page.goto("/app");
  await expect.poll(() => fake.calls.length).toBe(1);
});

test("several sessions can be chosen and deleted", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await hold(page);
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
  await expect(work.locator("summary")).toContainText("Changed 1 file");
  await expect(work.locator(".term-line").first()).toBeHidden();
  await work.locator("summary").click();
  await expect(work.locator(".term-line").first()).toBeVisible();
});

test("moving between tabs keeps the frame and its data", async ({ page, context }) => {
  await fakeAccount(context);
  // Tapped before this arrives, a static export falls back to a full page load.
  const ready = page.waitForResponse(/app\/team\/__next\.app\.team/);
  await page.goto("/app");
  await page.locator(".agent-row").first().waitFor();
  await ready;
  await page.evaluate(() => { (window as unknown as { same: boolean }).same = true; });
  await page.locator(".app-tab", { hasText: "Team" }).click();
  await page.waitForURL(/\/app\/team/);
  await page.locator(".team-card").first().waitFor();
  await page.locator(".app-tab", { hasText: "Home" }).click();
  await expect(page.locator(".agent-row").first()).toBeVisible({ timeout: 500 });
  expect(await page.evaluate(() => (window as unknown as { same?: boolean }).same)).toBe(true);
});

test("the app opens on Home, and what needs you is first", async ({ page, context }) => {
  await fakeAccount(context, { handoffs: [handoff()] });
  await page.goto("/app");
  await expect(page.locator(".page > header h1")).toContainText("Test");
  await expect(page.locator(".ok-card").first()).toContainText("CTO asks Reviewer");
  await expect(page.locator(".tiles3 a").first()).toContainText("90%");
});

test("a chat list opens with what needs you first", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app/sessions");
  await expect(page.locator(".chat-row").first()).toContainText("Needs your OK");
});

test("the new chat button sits exactly in the middle of the bar", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app");
  const plus = (await page.locator(".app-tab-new").boundingBox())!;
  const width = page.viewportSize()!.width;
  expect(Math.abs(plus.x + plus.width / 2 - width / 2)).toBeLessThan(2);
});

test("every chat row shows its model", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  fake.list = [{ ...fake.list[0], model: "gpt-6-astra" }];
  await page.goto("/app/sessions");
  await expect(page.locator(".chat-row").first()).toContainText("GPT-6 Astra");
});

test("with no connection the app says so and shows the last list", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app");
  await page.locator(".agent-row").first().waitFor();
  await context.unroute("https://api.creovine.com/**");
  await context.route("https://api.creovine.com/**", (r) => r.abort());
  await page.reload();
  await expect(page.locator(".offline-bar")).toBeVisible({ timeout: 12000 });
  await expect(page.locator(".agent-row").first()).toBeVisible();
  await page.goto("/app/sessions");
  await expect(page.locator(".chat-row").first()).toBeVisible({ timeout: 12000 });
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

test("the tabs are Home, Chats, Team and Settings, with new chat in the middle", async ({ page, context }) => {
  await fakeAccount(context);
  await page.goto("/app");
  await expect(page.locator(".app-tab")).toHaveText([/Home/, /Chats\d?/, /Team/, /Settings/]);
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
  await expect(page.locator(".term-head")).toHaveCount(1);
  await expect(page.locator(".term-head")).toContainText("used a helper: Explore");
});

test("a message refused because the chat is open says so and can go again", async ({ page, context }) => {
  await remembered(page, "j1", "carry on");
  await fakeAccount(context, { jobs: [job("j1", "refused", { code: "in-use", reason: "this chat is open on your computer. Close it there and send again, or start a new task" })] });
  await page.goto("/app/session?id=s1");
  await expect(page.getByText("This chat is open on your computer")).toBeVisible();
});

test("the team lists members, and a profile is their record", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/team");
  await page.getByRole("link", { name: /Reviewer/ }).click();
  await page.waitForURL(/\/app\/agent/);
  await expect(page.locator(".profile-name")).toContainText("AI agent");
  await expect(page.locator(".record")).toContainText("100%");
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

test("an old install is told what to run, and an up-to-date one is told nothing", async ({ page, context }) => {
  await fakeAccount(context, { cli: "0.14.0", latest: "0.15.0", behind: ["claude-code"] });
  await page.goto("/app");
  const card = page.locator(".update-card");
  await expect(card.getByText("Trackline 0.15.0 is out. Your computer has 0.14.0.")).toBeVisible();
  await expect(card.locator("code").first()).toHaveText("npm install -g trackline@latest");
  await expect(card.getByText("In trackline, Claude Code is missing new hooks.")).toBeVisible();
  // The phone reaches this project, so init alone would leave its messages refused.
  await expect(card.locator("code").nth(1)).toHaveText("trackline init && trackline remote enable");
  const box = await card.boundingBox();
  expect(box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
});

test("nothing to update says nothing, and a development build is never called old", async ({ page, context }) => {
  await fakeAccount(context, { cli: "dev", latest: "0.15.0" });
  await page.goto("/app");
  await expect(page.locator(".agent-row").first()).toBeVisible();
  await expect(page.locator(".update-card")).toHaveCount(0);
});

test("a computer that has gone quiet is said plainly on the chats page", async ({ page, context }) => {
  await fakeAccount(context, { offline: true });
  await page.goto("/app/sessions");
  await expect(page.getByText("Computer offline.")).toBeVisible();
});

test("a position is set in the person's words and says what it reaches", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/agent?id=a1");
  await expect(page.locator(".profile-head h1")).toHaveText("Reviewer");
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
  await expect(page.getByLabel("What should Reviewer do?")).toBeVisible();
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
  await page.locator(".ok-card").first().click();
  await page.waitForURL(/\/app\/approve/);
  await expect(page.locator(".approve-who")).toContainText("CTO asks Reviewer");
  await expect(page.locator(".approve-who")).toContainText("trackline · a new chat");
  await expect(page.locator(".approve-task")).toHaveText("Review the login change.\n\nLook at the tests too.");
  await expect(page.getByRole("button", { name: "Approve" })).toBeVisible();
});

test("a position signed into a message is on screen beside it", async ({ page, context }) => {
  await fakeAccount(context, { position: { title: "CTO", description: "Reviews every change." } });
  await page.goto("/app/session?id=s1");
  await expect(page.locator(".sends-position")).toHaveText("Sent with your position for reviewer-3b: CTO: Reviews every change.");
});

test("a handoff can be declined", async ({ page, context }) => {
  const fake = await fakeAccount(context, { handoffs: [handoff()] });
  await page.goto("/app/approve?id=hf_abcdefgh12");
  await page.getByRole("button", { name: "Decline" }).click();
  await expect.poll(() => fake.calls.some((c) => c.path.endsWith("/decline"))).toBe(true);
});

test("a handoff from an agent still working cannot be approved yet", async ({ page, context }) => {
  await fakeAccount(context, { handoffs: [handoff({ from: { name: "cto-1", title: "CTO", agent: "claude", session: null, working: true } })] });
  await page.goto("/app/approve?id=hf_abcdefgh12");
  await expect(page.locator(".approve")).toContainText("still working");
  await expect(page.getByRole("button", { name: "Approve" })).toHaveCount(0);
});

test("one nobody approved says so", async ({ page, context }) => {
  await fakeAccount(context, { handoffs: [handoff({ status: "expired" })] });
  await page.goto("/app/sessions");
  await expect(page.getByText("Nobody approved this in time")).toBeVisible();
});

test("the three dots add to the team in one tap", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.locator(".swipe-row").first().waitFor();
  await hold(page);
  await page.getByRole("button", { name: "Add to team" }).click();
  await expect.poll(() => fake.calls.some((c) => c.path.endsWith("/team"))).toBe(true);
});

test("the tags around pasted text never show", async ({ page, context }) => {
  const title = '<\\pasted_content id="4aa2"> Do you know FIFA Street?';
  const fake = await fakeAccount(context);
  fake.list = [{ ...fake.list[0], title }];
  await page.goto("/app/sessions");
  await expect(page.locator(".chat-row-title").first()).toHaveText("Do you know FIFA Street?");
  await expect(page.getByText("pasted_content")).toHaveCount(0);
});

test("a message from the computer shows at once, with what the agent is saying", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  fake.live = { turn: "just-sent", request: "Make sure everything aligns before you push", at: new Date().toISOString(), saying: "Checking the phone menu first.", sayingAt: new Date().toISOString(), waiting: null };
  await openChat(page);
  await expect(page.getByText("Make sure everything aligns before you push")).toBeInViewport();
  await expect(page.getByText("Checking the phone menu first.")).toBeInViewport();
});

test("a chat says when the agent is waiting at the computer", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  fake.live = { turn: null, request: null, at: null, saying: null, sayingAt: null, waiting: { what: "Claude needs your permission to use Bash", at: new Date().toISOString() } };
  await openChat(page);
  await expect(page.getByText("Waiting for you at your computer")).toBeInViewport();
  await expect(page.getByText("Claude needs your permission to use Bash")).toBeVisible();
});

test("a memory summary stored as your message is not shown", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  fake.replies = [{ id: "r", at: new Date().toISOString(), turn: "t", text: "Done.", request: "This session is being continued from a previous conversation that ran out of context." }];
  fake.events = [];
  await openChat(page);
  await expect(page.getByText("Done.")).toBeVisible();
  await expect(page.getByText("being continued from a previous")).toHaveCount(0);
});

test("a step shows the agent's own line, and the command where that is on", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  const at = new Date().toISOString();
  const step = { id: "e", at, turn: "t", request: "Check it", action: "run-command", tool: "Bash", paths: [], installs: [], pathsUnknown: false, commandUnderstood: true, blocked: false, severity: null, checked: true, findings: [], unmeasured: [], detail: "Find the minimap", command: "grep -rn minimap tools" };
  fake.events = [step, { ...step, id: "e2", detail: null, command: null }];
  fake.replies = [];
  await page.goto("/app/session?id=s1");
  await page.locator(".aterm").first().waitFor();
  await page.locator(".term-head").first().click();
  await expect(page.getByText("Find the minimap")).toBeVisible();
  await expect(page.getByText("grep -rn minimap tools")).toBeVisible();
  await expect(page.getByText("a command", { exact: true })).toBeVisible();
  await expect(page.locator(".aterm")).not.toContainText("Bash");
});

test("an open chat tells the server it is in front of you", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  await expect.poll(() => fake.viewing.includes(true)).toBe(true);
});

const PHONE_KEY = [{ id: "cGhvbmUta2V5", publicKey: "x", name: "Phone", transports: ["internal"] }];

// A browser asked for a passkey it does not hold says only "not allowed".
async function noPasskeyHere(context: import("@playwright/test").BrowserContext) {
  await context.addInitScript(() => {
    (window as unknown as { asked: number }).asked = 0;
    Object.defineProperty(navigator.credentials, "get", {
      value: () => { (window as unknown as { asked: number }).asked++; return Promise.reject(new DOMException("no passkey", "NotAllowedError")); },
    });
  });
}

test("a passkey this device lacks opens every other way, and the send password sends", async ({ page, context }) => {
  await noPasskeyHere(context);
  const fake = await fakeAccount(context, { passkeys: PHONE_KEY, password: { id: "pw_abcdefghijklmnop", salt: "c2FsdHNhbHRzYWx0c2FsdA", rounds: 1000 } });
  await openChat(page);
  await page.locator("textarea").fill("fix the footer");
  await page.locator(".composer-send").click();

  const sheet = page.getByRole("dialog", { name: "Confirm it's you" });
  await expect(sheet.getByText("Your passkey wasn't used on this device.")).toBeVisible();
  await expect(sheet.getByRole("button", { name: "Use a passkey" })).toBeVisible();
  await expect(sheet.getByRole("button", { name: "Set up this device instead" })).toBeVisible();
  const box = await sheet.boundingBox();
  expect(box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);

  await sheet.getByLabel(/send password for MacBook/).fill("correct horse battery");
  await sheet.getByRole("button", { name: "Send with password" }).click();
  await expect(sheet).toHaveCount(0);
  await expect.poll(() => fake.calls.filter((c) => c.path === "/app/remote/jobs").length).toBe(1);
  const env = JSON.parse(fake.calls.find((c) => c.path === "/app/remote/jobs")!.body!).envelope;
  expect(env.key).toBe("pw_abcdefghijklmnop");
  expect(env.authenticatorData).toBe("");
  expect(env.signature).toMatch(/^[A-Za-z0-9_-]{43}$/);
  // Nothing typed leaves the page.
  expect(fake.calls.some((c) => c.body?.includes("correct horse"))).toBe(false);

  // Typed once this visit: the next message goes without asking. The first
  // has to finish, or the second waits behind it.
  fake.jobs = [job("job_sent1", "done")];
  await expect(page.locator(".composer-note").first()).not.toContainText("is working", { timeout: 15_000 });
  await page.locator("textarea").fill("and the header");
  await page.locator(".composer-send").click();
  await expect.poll(() => fake.calls.filter((c) => c.path === "/app/remote/jobs").length).toBe(2);
  await expect(page.getByRole("dialog", { name: "Confirm it's you" })).toHaveCount(0);
});

test("with no send password the sheet says how to set one, and a device that lacked a passkey asks first next time", async ({ page, context }) => {
  await noPasskeyHere(context);
  const fake = await fakeAccount(context, { passkeys: PHONE_KEY });
  await openChat(page);
  await page.locator("textarea").fill("fix the footer");
  await page.locator(".composer-send").click();
  const sheet = page.getByRole("dialog", { name: "Confirm it's you" });
  await expect(sheet.locator("code")).toHaveText("trackline remote password");
  await sheet.getByRole("button", { name: "Set up this device instead" }).click();
  await expect(sheet.getByRole("button", { name: "Add a passkey" })).toBeVisible();
  await expect(sheet.locator("code").last()).toHaveText("trackline remote enable --pair");
  await sheet.getByRole("button", { name: "Cancel" }).click();
  await expect(page.getByText("Nothing was sent.").first()).toBeVisible();
  expect(fake.calls.filter((c) => c.path === "/app/remote/jobs")).toHaveLength(0);

  await page.reload();
  await page.locator("textarea").fill("again");
  await page.locator(".composer-send").click();
  await expect(page.getByRole("dialog", { name: "Confirm it's you" })).toBeVisible();
  expect(await page.evaluate(() => (window as unknown as { asked: number }).asked)).toBe(0);
});
