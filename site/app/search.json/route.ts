import { agents } from "@/lib/hosts";
import { experiments } from "@/lib/experiments";
import { pages } from "@/lib/guide";
import { sections } from "@/lib/markdown";

export const dynamic = "force-static";

// The search index, built once at deploy: one entry per section of every page,
// so a result lands on the right heading. Small enough to fetch whole.
export function GET() {
  const entries = [
    ...pages().flatMap((p) =>
      sections(p.source).map((s) => ({
        page: p.title, heading: s.heading, url: `/docs/${p.slug}${s.id ? `#${s.id}` : ""}`, text: s.text.slice(0, 600), kind: "Docs",
      })),
    ),
    ...experiments().flatMap((e) =>
      sections(e.markdown).map((s) => ({
        page: e.title, heading: s.heading, url: `/evidence/${e.slug}${s.id ? `#${s.id}` : ""}`, text: s.text.slice(0, 600), kind: "Evidence",
      })),
    ),
    ...agents().map((a) => ({
      page: a.title, heading: "", url: `/agents/${a.slug}`, text: `${a.summary} ${a.limits.join(" ")}`, kind: "Agents",
    })),
  ];
  return Response.json(entries);
}
