"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const LINES: { cls?: string; text: string }[] = [
  { cls: "who", text: "trackline [BLOCKED]" },
  { text: "writing to a protected path: .env" },
  { cls: "quiet", text: "do this instead: leave this file alone; if the change is genuinely needed, make it by hand" },
  { cls: "agent", text: "agent · I couldn't add API_KEY to .env. That path is protected, so you'll need to edit it yourself." },
];

const FULL = LINES.map((l) => l.text.length);

// The transcript is complete at rest, and types itself out when it first comes
// into view and on Replay.
export default function TypedChat() {
  const [shown, setShown] = useState<number[]>(FULL);
  const [typing, setTyping] = useState(false);
  const run = useRef(0);
  const root = useRef<HTMLDivElement>(null);

  const play = useCallback(() => {
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const id = ++run.current;
    const counts = LINES.map(() => 0);
    let line = 0;
    setTyping(true);
    setShown([...counts]);
    const step = () => {
      if (id !== run.current) return;
      if (line >= LINES.length) { setTyping(false); return; }
      counts[line] = Math.min(FULL[line], counts[line] + (line === 0 ? 1 : 2));
      setShown([...counts]);
      if (counts[line] < FULL[line]) setTimeout(step, 16);
      else { line++; setTimeout(step, line === 1 ? 380 : 260); }
    };
    step();
  }, []);

  useEffect(() => {
    const el = root.current!;
    const io = new IntersectionObserver((es) => {
      if (es[0].isIntersecting) { io.disconnect(); play(); }
    }, { threshold: 0.6 });
    io.observe(el);
    return () => { io.disconnect(); run.current++; };
  }, [play]);

  const last = LINES.length - 1;
  return (
    <>
      <button className="replay" type="button" onClick={play}>↻ Replay</button>
      <div
        ref={root}
        className="chat"
        role="img"
        aria-label="An agent is blocked from writing .env, is told why, and tells the user to make the change by hand."
      >
        <div className="icon">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M3 17c4 0 5-10 9-10s5 10 9 10" stroke="#fff" strokeWidth="2.4" strokeLinecap="round" />
          </svg>
        </div>
        {LINES.map((l, i) => (
          <p key={i} className={l.cls === "who" ? undefined : l.cls}>
            {l.cls === "who" ? <span className="who">{l.text.slice(0, shown[i])}</span> : l.text.slice(0, shown[i])}
            {i === last && !typing && shown[i] === FULL[i] && <span className="caret" />}
          </p>
        ))}
      </div>
    </>
  );
}
