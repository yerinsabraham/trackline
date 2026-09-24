import fs from "node:fs";
import path from "node:path";
import { marked } from "marked";
import { SITE } from "./site";

// The guide is written once, in docs/guide/ at the repository root, and read
// here at build time. GitHub renders the same files, so the site and the repo
// cannot say different things.
const DIR = path.join(process.cwd(), "..", "docs", "guide");

export type Page = {
  slug: string;
  title: string;
  description: string;
  order: number;
  html: string;
  markdown: string;
};

function frontMatter(raw: string): { meta: Record<string, string>; body: string } {
  const m = raw.match(/^---\n([\s\S]*?)\n---\n/);
  if (!m) return { meta: {}, body: raw };
  const meta: Record<string, string> = {};
  for (const line of m[1].split("\n")) {
    const i = line.indexOf(":");
    if (i > 0) meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return { meta, body: raw.slice(m[0].length) };
}

const REPO = "https://github.com/yerinsabraham/trackline/blob/main/docs";

// Links between guide pages are written as "configuration.md" so they work on
// GitHub. Here they become site routes; anything pointing out of the guide
// goes to the file on GitHub.
function rewriteLinks(md: string): string {
  return md.replace(/\]\(([^)#\s]+?)(#[^)]*)?\)/g, (all, target: string, hash = "") => {
    if (/^[a-z]+:/.test(target) || target.startsWith("/")) return all;
    if (/^[\w-]+\.md$/.test(target)) return `](/docs/${target.replace(/\.md$/, "")}${hash})`;
    if (target.startsWith("../")) return `](${REPO}/${target.slice(3)}${hash})`;
    return all;
  });
}

export function pages(): Page[] {
  return fs
    .readdirSync(DIR)
    .filter((f) => f.endsWith(".md") && f !== "README.md")
    .map((f) => {
      const { meta, body } = frontMatter(fs.readFileSync(path.join(DIR, f), "utf8"));
      // The page's own H1 is dropped: the layout renders the title.
      const content = body.trimStart().replace(/^# .*\n+/, "");
      return {
        slug: f.replace(/\.md$/, ""),
        title: meta.title ?? f,
        description: meta.description ?? "",
        order: Number(meta.order ?? 99),
        markdown: rewriteLinks(content).replace(/\]\(\/docs\//g, `](${SITE}/docs/`),
        html: marked.parse(rewriteLinks(content), { async: false }) as string,
      };
    })
    .sort((a, b) => a.order - b.order);
}

export function page(slug: string): Page | undefined {
  return pages().find((p) => p.slug === slug);
}
