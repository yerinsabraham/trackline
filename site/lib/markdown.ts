import { Marked, type Tokens } from "marked";

export type Heading = { id: string; text: string; depth: number };

export type Rendered = { html: string; toc: Heading[]; text: string };

export function slugify(s: string): string {
  return s.toLowerCase().replace(/<[^>]+>/g, "").replace(/[^\w\s-]/g, "").trim().replace(/\s+/g, "-");
}

export function frontMatter(raw: string): { meta: Record<string, string>; body: string } {
  const m = raw.match(/^---\n([\s\S]*?)\n---\n/);
  if (!m) return { meta: {}, body: raw };
  const meta: Record<string, string> = {};
  for (const line of m[1].split("\n")) {
    const i = line.indexOf(":");
    if (i > 0) meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return { meta, body: raw.slice(m[0].length) };
}

// Every heading gets an id, so a section can be linked, listed under "On this
// page" and found by search. The same ids as GitHub's, as far as plain words go,
// so an anchor copied from one works on the other.
export function render(md: string): Rendered {
  const toc: Heading[] = [];
  const seen = new Map<string, number>();
  const m = new Marked({
    renderer: {
      heading({ tokens, depth }: Tokens.Heading) {
        const inner = this.parser.parseInline(tokens);
        const base = slugify(inner) || "section";
        const n = seen.get(base) ?? 0;
        seen.set(base, n + 1);
        const id = n ? `${base}-${n}` : base;
        const text = inner.replace(/<[^>]+>/g, "");
        if (depth === 2 || depth === 3) toc.push({ id, text, depth });
        return `<h${depth} id="${id}"><a class="anchor" href="#${id}" aria-hidden="true" tabindex="-1">#</a>${inner}</h${depth}>\n`;
      },
    },
  });
  const html = m.parse(md, { async: false }) as string;
  const text = html.replace(/<[^>]+>/g, " ").replace(/&[a-z#0-9]+;/g, " ").replace(/\s+/g, " ").trim();
  return { html, toc, text };
}

// Plain text of each section under its heading, for search results that land on
// the right part of a page rather than its top.
export function sections(md: string): { id: string; heading: string; text: string }[] {
  const out: { id: string; heading: string; text: string }[] = [];
  const seen = new Map<string, number>();
  let cur = { id: "", heading: "", text: "" };
  for (const line of md.split("\n")) {
    const h = line.match(/^(#{2,3})\s+(.*)$/);
    if (h) {
      out.push(cur);
      const plain = h[2].replace(/[`*_]/g, "");
      const base = slugify(plain) || "section";
      const n = seen.get(base) ?? 0;
      seen.set(base, n + 1);
      cur = { id: n ? `${base}-${n}` : base, heading: plain, text: "" };
    } else cur.text += " " + line;
  }
  out.push(cur);
  return out
    .map((s) => ({ ...s, text: s.text.replace(/```[\s\S]*?```/g, " ").replace(/[`*_>#|[\]()-]/g, " ").replace(/\s+/g, " ").trim() }))
    .filter((s) => s.text || s.heading);
}
