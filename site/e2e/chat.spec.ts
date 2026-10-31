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
  await page.locator(".chat-scroll").evaluate((el) => { el.scrollTop = 0; });
  await expect(page.locator(".bubble-agent", { hasText: "Earlier reply 0" })).toHaveCount(1);
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
  await page.getByRole("button", { name: "Session options" }).click();
  await page.getByRole("button", { name: "Rename" }).click();
  await page.fill("#session-name", "Login work");
  await page.getByRole("button", { name: "Save" }).click();
  await expect.poll(() => fake.calls.find((c) => c.method === "PATCH")?.body).toContain("Login work");
});

test("clearing a chat asks once, then empties it", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  await page.getByRole("button", { name: "Session options" }).click();
  await page.getByRole("button", { name: "Clear chat" }).click();
  await expect(page.getByText("can't be undone")).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Clear chat" }).click();
  await expect(page.locator(".bubble-agent")).toHaveCount(0);
  expect(fake.calls.some((c) => c.path.endsWith("/clear"))).toBe(true);
});

test("a deleted session can be undone, and nothing is sent", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.getByRole("button", { name: "Session options" }).first().click();
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
  await page.getByRole("button", { name: "Session options" }).first().click();
  await page.getByRole("button", { name: "Delete" }).click();
  await expect.poll(() => fake.calls.length, { timeout: 9000 }).toBe(1);
  expect(fake.calls[0].body).toContain('"delete"');
  await expect(page.getByRole("button", { name: "Undo" })).toHaveCount(0);
});

test("leaving the page does not lose a delete", async ({ page, context, browserName }) => {
  test.skip(browserName === "chromium", "Playwright does not see Chromium's keepalive request during unload");
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.getByRole("button", { name: "Session options" }).first().click();
  await page.getByRole("button", { name: "Delete" }).click();
  await page.goto("/app");
  await expect.poll(() => fake.calls.length).toBe(1);
});

test("several sessions can be chosen and deleted", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await page.goto("/app/sessions");
  await page.getByRole("button", { name: "Session options" }).first().click();
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
