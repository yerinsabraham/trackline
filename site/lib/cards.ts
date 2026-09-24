import { agents } from "./hosts";
import { experiments } from "./experiments";
import { pages } from "./guide";
import { ogKey } from "./seo";

// One share card per public page: the words on it are the page's own title.
export type Card = { path: string; eyebrow: string; title: string };

export function cards(): Card[] {
  return [
    { path: "/", eyebrow: "trackline", title: "Know when your agent goes off track." },
    { path: "/docs", eyebrow: "Docs", title: "Everything trackline does." },
    ...pages().map((p) => ({ path: `/docs/${p.slug}`, eyebrow: "Docs", title: p.title })),
    { path: "/evidence", eyebrow: "Evidence", title: "Every claim here was measured first." },
    ...experiments().map((e) => ({ path: `/evidence/${e.slug}`, eyebrow: `Experiment ${String(e.n).padStart(2, "0")}`, title: e.title })),
    { path: "/agents", eyebrow: "Agents", title: "One set of rules for every agent you use." },
    ...agents().map((a) => ({ path: `/agents/${a.slug}`, eyebrow: "Agents", title: `trackline in ${a.title}` })),
    { path: "/changelog", eyebrow: "Changelog", title: "What changed in each release." },
    { path: "/about", eyebrow: "About", title: "A neutral layer for every AI agent." },
    { path: "/privacy", eyebrow: "Privacy", title: "What trackline collects, and what it never does." },
    { path: "/terms", eyebrow: "Terms", title: "The terms for using trackline." },
  ];
}

export const cardFor = (key: string) => cards().find((c) => ogKey(c.path) === key);
