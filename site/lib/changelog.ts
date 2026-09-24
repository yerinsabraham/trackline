import fs from "node:fs";
import path from "node:path";
import { render } from "./markdown";

// docs/CHANGELOG.md is the one record of releases; GitHub shows it as written,
// and the site splits it into entries for the page and the feed.
const FILE = path.join(process.cwd(), "..", "docs", "CHANGELOG.md");

export type Release = { version: string; date: string; html: string; markdown: string };

export function releases(): Release[] {
  const raw = fs.readFileSync(FILE, "utf8");
  return raw
    .split(/^## /m)
    .slice(1)
    .map((chunk) => {
      const [head, ...rest] = chunk.split("\n");
      const [version, date] = head.split("·").map((s) => s.trim());
      const markdown = rest.join("\n").trim();
      return { version, date, markdown, html: render(markdown).html };
    });
}
