import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import Link from "next/link";
import Mark from "@/components/Mark";
import { GITHUB } from "@/lib/site";
import "./globals.css";

const sans = Inter({ subsets: ["latin"], variable: "--font-sans" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-mono" });

export const metadata: Metadata = {
  title: { default: "trackline", template: "%s · trackline" },
  description:
    "Know when your AI agent stops doing what you asked. trackline checks what coding and production agents actually do against the task, the rules and the evidence they were given.",
};

function GitHubIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M8 0C3.58 0 0 3.58 0 8a8 8 0 0 0 5.47 7.59c.4.07.55-.17.55-.38v-1.33c-2.23.48-2.7-1.07-2.7-1.07-.36-.92-.89-1.17-.89-1.17-.73-.5.05-.49.05-.49.81.06 1.23.83 1.23.83.72 1.23 1.88.87 2.34.67.07-.52.28-.87.51-1.07-1.78-.2-3.65-.89-3.65-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 0 1 4 0c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48v2.2c0 .21.15.46.55.38A8 8 0 0 0 16 8c0-4.42-3.58-8-8-8Z" />
    </svg>
  );
}

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
            <nav className="nav-links">
              <Link href="/docs">Docs</Link>
              <Link href="/evidence">Evidence</Link>
              <a href={GITHUB} className="hide-sm">GitHub</a>
              <Link href="/docs/install" className="btn btn-primary">Install</Link>
            </nav>
          </div>
        </header>
        <main>{children}</main>
        <footer>
          <div className="wrap">
            <a href={GITHUB} className="btn">
              <GitHubIcon />
              GitHub
            </a>
            <div className="links">
              <Link href="/docs">Docs</Link>
              <Link href="/evidence">Evidence</Link>
            </div>
          </div>
        </footer>
      </body>
    </html>
  );
}
