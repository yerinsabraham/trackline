"use client";

import { useEffect, useRef } from "react";

type Verdict = "ok" | "note" | "block";

const ACTIONS: { label: string; v: Verdict; says: string }[] = [
  { label: "read README.md", v: "ok", says: "on task" },
  { label: "edit greet.js", v: "ok", says: "on task" },
  { label: "write .env", v: "block", says: "blocked · off-limits" },
  { label: "edit greet.js", v: "ok", says: "on task" },
  { label: "npm install left-pad", v: "note", says: "noted · new dependency" },
  { label: "bash npm test", v: "ok", says: "on task" },
  { label: "edit billing/limits.ts", v: "note", says: "noted · scope" },
];

// Actions stream past the hook and each is judged as it crosses. The blocked one
// stops the stream, because stopping the action is what a block does.
//
// The list is rendered twice so the strip can wrap without moving DOM nodes:
// when the first copy has scrolled out, the second takes its place and inherits
// its verdicts.
export default function WatchGate() {
  const gateRef = useRef<HTMLDivElement>(null);
  const stripRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const gate = gateRef.current!, strip = stripRef.current!;
    const items = Array.from(strip.children) as HTMLElement[];
    const n = ACTIONS.length;
    const judge = (el: HTMLElement) => {
      el.dataset.judged = "1";
      el.classList.add(el.dataset.v!);
    };

    if (matchMedia("(prefers-reduced-motion: reduce)").matches) {
      strip.style.transform = "translateX(40px)";
      items.slice(0, 3).forEach(judge);
      return;
    }

    let x = gate.clientWidth * 0.55, hold = 0, prev = performance.now(), raf = 0;
    const timers: number[] = [];
    const tick = (t: number) => {
      const dt = Math.min(50, t - prev);
      prev = t;
      if (hold > 0) hold -= dt;
      else x -= dt * 0.055;
      const gx = gate.clientWidth / 2;
      for (const el of items) {
        if (el.dataset.judged || x + el.offsetLeft + el.offsetWidth / 2 > gx) continue;
        judge(el);
        if (el.dataset.v === "block") {
          hold = 1500;
          timers.push(window.setTimeout(() => el.classList.add("done"), 1500));
        }
      }
      const setWidth = items[n].offsetLeft - items[0].offsetLeft;
      if (x + setWidth < 0) {
        x += setWidth;
        for (let i = 0; i < n; i++) {
          const a = items[i], b = items[i + n];
          a.className = b.className;
          if (b.dataset.judged) a.dataset.judged = "1";
          else delete a.dataset.judged;
          b.className = "act";
          delete b.dataset.judged;
        }
      }
      strip.style.transform = `translateX(${x}px)`;
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => { cancelAnimationFrame(raf); timers.forEach(clearTimeout); };
  }, []);

  return (
    <div className="gate" ref={gateRef}>
      <div className="gate-line"><span>hook</span></div>
      <div className="strip" ref={stripRef}>
        {[...ACTIONS, ...ACTIONS].map((a, i) => (
          <span key={i} className="act" data-v={a.v} aria-hidden={i >= ACTIONS.length || undefined}>
            {a.label}
            <span className="v">{a.says}</span>
          </span>
        ))}
      </div>
      <p className="gate-cap">checked before it runs · <span>~14 ms</span></p>
    </div>
  );
}
