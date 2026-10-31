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

test("scrolling up to read is not undone by a new reply", async ({ page, context }) => {
  const fake = await fakeAccount(context);
  await openChat(page);
  await expect.poll(() => atBottom(page)).toBeLessThan(4);
  await page.locator(".chat-scroll").evaluate((el) => { el.scrollTop = 800; });
  await page.waitForTimeout(300);
  fake.extra = [{ id: "new", at: new Date().toISOString(), turn: "new", text: "The newest reply" }];
  await page.getByText("The newest reply").waitFor({ state: "attached", timeout: 8000 });
  await page.waitForTimeout(300);
  expect(await page.locator(".chat-scroll").evaluate((el) => el.scrollTop)).toBeLessThan(1200);
});

test("a table renders as a table and does not widen the page", async ({ page, context }) => {
  await fakeAccount(context);
  await openChat(page);
  await expect(page.getByRole("cell", { name: "288 KB" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0);
});

test("a chat left open learns the laptop came online without a reload", async ({ page, context }) => {
  const fake = await fakeAccount(context, { continueWith: null });
  await openChat(page);
  await expect(page.locator("textarea")).toBeDisabled();
  fake.continueWith = REMOTE_ON;
  await expect(page.locator("textarea")).toBeEnabled({ timeout: 8000 });
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
