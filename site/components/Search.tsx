"use client";

import { useEffect, useMemo, useRef, useState } from "react";

type Entry = { page: string; heading: string; url: string; text: string; kind: string };

// Every word must appear somewhere in the entry; a match in the heading or the
// page title ranks above one in the body.
function score(e: Entry, words: string[]): number {
  const head = `${e.page} ${e.heading}`.toLowerCase();
  const body = e.text.toLowerCase();
  let s = 0;
  for (const w of words) {
    if (head.includes(w)) s += 3;
    else if (body.includes(w)) s += 1;
    else return 0;
  }
  return s;
}

function snippet(text: string, words: string[]): string {
  const lower = text.toLowerCase();
  const at = Math.max(0, Math.min(...words.map((w) => lower.indexOf(w)).filter((i) => i >= 0)) - 40);
  return (at > 0 ? "…" : "") + text.slice(at, at + 150) + (text.length > at + 150 ? "…" : "");
}

export default function Search() {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [index, setIndex] = useState<Entry[] | null>(null);
  const [active, setActive] = useState(0);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") { e.preventDefault(); setOpen((o) => !o); }
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (!open) return;
    input.current?.focus();
    if (!index) fetch("/search.json").then((r) => r.json()).then(setIndex).catch(() => setIndex([]));
  }, [open, index]);

  const words = useMemo(() => q.toLowerCase().split(/\s+/).filter(Boolean), [q]);
  const results = useMemo(() => {
    if (!index || !words.length) return [];
    return index
      .map((e) => ({ e, s: score(e, words) }))
      .filter((r) => r.s > 0)
      .sort((a, b) => b.s - a.s)
      .slice(0, 8)
      .map((r) => r.e);
  }, [index, words]);

  const go = (url: string) => { setOpen(false); setQ(""); window.location.href = url; };

  return (
    <>
      <button type="button" className="search-btn" onClick={() => setOpen(true)} aria-label="Search the docs">
        <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
          <circle cx="7" cy="7" r="4.5" /><path d="M10.5 10.5 14 14" />
        </svg>
        <span className="wide">Search</span>
        <kbd className="wide">⌘K</kbd>
      </button>
      {open && (
        <div className="search-scrim" onMouseDown={() => setOpen(false)}>
          <div className="search-box" role="dialog" aria-label="Search" onMouseDown={(e) => e.stopPropagation()}>
            <input
              ref={input}
              id="site-search"
              value={q}
              placeholder="Search the docs, agents and evidence"
              onChange={(e) => { setQ(e.target.value); setActive(0); }}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(a + 1, results.length - 1)); }
                if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
                if (e.key === "Enter" && results[active]) go(results[active].url);
              }}
            />
            <div className="search-results">
              {!index && <p className="search-empty">Loading…</p>}
              {index && words.length > 0 && results.length === 0 && <p className="search-empty">Nothing matches “{q}”.</p>}
              {results.map((r, i) => (
                <a key={r.url + i} href={r.url} className={i === active ? "on" : undefined} onMouseEnter={() => setActive(i)}>
                  <span className="search-where">{r.kind} · {r.page}{r.heading ? ` › ${r.heading}` : ""}</span>
                  <span className="search-text">{snippet(r.text, words)}</span>
                </a>
              ))}
            </div>
          </div>
        </div>
      )}
    </>
  );
}
