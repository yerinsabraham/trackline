// trackline's service worker. It does one thing: show an alert the server
// pushed, and open the session it is about when tapped. No caching, no fetch
// handling: the site is always loaded fresh.

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

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
    self.registration.showNotification(data.title || "trackline", {
      body: data.body || "",
      tag: data.tag || url,
      renotify: true,
      icon: "/icon-192.png",
      badge: "/icon-192.png",
      data: { url },
    }),
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
