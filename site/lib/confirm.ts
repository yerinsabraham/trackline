// What this device remembers about confirming a message: which way worked
// last, so a passkey it lacks is not tried every time, and until when an email
// code just entered still stands, so one code covers a few messages.

const WITH = "trackline.confirm";
let accountUntil = 0;

export function confirmWith(set?: "passkey" | "choose"): string | null {
  try {
    if (set) localStorage.setItem(WITH, set);
    return localStorage.getItem(WITH);
  } catch {
    return null;
  }
}

/** An email code lasts the server's fifteen minutes; this stops short of them. */
export const accountConfirmed = () => { accountUntil = Date.now() + 14 * 60_000; };
export const accountFresh = () => Date.now() < accountUntil;
