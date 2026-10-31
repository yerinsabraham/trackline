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
