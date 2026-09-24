import { experiments } from "@/lib/experiments";
import { pages } from "@/lib/guide";
import { SITE } from "@/lib/site";

export const dynamic = "force-static";
export const dynamicParams = false;

// Every docs page and experiment as Markdown, for agents and "Copy page".
// Served at /docs/<slug>.md and /evidence/<slug>.md by rewrites in vercel.json;
// the files themselves live under /md/ because a folder cannot hold both a page
// and a route of the same name.
function documents(): Record<string, Record<string, string>> {
  const abs = (md: string) => md.replace(/\]\(\/(?!\/)/g, `](${SITE}/`);
  return {
    docs: Object.fromEntries(
      pages().map((p) => [`${p.slug}.md`, `# ${p.title}\n\n${p.description ? `> ${p.description}\n\n` : ""}${p.markdown}\n`]),
    ),
    evidence: Object.fromEntries(
      experiments().map((e) => [`${e.slug}.md`, `# ${e.title}\n\nPublished ${e.published}. ${SITE}/evidence/${e.slug}\n\n${abs(e.markdown)}\n`]),
    ),
  };
}

export function generateStaticParams() {
  return Object.entries(documents()).flatMap(([section, files]) => Object.keys(files).map((file) => ({ section, file })));
}

export async function GET(_: Request, { params }: { params: Promise<{ section: string; file: string }> }) {
  const { section, file } = await params;
  return new Response(documents()[section][file], { headers: { "content-type": "text/markdown; charset=utf-8" } });
}
