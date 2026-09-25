"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { session } from "@/lib/account";

// "Sign in" for a visitor, "Dashboard" once signed in. The page is static, so
// it renders as "Sign in" and switches after loading if a session is kept.
export default function AccountButton() {
  const [signedIn, setSignedIn] = useState(false);
  useEffect(() => setSignedIn(!!session()), []);
  // A tapped alert asks an open window to go to its session when the service
  // worker cannot move the window itself. The header is on every page, so this
  // listens everywhere. Only paths on this site are followed.
  useEffect(() => {
    if (!("serviceWorker" in navigator)) return;
    const go = (e: MessageEvent) => {
      const url = e.data?.type === "trackline:open" ? e.data.url : null;
      if (typeof url === "string" && url.startsWith("/") && !url.startsWith("//")) window.location.assign(url);
    };
    navigator.serviceWorker.addEventListener("message", go);
    return () => navigator.serviceWorker.removeEventListener("message", go);
  }, []);
  return signedIn ? (
    <Link className="btn btn-quiet" href="/app"><span className="plus">+</span>Dashboard</Link>
  ) : (
    <Link className="btn btn-quiet" href="/signin"><span className="plus">+</span>Sign in</Link>
  );
}
