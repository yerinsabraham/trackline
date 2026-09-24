import Link from "next/link";
import CheckTiles from "@/components/CheckTiles";
import DotText from "@/components/DotText";
import HeroActions from "@/components/HeroActions";
import { ClaudeLogo, CursorLogo, OpenAILogo, TraceBars, TracklineTile } from "@/components/HostLogos";
import InstallTabs from "@/components/InstallTabs";
import TypedChat from "@/components/TypedChat";
import WatchGate from "@/components/WatchGate";
import { SITE } from "@/lib/site";

const hosts = [
  { name: <>Claude<br />Code</>, logo: <ClaudeLogo />, via: "Hook", state: "Blocks", cls: "", href: "/agents/claude-code" },
  { name: "Codex", logo: <OpenAILogo />, via: "Hook", state: "Blocks", cls: "", href: "/agents/codex" },
  { name: "Your agent", logo: <TracklineTile />, via: "Any MCP client", state: "Advises", cls: "adv", center: true, href: "/agents/mcp" },
  { name: "Cursor", logo: <CursorLogo />, via: "Hook", state: "Blocks", cls: "", href: "/agents/cursor" },
  { name: <>Production<br />traces</>, logo: <TraceBars />, via: "OTLP", state: "Alerts", cls: "alert", href: "/agents/production" },
];

const table = {
  hosts: ["Claude Code", "Codex", "Cursor", "Any MCP client", "Production traces"],
  rows: [
    { label: "Stops an action before it happens", cells: ["yes", "yes", "yes", "no, advises only", "no, alerts after"] },
    { label: "Tells the agent why", cells: ["yes", "yes", "yes", "yes", "no"] },
    { label: "Knows what you asked", cells: ["yes", "yes", "yes", "if the agent says", "if content capture is on"] },
  ],
};

const stats = [
  { big: "11/11", small: "times a blocked agent corrected itself when told why", n: "01", slug: "do-agents-self-correct", when: "22 Sep 2026" },
  { big: "14 ms", small: "the cost of every check, before every action", n: "02", slug: "hook-latency", when: "22 Sep 2026" },
  { big: "60/60", small: "held-out drifts caught by the judge, against 2 for the rules alone", n: "06", slug: "the-judge-measured", when: "23 Sep 2026" },
  { big: "0 lines", small: "changed in the core engine to add production", n: "07", slug: "production-traces", when: "24 Sep 2026" },
];

const limits: [string, string][] = [
  ["In production it alerts, it cannot stop.", "A trace is a record of what already happened."],
  ["Production needs content capture on.", "Without it, a trace does not say which tool was called, and trackline reports that rather than guessing."],
  ["Scope stays quiet when your request names no file.", "It will not invent a scope you did not state."],
  ["Through MCP, the agent chooses whether to ask.", "An agent that does not ask is not watched."],
  ["Production has not yet met real traffic.", "It was tested on a stream sent by the real OpenTelemetry libraries, with scripted conversations."],
];

function Plus() {
  return <span className="plus">+</span>;
}

