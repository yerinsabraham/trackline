// Installing the site as an app on a phone. iPhone has no button a page can
// press: the person uses Share, then Add to Home Screen. Android and desktop
// Chrome offer a prompt the page can show when asked.

type InstallPrompt = Event & { prompt: () => Promise<void>; userChoice: Promise<{ outcome: string }> };

let deferred: InstallPrompt | null = null;
const listeners = new Set<() => void>();

if (typeof window !== "undefined") {
  window.addEventListener("beforeinstallprompt", (e) => {
    e.preventDefault();
    deferred = e as InstallPrompt;
    listeners.forEach((f) => f());
  });
  window.addEventListener("appinstalled", () => { deferred = null; listeners.forEach((f) => f()); });
}

export function installed(): boolean {
  if (typeof window === "undefined") return false;
  return window.matchMedia?.("(display-mode: standalone)").matches || (navigator as { standalone?: boolean }).standalone === true;
}

export const isIPhone = () => typeof navigator !== "undefined" && /iPhone|iPad|iPod/.test(navigator.userAgent);
export const isPhone = () => typeof navigator !== "undefined" && /iPhone|iPad|iPod|Android/.test(navigator.userAgent);

/** Whether this browser will show its own install prompt when asked. */
export const canPrompt = () => deferred !== null;

export async function promptInstall(): Promise<boolean> {
  if (!deferred) return false;
  await deferred.prompt();
  const { outcome } = await deferred.userChoice;
  deferred = null;
  return outcome === "accepted";
}

export function onInstallChange(f: () => void): () => void {
  listeners.add(f);
  return () => listeners.delete(f);
}
