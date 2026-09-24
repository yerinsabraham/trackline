import fs from "node:fs";
import path from "node:path";
import { render, type Heading } from "./markdown";
import { evidence, GITHUB } from "./site";

// The write-ups live in docs/experiments/ and are read at build time, like the
// guide: one text, shown on GitHub and here.
const DIR = path.join(process.cwd(), "..", "docs", "experiments");
const TREE = `${GITHUB}/tree/main/docs/experiments`;
const BLOB = `${GITHUB}/blob/main/docs/experiments`;

export type Experiment = (typeof evidence)[number] & {
  slug: string;
  title: string;
  html: string;
  toc: Heading[];
  markdown: string;
};

export const slugOf = (file: string) => file.replace(/^\d+-/, "").replace(/\.md$/, "");

// Another experiment becomes a page here; the code and data behind a result
// stay on GitHub, where they can be browsed and run.
function rewriteLinks(md: string): string {
  return md.replace(/\]\(([^)#\s]+?)(#[^)]*)?\)/g, (all, target: string, hash = "") => {
    if (/^[a-z]+:/.test(target) || target.startsWith("/") || target.startsWith("#")) return all;
    const clean = target.replace(/^\.\//, "");
    if (/^\d\d-[\w-]+\.md$/.test(clean)) return `](/evidence/${slugOf(clean)}${hash})`;
    if (clean.startsWith("../guide/")) return `](/docs/${clean.slice(9).replace(/\.md$/, "")}${hash})`;
    if (clean.startsWith("../")) return `](${GITHUB}/blob/main/docs/${clean.slice(3)}${hash})`;
    return `](${clean.endsWith("/") ? TREE : BLOB}/${clean}${hash})`;
  });
}

export function experiments(): Experiment[] {
  return evidence.map((e) => {
    const raw = fs.readFileSync(path.join(DIR, e.file), "utf8");
    const title = raw.match(/^# (.*)$/m)?.[1] ?? e.question;
    const body = rewriteLinks(raw.replace(/^# .*\n+/, ""));
    const r = render(body);
    return { ...e, slug: slugOf(e.file), title, html: r.html, toc: r.toc, markdown: body };
  });
}

export function experiment(slug: string): Experiment | undefined {
  return experiments().find((e) => e.slug === slug);
}
