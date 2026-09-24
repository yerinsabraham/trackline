"use client";

import { useEffect, useState, type ReactNode } from "react";

const icon = (d: ReactNode) => (
  <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">{d}</svg>
);

const CHECKS: { name: string; what: string; judge?: boolean; icon: ReactNode }[] = [
  { name: "Off-limits", what: "Writes to .env, keys, credentials, whatever you protect.",
    icon: icon(<><rect x="3" y="7" width="10" height="7.5" rx="1.5" /><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" /></>) },
  { name: "New dependency", what: "A package appearing that nobody asked for.",
    icon: icon(<><path d="M8 1.5l6 3.2v6.6L8 14.5l-6-3.2V4.7z" /><path d="M2 4.7l6 3.2 6-3.2M8 7.9v6.6" /></>) },
  { name: "Scope", what: "Edits outside the files or area your request named.",
    icon: icon(<><circle cx="8" cy="8" r="5.5" /><circle cx="8" cy="8" r="2" /></>) },
  { name: "Diff size", what: "A change far bigger than the ask.",
    icon: icon(<><path d="M3 5h6M6 2v6M3 12h6" /><path d="M12 3v10" /></>) },
  { name: "Loops", what: "The same action repeated over and over.",
    icon: icon(<><path d="M13 8a5 5 0 1 1-1.5-3.6" /><path d="M13 2.5v3h-3" /></>) },
  { name: "Tool policy", what: "In production: a tool that must never be called, or one called without its approval step first.",
    icon: icon(<path d="M8 1.5l5.5 2v4c0 3.5-2.5 5.8-5.5 7-3-1.2-5.5-3.5-5.5-7v-4z" />) },
  { name: "The judge", what: "Optional. Work that does not serve the request, when no rule could say so.", judge: true,
    icon: icon(<path d="M8 2v12M3 14h10M3.5 5h9M3.5 5L1.8 9.5a1.9 1.9 0 0 0 3.4 0zM12.5 5l-1.7 4.5a1.9 1.9 0 0 0 3.4 0z" />) },
];

// The checks cycle on their own until someone picks one.
export default function CheckTiles() {
  const [active, setActive] = useState(0);
  const [auto, setAuto] = useState(true);

  useEffect(() => {
    if (!auto || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const id = setInterval(() => {
      if (!document.hidden) setActive((i) => (i + 1) % CHECKS.length);
    }, 2800);
    return () => clearInterval(id);
  }, [auto]);

  const pick = (i: number) => { setAuto(false); setActive(i); };
  const c = CHECKS[active];

  return (
    <div className="checks">
      <div className="check-grid" role="group" aria-label="Checks">
        {CHECKS.map((x, i) => (
          <button
            key={x.name}
            type="button"
            className={x.judge ? "check judge" : "check"}
            aria-pressed={i === active}
            onClick={() => pick(i)}
            onMouseEnter={() => pick(i)}
          >
            {x.icon}
            {x.name}
          </button>
        ))}
      </div>
      <div className="check-detail" aria-live="polite">
        <b>{c.name}</b>
        {c.what}
      </div>
      <p className="check-note">
        The checks are arithmetic and never guess. The judge is a model, measured before it was trusted, and off
        until you turn it on.
      </p>
    </div>
  );
}
