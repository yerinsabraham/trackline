import type { Metadata } from "next";
import { Doto } from "next/font/google";
import Link from "next/link";
import InstallPrompt from "@/components/landing/InstallPrompt";
import { SITE } from "@/lib/site";
import "./landing.css";

// One idea a section, and the product itself in place of a list of features.
// The numbers in the product shot are an example and say so; every figure under
// "Measured" is a real result that links to its write-up.

const doto = Doto({ subsets: ["latin"], weight: "900", variable: "--font-dot" });

export const metadata: Metadata = {
  title: { absolute: "trackline: your AI agents, on track" },
  alternates: { canonical: "/" },
};

const works = ["Claude Code", "Codex", "Cursor", "Any MCP agent", "Production traces"];

const stats = [
  { big: "11/11", small: "times a blocked agent corrected itself when told why", slug: "do-agents-self-correct" },
  { big: "14 ms", small: "the cost of every check, before every action", slug: "hook-latency" },
  { big: "60/60", small: "held-out drifts caught by the judge", slug: "the-judge-measured" },
  { big: "0 lines", small: "changed in the core engine to add production", slug: "production-traces" },
];

const agents = [
  { name: "CTO", doing: "Fixing the lobby timer · Opus 5.5", status: "Working", on: true },
  { name: "Reviewer", doing: "Checking the match tests · GPT-6 Astra", status: "Working", on: true },
  { name: "Designer", doing: "Updated the pricing page · Sonnet 5.5", status: "Done", on: false },
];

export default function Home() {
  return (
    <div className={`land ${doto.variable}`}>
            {/* Tells search engines this is a developer tool, not an article. */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify({
            "@context": "https://schema.org",
            "@type": "SoftwareApplication",
            name: "trackline",
            applicationCategory: "DeveloperApplication",
            operatingSystem: "macOS, Linux, Windows",
            description:
              "Run Claude Code, Codex and Cursor from one place. See who is doing what, approve what matters, and know the moment one drifts.",
            url: SITE,
            downloadUrl: "https://www.npmjs.com/package/trackline",
            offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
          }),
        }}
      />
      <section className="land-hero land-wrap">
        <h1>Your AI agents.<br /><span className="land-dot">On track.</span></h1>
        <p className="land-sub">Run Claude Code, Codex and Cursor from one place. See who is doing what, approve what matters, and know the moment one drifts.</p>
        <InstallPrompt second={{ href: "#product", label: "See how it works" }} />
        <p className="land-small">Free and open source. Your agents keep running on your own computer.</p>
      </section>

      <section className="land-wrap" aria-label="The app">
        <div className="pshot">
          <div className="pshot-side" aria-hidden="true">
            <strong>trackline</strong>
            <span className="on">Home</span><span>Chats</span><span>Team</span><span>On track</span>
          </div>
          <div className="pshot-main">
            <h2>Good evening</h2>
            <p>2 agents are working. 1 needs your OK.</p>
            <div className="pshot-ok"><strong>CTO asks Reviewer to check the timer tests</strong><span>Waiting for your OK</span></div>
            {agents.map((a) => (
              <div key={a.name} className="pshot-row">
                <span><strong>{a.name}</strong><small>{a.doing}</small></span>
                <em className={a.on ? "on" : ""}>{a.status}</em>
              </div>
            ))}
          </div>
          <div className="pshot-score">
            <small>On track today</small>
            <strong>94%</strong>
            <small>46 of 49 checked steps matched what you asked for.</small>
          </div>
        </div>
        <p className="land-small land-note">Example data</p>
      </section>

      <section className="land-wrap land-works" aria-label="Works with">
        <p className="land-small">Works with the agents you already use</p>
        <p className="land-works-list">{works.map((w) => <span key={w}>{w}</span>)}</p>
      </section>

      <section id="product" className="land-wrap land-section">
        <h2 className="land-h2">A team you can actually see.</h2>
        <p className="land-sub">Every agent gets a name, a job and a record of what it has done.</p>
        <div className="feats">
          <article className="feat">
            <h3>Know who did what.</h3>
            <p>Every reply carries the agent&apos;s name and the model behind it.</p>
            <div className="feat-demo"><strong>CTO · monkey-post-3b</strong> <small>Opus 5.5</small><br />The timer now follows the server&apos;s clock, so a slow phone can&apos;t drift.</div>
          </article>
          <article className="feat">
            <h3>Give each agent a job.</h3>
            <p>Set a title and a description. It keeps them across every chat.</p>
            <div className="feat-chips">{["CTO", "Reviewer", "Designer", "QA"].map((c) => <span key={c}>{c}</span>)}</div>
          </article>
          <article className="feat feat-dark">
            <h3>Claude can ask Codex.</h3>
            <p>One agent asks another for help. Nothing starts until you read the request and say yes.</p>
            <div className="feat-demo"><span><strong>CTO</strong> asks <strong>Reviewer</strong> to check the timer tests.</span><b>Approve</b></div>
          </article>
          <article className="feat">
            <h3>Stopped before it happens.</h3>
            <p>Secrets and new dependencies are blocked first, and the agent is told why.</p>
            <div className="feat-demo warm"><strong>Blocked: writing to .env</strong><br /><small>Allow once, or keep it blocked.</small></div>
          </article>
        </div>
      </section>

      <section className="land-night">
        <div className="land-wrap">
          <h2 className="land-h2">One number you can trust.</h2>
          <p className="land-sub">On track is what share of your agents&apos; checked actions matched what you asked for. Counted, never guessed.</p>
          <strong className="land-dot big">94%</strong>
          <p className="land-small">Example figure. By project, by agent and by check, and with nothing checked, there is no number.</p>
        </div>
      </section>

      <section className="land-wrap land-section land-phone">
        <div>
          <h2 className="land-h2">Leave your desk. Not your work.</h2>
          <p className="land-sub">Send a message from your phone and it continues on your computer, with the same history. Every message is signed with your passkey.</p>
        </div>
        <div className="land-device" aria-hidden="true">
          <div>
            <strong>CTO · monkey-post-3b</strong>
            <p className="mine">Fix the lobby timer, then get it reviewed.</p>
            <p className="theirs">Fixed. Reviewer is checking the tests now.</p>
            <span>Message CTO</span>
          </div>
        </div>
      </section>

      <section className="land-wrap land-section">
        <h2 className="land-h2">Measured, not promised.</h2>
        <p className="land-sub">Every claim was tested first, and each write-up names its limits.</p>
        <div className="proofs">
          {stats.map((s) => (
            <Link key={s.slug} href={`/evidence/${s.slug}`}><strong>{s.big}</strong><span>{s.small}</span></Link>
          ))}
        </div>
        <Link className="land-link" href="/evidence#limits">Read what it cannot do yet</Link>
      </section>

      <section id="install" className="land-night land-end">
        <div className="land-wrap">
          <h2 className="land-h2">Up and running with <span className="land-dot">one paste.</span></h2>
          <p className="land-sub">Copy the prompt, paste it into your agent, and it installs Trackline for itself.</p>
          <InstallPrompt second={{ href: "/docs/install", label: "Full install guide" }} />
          <p className="land-small">Open source under Apache-2.0</p>
        </div>
      </section>
    </div>
  );
}
