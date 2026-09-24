import fs from "node:fs";
import path from "node:path";
import { frontMatter, render, type Heading } from "./markdown";
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
  toc: Heading[];
  /** The page as Markdown, links made absolute, for agents and "Copy page". */
  markdown: string;
  /** The source Markdown with site links, for search sections. */
  source: string;
};

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
      const r = render(rewriteLinks(content));
      return {
        slug: f.replace(/\.md$/, ""),
        title: meta.title ?? f,
        description: meta.description ?? "",
        order: Number(meta.order ?? 99),
        markdown: rewriteLinks(content).replace(/\]\(\/docs\//g, `](${SITE}/docs/`),
        html: r.html,
        toc: r.toc,
        source: rewriteLinks(content),
      };
    })
    .sort((a, b) => a.order - b.order);
}

export function page(slug: string): Page | undefined {
  return pages().find((p) => p.slug === slug);
}