export default function Home() {
  return (
    <>
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
              "Watches what coding agents and production agents do, checks it against the task, the rules and the evidence they were given, and tells you or the agent when they part ways.",
            url: SITE,
            downloadUrl: "https://www.npmjs.com/package/trackline",
            offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
          }),
        }}
      />
      <section className="hero wrap">
        <Link className="badge" href="/docs/production">
          <b>New</b> · Production traces <i>›</i>
        </Link>
        <h1>
          <span className="solid">Know when your agent</span>
          <DotText text="goes off track" max={92} pitch={1 / 16} color="#1c1c1e" interactive drift />
        </h1>
        <p className="sub">
          trackline watches what coding agents and production agents actually do, and tells you, or the agent, the
          moment it stops matching what you asked.
        </p>
        <HeroActions />
      </section>

      <section className="hosts wrap" aria-label="Where it works">
        <div className="host-scroll">
          <div className="host-row">
            {hosts.map((h, i) => (
              <Link key={i} href={h.href} className={h.center ? "host center" : "host"}>
                <div className="thumb"><div className="tile">{h.logo}</div></div>
                <h3>{h.name}</h3>
                <div className="host-meta">
                  <span className="chip">{h.via}</span>
                  <span className={`state ${h.cls}`}><i />{h.state}</span>
                </div>
              </Link>
            ))}
          </div>
          <div className="track" aria-hidden="true">
            <span className="pulse" />
            {hosts.map((_, i) => <span key={i} className="node" />)}
          </div>
        </div>
        <p className="track-cap">One set of rules for every agent you use</p>
      </section>

      <section className="problem wrap">
        <div className="problem-grid">
          <div>
            <p className="eyebrow">The problem</p>
            <h2>An agent that goes off task does not crash.</h2>
          </div>
          <div>
            <p>
              It edits files you never mentioned. It installs a package nobody asked for. It ignores the rules file
              it read an hour ago. A support agent changes a credit limit it was told never to touch. And the build
              stays green, because tests check that code does what it was written to do, not whether it is the code
              you asked for.
            </p>
            <p className="kicker">Nothing in the toolchain is watching for that. <span>trackline is.</span></p>
          </div>
        </div>
      </section>

      <section className="dark how" id="how">
        <div className="wrap">
          <div className="how-head">
            <h2>One engine, two places.</h2>
            <p>Beside a coding agent while it works, and over the traces of an agent in production.</p>
            <div className="actions">
              <span className="frame"><Link className="btn btn-quiet" href="/docs"><Plus />Read the docs</Link></span>
              <span className="frame"><Link className="btn btn-signal" href="#install"><Plus />Install</Link></span>
            </div>
          </div>

          <div className="bento">
            <article className="slab s-watch">
              <div className="copy">
                <b>It watches.</b>Beside a coding agent, a hook sees every action before it runs. In production, it
                reads the traces your agent already sends.
              </div>
              <WatchGate />
            </article>

            <article className="slab s-compare">
              <div className="sources">
                <div className="src" style={{ "--i": 3 } as React.CSSProperties}><span className="tick" />What you asked for<small>request</small></div>
                <div className="src" style={{ "--i": 2 } as React.CSSProperties}><span className="tick" />AGENTS.md<small>rules file</small></div>
                <div className="src" style={{ "--i": 1 } as React.CSSProperties}><span className="tick" />Tool policy<small>production</small></div>
                <div className="src hot" style={{ "--i": 0 } as React.CSSProperties}>
                  <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
                    <rect x="3" y="7" width="10" height="7.5" rx="1.5" />
                    <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
                  </svg>
                  .env<small>protected path</small>
                </div>
              </div>
              <div className="copy bottom">
                <b>It compares.</b>Each action is checked against what you asked for, your rules files and a tool
                policy. Most checks are plain rules, not a model.
              </div>
            </article>

            <article className="slab s-tell">
              <TypedChat />
              <div className="copy bottom">
                <b>It tells you, or the agent.</b>By default it only writes things down. You choose, check by check,
                whether it should also stop the agent and explain why, so the agent can correct itself.
              </div>
            </article>

            <article className="slab s-catch">
              <div className="copy inline"><b>What it catches.</b> Six checks, and one question only a model can answer.</div>
              <CheckTiles />
            </article>
          </div>
        </div>
      </section>

      <section className="section wrap" aria-labelledby="where-h">
        <div className="sec-head">
          <div>
            <p className="eyebrow">Where it works</p>
            <h2 id="where-h">One set of rules for every agent you use.</h2>
          </div>
          <p>
            A vendor can govern its own agent. Only something that belongs to none of them can govern all of them the
            same way. trackline is neutral by construction, not by promise: open source, on your machine, with no
            vendor&apos;s model in the path.
          </p>
        </div>
        <div className="table-card">
          <table>
            <thead>
              <tr>
                <th />
                {table.hosts.map((h) => <th key={h}>{h}</th>)}
              </tr>
            </thead>
            <tbody>
              {table.rows.map((r) => (
                <tr key={r.label}>
                  <td>{r.label}</td>
                  {r.cells.map((c, i) => <td key={i} className={c === "yes" ? "yes" : "no"}>{c}</td>)}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="foot-note">
          <code>trackline doctor --host &lt;name&gt;</code> prints the full list of what it can and cannot see in each.{" "}
          <Link className="text-link" href="/agents">See each agent</Link>
        </p>
      </section>

      <section className="section wrap" aria-labelledby="ev-h">
        <div className="sec-head">
          <div>
            <p className="eyebrow">The evidence</p>
            <h2 id="ev-h">Every claim here was measured first.</h2>
          </div>
          <p>Each was measured before it was built on. Where a result has limits, the write-up names them next to the number.</p>
        </div>
        <div className="stats">
          {stats.map((s) => (
            <Link key={s.n} className="stat" href={`/evidence/${s.slug}`}>
              {/* Plain text: a number is read at a glance, and in dots the
                  tilde of "~14ms" read as a minus sign. */}
              <b className="stat-num">{s.big}</b>
              <p>{s.small}</p>
              <span className="src-link"><span>Experiment {s.n} · {s.when}</span><span>→</span></span>
            </Link>
          ))}
        </div>
        <p className="more"><Link href="/evidence">See all seven experiments</Link></p>
      </section>

      <section className="section wrap" aria-labelledby="lim-h">
        <div className="sec-head">
          <div>
            <p className="eyebrow">What it cannot do</p>
            <h2 id="lim-h">Where it sees less.</h2>
          </div>
          <p>Named plainly, because a tool that looks complete stops getting better.</p>
        </div>
        <ul className="limits">
          {limits.map(([b, s]) => <li key={b}><b>{b}</b><span>{s}</span></li>)}
        </ul>
      </section>

      <section className="dark install" id="install" aria-labelledby="inst-h">
        <div className="wrap">
          <h2 id="inst-h">
            Install with
            <DotText text="one paste" max={80} pitch={1 / 15} color="#e4e4e6" board="#1b1b1e" interactive drift introOnView />
          </h2>
          <ol className="steps-strip">
            <li><span>1</span><b>Copy the prompt</b>The button above, or the tab below.</li>
            <li><span>2</span><b>Paste it into your agent</b>It installs trackline and runs <code>init</code> for itself.</li>
            <li><span>3</span><b>Check it fired</b>After the next edit, <code>trackline status</code> shows the hook has run.</li>
          </ol>
          <InstallTabs />
          <p className="after">
            Paste the prompt into your agent, or run the commands yourself. It starts in warn mode: it notices
            things and writes them down, and never interrupts you. Run{" "}
            <code>trackline status</code> after your agent&apos;s next edit to see that it fired.
          </p>
          <div className="actions">
            <span className="frame"><Link className="btn btn-quiet" href="/evidence"><Plus />Read the evidence</Link></span>
            <span className="frame"><Link className="btn btn-signal" href="/docs/install"><Plus />Full install guide</Link></span>
          </div>
        </div>
      </section>
    </>
  );
}
