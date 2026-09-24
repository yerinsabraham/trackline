import { releases } from "@/lib/changelog";
import { SITE } from "@/lib/site";

export const dynamic = "force-static";

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

export function GET() {
  const items = releases()
    .map((r) => `    <item>
      <title>trackline ${esc(r.version)}</title>
      <link>${SITE}/changelog#v${r.version}</link>
      <guid>${SITE}/changelog#v${r.version}</guid>
      <pubDate>${new Date(r.date + "T12:00:00Z").toUTCString()}</pubDate>
      <description>${esc(r.html)}</description>
    </item>`)
    .join("\n");
  const xml = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>trackline changelog</title>
    <link>${SITE}/changelog</link>
    <description>What changed in each release of trackline.</description>
${items}
  </channel>
</rss>
`;
  return new Response(xml, { headers: { "content-type": "application/rss+xml; charset=utf-8" } });
}
