"use client";

import { useEffect, useState } from "react";
import { canPrompt, installed, isIPhone, isPhone, onInstallChange, promptInstall } from "@/lib/install";
import { IconClose } from "./icons";

// On a phone, the one thing that makes trackline work like an app: on the
// Home Screen, then alerts on. Said once, at the top, until done or dismissed.

const KEY = "trackline.hint.dismissed";

export default function InstallHint() {
  const [hint, setHint] = useState<"install" | "alerts" | null>(null);
  const [, rerender] = useState(0);

  useEffect(() => {
    let dismissed = "";
    try { dismissed = localStorage.getItem(KEY) ?? ""; } catch { /* private mode */ }
    if (!isPhone() || window.location.pathname === "/app/alerts") return;
    if (!installed()) setHint(dismissed === "install" ? null : "install");
    else if ("Notification" in window && Notification.permission === "default") setHint(dismissed === "alerts" ? null : "alerts");
    return onInstallChange(() => rerender((n) => n + 1));
  }, []);

  if (!hint) return null;
  const dismiss = () => { try { localStorage.setItem(KEY, hint); } catch { /* private mode */ } setHint(null); };

  return (
    <div className="install-hint" role="note">
      {hint === "install" ? (
        isIPhone() || !canPrompt()
          ? <span>Add trackline to your Home Screen to get alerts on this phone. <a href="/app/alerts">How</a></span>
          : <span>Install trackline to get alerts on this phone. <a href="#" onClick={(e) => { e.preventDefault(); promptInstall(); }}>Install</a></span>
      ) : (
        <span>Hear when an agent needs you. <a href="/app/alerts">Turn on alerts</a></span>
      )}
      <button aria-label="Dismiss" onClick={dismiss}><span style={{ width: 16, display: "flex" }}><IconClose /></span></button>
    </div>
  );
}
