import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import Link from "next/link";
import AccountButton from "@/components/AccountButton";
import Mark from "@/components/Mark";
import Search from "@/components/Search";
import { GITHUB, SITE } from "@/lib/site";
import "./globals.css";

const sans = Geist({ subsets: ["latin"], variable: "--font-sans" });
const mono = Geist_Mono({ subsets: ["latin"], variable: "--font-mono" });

const DESCRIPTION =
  "Run Claude Code, Codex and Cursor from one place. See who is doing what, approve what matters, and know the moment one drifts.";

// metadataBase makes every relative URL below absolute on trackline.dev,
// including the canonical, so the old vercel.app address never competes with
// the real one in search results.
export const metadata: Metadata = {
  metadataBase: new URL(SITE),
  title: { default: "trackline: your AI agents, on track", template: "%s · trackline" },
  description: DESCRIPTION,
  alternates: { canonical: "/" },
  openGraph: {
    type: "website",
    url: "/",
    siteName: "trackline",
    title: "trackline: your AI agents, on track",
    description: DESCRIPTION,
    // A real .png file: the static build wrote the generated card with no
    // extension, and some link previews reject an image served without one.
    images: [{ url: "/og.png", width: 1200, height: 630, alt: "trackline: your AI agents, on track" }],
  },
  twitter: { card: "summary_large_image", images: ["/og.png"] },
  robots: { index: true, follow: true },
  // Added to a phone's home screen, the dashboard opens as an app, which is
  // what lets an iPhone receive its alerts.
  manifest: "/manifest.webmanifest",
  // Listed in full: naming only the Apple icon here replaced the favicon link
  // Next would otherwise add, and browser tabs showed none.
  icons: {
    // favicon.ico in app/ is linked by Next itself, for browsers without SVG icons.
    icon: [{ url: "/icon.svg", type: "image/svg+xml" }],
    apple: "/apple-touch-icon.png",
  },
  appleWebApp: { capable: true, title: "trackline", statusBarStyle: "default" },
};

// viewport-fit=cover is what makes env(safe-area-inset-*) real on an iPhone:
// without it every inset the CSS asks for is 0.
export const viewport: Viewport = { width: "device-width", initialScale: 1, viewportFit: "cover", themeColor: [{ media: "(prefers-color-scheme: light)", color: "#f1f1ef" }, { media: "(prefers-color-scheme: dark)", color: "#0f0f10" }] };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`}>
      <body>
        <header className="nav">
          <div className="wrap">
            <Link href="/" className="brand">
              <Mark className="brand-mark" />
              trackline
            </Link>
            <nav className="nav-links" aria-label="Main">
              <Link href="/#product" className="wide">Product</Link>
              <Link href="/evidence" className="wide">Evidence</Link>
              <Link href="/docs">Docs</Link>
              <a href={GITHUB} className="wide">Open source</a>
              <Search />
            </nav>
            <div className="nav-cta">
              <AccountButton />
              <Link className="nav-start cta-install" href="/#install">Get started</Link>
            </div>
          </div>
        </header>
        <main>{children}</main>
        {/* The code is open, and the footer is where that is said: the product
            pages sell the product, not the repository. */}
        <footer className="dark">
          <div className="wrap">
            <Link href="/" className="brand">
              <Mark className="brand-mark" />
              trackline
            </Link>
            <span>Open source under Apache-2.0</span>
            <nav aria-label="Footer">
              <Link href="/docs">Docs</Link>
              <Link href="/agents">Agents</Link>
              <Link href="/evidence">Evidence</Link>
              <Link href="/changelog">Changelog</Link>
              <Link href="/about">About</Link>
              <a href={GITHUB}>GitHub</a>
              <Link href="/privacy">Privacy</Link>
              <Link href="/terms">Terms</Link>
            </nav>
          </div>
        </footer>
      </body>
    </html>
  );
}
