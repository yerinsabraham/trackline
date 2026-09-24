"use client";

import { useEffect, useState } from "react";
import { api, githubCallback, setSession, takeGithubState } from "@/lib/account";

export default function Callback() {
  const [error, setError] = useState("");

  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    const code = q.get("code");
    const state = q.get("state");
    const expected = takeGithubState();
    // Clear the code from the address bar and history straight away.
    window.history.replaceState(null, "", "/auth/github/callback");

    if (q.get("error")) return setError("GitHub sign-in was cancelled.");
    if (!code || !state) return setError("That sign-in response was incomplete.");
    if (!expected || expected !== state) {
      return setError("This sign-in was not started in this browser tab. Start again from the sign-in page.");
    }
    api<{ token: string }>("/auth/github/callback", {
      method: "POST",
      body: JSON.stringify({ code, state, redirectUri: githubCallback() }),
    })
      .then((r) => { setSession(r.token); window.location.replace("/account"); })
      .catch((e) => setError((e as Error).message));
  }, []);

  return (
    <div className="wrap account-page">
      <div className="account-card">
        <p className="eyebrow">trackline account</p>
        <h1>{error ? "Could not sign in" : "Signing in…"}</h1>
        {error && (
          <>
            <p className="account-error" role="alert">{error}</p>
            <a className="btn btn-quiet" href="/signin">Back to sign in</a>
          </>
        )}
      </div>
    </div>
  );
}
