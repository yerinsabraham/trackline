import { ImageResponse } from "next/og";
import { cardFor, cards } from "@/lib/cards";
import { ogKey } from "@/lib/seo";
import { LOGO } from "@/lib/logo";
import { DESCRIPTION, SITE, TAGLINE } from "@/lib/site";

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
  if (card.path === "/") return new ImageResponse(<Home />, { width: 1200, height: 630, fonts: fonts.length ? fonts : undefined });

  return new ImageResponse(
    (
      <div style={{ width: "100%", height: "100%", display: "flex", flexDirection: "column", background: "#f1f1ef", padding: "72px 80px", fontFamily: "Geist" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 20 }}>
          <div style={{ width: 56, height: 56, borderRadius: 14, background: "#111112", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <svg width="36" height="36" viewBox={LOGO.bare} fill="none">
              <path d={LOGO.curve} stroke="#ffffff" strokeWidth={LOGO.stroke} strokeLinecap="round" />
              <circle cx={LOGO.ball.cx} cy={LOGO.ball.cy} r={LOGO.ball.r} fill={LOGO.orange} />
            </svg>
          </div>
          <div style={{ fontSize: 36, color: "#111112" }}>trackline</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", marginTop: "auto", marginBottom: "auto" }}>
          <div style={{ fontSize: 26, color: "#ff5a14", letterSpacing: 2, textTransform: "uppercase" }}>{card.eyebrow}</div>
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

const TEAM = [
  { mark: "CC", tone: "#c1633f", title: "CTO", tool: "Claude Code", status: "Working", ink: "#1a7f43" },
  { mark: "Cx", tone: "#111112", title: "Reviewer", tool: "Codex", status: "Needs your OK", ink: "#cf360e" },
  { mark: "Cu", tone: "#4e4e53", title: "Designer", tool: "Cursor", status: "Done", ink: "#6b6b70" },
];

// The home card is what a link to trackline looks like in WhatsApp, X or
// Slack, so it shows the product: a team of agents and where each one is.
// Small previews crop to the middle, so the words stay large and on the left.
function Home() {
  return (
    <div style={{ width: "100%", height: "100%", display: "flex", background: "#f1f1ef", padding: "64px 72px", fontFamily: "Geist", gap: 56 }}>
      <div style={{ display: "flex", flexDirection: "column", flex: 1 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 18 }}>
          <div style={{ width: 52, height: 52, borderRadius: 13, background: "#111112", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <svg width="34" height="34" viewBox={LOGO.bare} fill="none">
              <path d={LOGO.curve} stroke="#ffffff" strokeWidth={LOGO.stroke} strokeLinecap="round" />
              <circle cx={LOGO.ball.cx} cy={LOGO.ball.cy} r={LOGO.ball.r} fill={LOGO.orange} />
            </svg>
          </div>
          <div style={{ fontSize: 34, color: "#111112" }}>trackline</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", marginTop: "auto", marginBottom: "auto" }}>
          <div style={{ fontSize: 76, fontWeight: 600, color: "#111112", lineHeight: 1.02, letterSpacing: -2.5, display: "flex", flexDirection: "column" }}>
            <span>{TAGLINE.split(". ")[0]}.</span>
            <span style={{ color: "#ff5a14" }}>{TAGLINE.split(". ")[1]}</span>
          </div>
          <div style={{ fontSize: 27, color: "#4e4e53", lineHeight: 1.35, marginTop: 26, maxWidth: 560 }}>{DESCRIPTION}</div>
        </div>
        <div style={{ fontSize: 26, color: "#111112" }}>{SITE.replace(/^https:\/\//, "")}</div>
      </div>
      <div style={{ width: 400, display: "flex", flexDirection: "column", justifyContent: "center", gap: 14 }}>
        <div style={{ fontSize: 22, color: "#6b6b70", marginLeft: 6 }}>Your team</div>
        {TEAM.map((t) => (
          <div key={t.title} style={{ display: "flex", alignItems: "center", gap: 18, background: "#ffffff", borderRadius: 24, padding: "20px 22px", border: "1px solid #e4e4e0" }}>
            <div style={{ width: 56, height: 56, borderRadius: 28, background: t.tone, color: "#ffffff", display: "flex", alignItems: "center", justifyContent: "center", fontSize: 21, fontWeight: 600 }}>{t.mark}</div>
            <div style={{ display: "flex", flexDirection: "column", flex: 1 }}>
              <div style={{ fontSize: 27, fontWeight: 600, color: "#111112" }}>{t.title}</div>
              <div style={{ fontSize: 20, color: "#6b6b70" }}>{t.tool}</div>
            </div>
            <div style={{ fontSize: 20, fontWeight: 600, color: t.ink }}>{t.status}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
