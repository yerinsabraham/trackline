// trackline's service worker. It shows an alert the server pushed, opens the
// session it is about when tapped, and keeps the app's pages for when there is
// no signal. Only same-origin page and asset loads are kept, never an API
// answer: the network is asked first for pages, so the site stays fresh.

const SHELL = "trackline-shell-v1";
const OFFLINE = "/app";
const KEPT = 150;

self.addEventListener("fetch", (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/_next/static/")) {
    // Named by their contents, so a kept one is never stale.
    event.respondWith(caches.open(SHELL).then(async (c) => {
      const hit = await c.match(req);
      if (hit) return hit;
      const res = await fetch(req);
      if (res.ok) {
        await c.put(req, res.clone());
        // Each deploy brings new names; the oldest go, so this cannot grow for ever.
        const all = await c.keys();
        await Promise.all(all.filter((k) => k.url.includes("/_next/static/")).slice(0, -KEPT).map((k) => c.delete(k)));
      }
      return res;
    }));
  } else if (req.mode === "navigate" && url.pathname.startsWith("/app")) {
    event.respondWith(fetch(req).then((res) => {
      const keep = res.clone();
      if (res.ok) caches.open(SHELL).then((c) => c.put(url.pathname, keep));
      return res;
    }).catch(async () => (await caches.match(url.pathname)) ?? (await caches.match(OFFLINE)) ?? Response.error()));
  }
});

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(
  caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== SHELL).map((k) => caches.delete(k)))).then(() => self.clients.claim()),
));

self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { body: event.data ? event.data.text() : "" };
  }
  // Only paths on this site: a push cannot send anyone elsewhere.
  const url = typeof data.url === "string" && data.url.startsWith("/") && !data.url.startsWith("//") ? data.url : "/app";
  event.waitUntil(
    Promise.resolve(self.navigator.setAppBadge?.()).catch(() => {}).then(() => self.registration.showNotification(data.title || "trackline", {
      body: data.body || "",
      tag: data.tag || url,
      renotify: true,
      icon: "/icon-192.png",
      badge: "/icon-192.png",
      data: { url },
    })),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const path = event.notification.data?.url || "/app";
  const url = new URL(path, self.location.origin).href;
  // An open window is sent to the session, not just brought forward: on an
  // iPhone the app is usually open already, and focusing it alone left people
  // on whatever page it was showing (found in real use, 2026-09-25).
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(async (wins) => {
      const w = wins.find((c) => "focus" in c);
      if (!w) return self.clients.openWindow(url);
      await w.focus();
      if (w.url === url) return w;
      if ("navigate" in w) {
        try {
          const moved = await w.navigate(url);
          if (moved) return moved;
        } catch {
          // A window this worker does not control cannot be navigated;
          // it is asked to go there itself.
        }
      }
      w.postMessage({ type: "trackline:open", url: path });
      return w;
    }),
  );
});
