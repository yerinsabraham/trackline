import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import Link from "next/link";
import Mark from "@/components/Mark";
import { GITHUB, SITE } from "@/lib/site";
import "./globals.css";

const sans = Geist({ subsets: ["latin"], variable: "--font-sans" });
const mono = Geist_Mono({ subsets: ["latin"], variable: "--font-mono" });

const DESCRIPTION =
  "Know when your AI agent stops doing what you asked. trackline checks what coding and production agents actually do against the task, the rules and the evidence they were given.";

// metadataBase makes every relative URL below absolute on trackline.dev,
// including the canonical, so the old vercel.app address never competes with
// the real one in search results.
export const metadata: Metadata = {
  metadataBase: new URL(SITE),
  title: { default: "trackline: know when your agent goes off track", template: "%s · trackline" },
  description: DESCRIPTION,
  alternates: { canonical: "/" },
  openGraph: {
    type: "website",
    url: "/",
    siteName: "trackline",
    title: "trackline: know when your agent goes off track",
    description: DESCRIPTION,
    // A real .png file: the static build wrote the generated card with no
    // extension, and some link previews reject an image served without one.
    images: [{ url: "/og.png", width: 1200, height: 630, alt: "trackline: know when your agent goes off track" }],
  },
  twitter: { card: "summary_large_image", images: ["/og.png"] },
  robots: { index: true, follow: true },
};

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
              <Link href="/#how" className="wide">How it works</Link>
              <Link href="/docs">Docs</Link>
            </nav>
            <div className="nav-cta">
              <span className="frame">
                <Link className="btn btn-quiet" href="/evidence"><span className="plus">+</span>Evidence</Link>
              </span>
              <span className="frame">
                <Link className="btn btn-signal" href="/#install"><span className="plus">+</span>Install</Link>
              </span>
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
              <Link href="/evidence">Evidence</Link>
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
