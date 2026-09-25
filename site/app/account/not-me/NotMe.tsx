"use client";

import { useEffect, useState } from "react";
import { api, clearSession } from "@/lib/account";

export default function NotMe() {
  const [state, setState] = useState("Securing your account…");

  useEffect(() => {
    const token = new URLSearchParams(window.location.search).get("token");
    if (!token) { setState("This security link is incomplete."); return; }
    api("/auth/not-me", { method: "POST", body: JSON.stringify({ token }) })
      .then(() => { clearSession(); setState("Every trackline session has been signed out."); })
      .catch((e) => setState((e as Error).message));
  }, []);

  return (
    <div className="wrap account-page">
      <div className="account-card">
        <p className="eyebrow">trackline security</p>
        <h1>Security check</h1>
        <p className="account-sub">{state}</p>
        <a className="btn btn-signal" href="/signin">Sign in again</a>
      </div>
    </div>
  );
}
