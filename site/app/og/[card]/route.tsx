import { ImageResponse } from "next/og";
import { cardFor, cards } from "@/lib/cards";
import { ogKey } from "@/lib/seo";
import { SITE } from "@/lib/site";

export const dynamic = "force-static";
export const dynamicParams = false;

// The parameter carries the extension, so the build writes real .png files:
// some link previews refuse an image served without one.
export function generateStaticParams() {
  return cards().map((c) => ({ card: `${ogKey(c.path)}.png` }));
}

async function geist(weight: number): Promise<ArrayBuffer | null> {
  try {
    const css = await (await fetch(`https://fonts.googleapis.com/css2?family=Geist:wght@${weight}`)).text();
    const url = css.match(/src: url\((.+?)\) format\('(?:truetype|opentype)'\)/)?.[1];
    return url ? await (await fetch(url)).arrayBuffer() : null;
  } catch {
    return null;
  }
}

export async function GET(_: Request, { params }: { params: Promise<{ card: string }> }) {
  const card = cardFor((await params).card.replace(/\.png$/, ""))!;
  const [regular, semibold] = await Promise.all([geist(400), geist(600)]);
  const fonts = [
    ...(regular ? [{ name: "Geist", data: regular, weight: 400 as const, style: "normal" as const }] : []),
    ...(semibold ? [{ name: "Geist", data: semibold, weight: 600 as const, style: "normal" as const }] : []),
  ];
  const size = card.title.length > 44 ? 64 : 76;

  return new ImageResponse(
    (
      <div style={{ width: "100%", height: "100%", display: "flex", flexDirection: "column", background: "#f1f1ef", padding: "72px 80px", fontFamily: "Geist" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 20 }}>
          <div style={{ width: 56, height: 56, borderRadius: 14, background: "#111112", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <svg width="36" height="36" viewBox="0 0 24 24" fill="none">
              <path d="M3 17c4 0 5-10 9-10s5 10 9 10" stroke="#ffffff" strokeWidth="2.4" strokeLinecap="round" />
              <circle cx="12" cy="7" r="2.6" fill="#ff4a1c" />
            </svg>
          </div>
          <div style={{ fontSize: 36, color: "#111112" }}>trackline</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", marginTop: "auto", marginBottom: "auto" }}>
          <div style={{ fontSize: 26, color: "#ff4a1c", letterSpacing: 2, textTransform: "uppercase" }}>{card.eyebrow}</div>
          <div style={{ fontSize: size, fontWeight: 600, color: "#111112", lineHeight: 1.08, letterSpacing: -2, marginTop: 18, maxWidth: 1000 }}>{card.title}</div>
        </div>
        <div style={{ display: "flex", justifyContent: "space-between", fontSize: 28, color: "#4e4e53" }}>
          <div>Claude Code · Codex · Cursor · MCP · production traces</div>
          <div style={{ color: "#111112" }}>{SITE.replace(/^https:\/\//, "")}</div>
        </div>
      </div>
    ),
    { width: 1200, height: 630, fonts: fonts.length ? fonts : undefined },
  );
}
