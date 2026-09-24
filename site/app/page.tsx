import Link from "next/link";
import CopyButton from "@/components/CopyButton";
import { EXPERIMENTS } from "@/lib/site";

const INSTALL = "npm install -g trackline";

const checks: [string, string][] = [
  ["Off-limits", "Writes to .env, keys, credentials, whatever you protect."],
  ["New dependency", "A package appearing that nobody asked for."],
  ["Scope", "Edits outside the files or area your request named."],
  ["Diff size", "A change far bigger than the ask."],
  ["Loops", "The same action repeated over and over."],
  ["Tool policy", "In production: a tool that must never be called, or one called without its approval step first."],
  ["The judge", "Optional. Work that does not serve the request, when no rule could say so."],
];

const hosts = ["Claude Code", "Codex", "Cursor", "Any MCP client", "Production traces"];
const rows: { label: string; cells: string[] }[] = [
  { label: "Stops an action before it happens", cells: ["yes", "yes", "yes", "no, advises only", "no, alerts after"] },
  { label: "Tells the agent why", cells: ["yes", "yes", "yes", "yes", "no"] },
  { label: "Knows what you asked", cells: ["yes", "yes", "yes", "if the agent says", "if content capture is on"] },
];

const stats = [
  { big: "11 of 11", small: "times a blocked agent corrected itself when told why", file: "01-do-agents-self-correct.md" },
  { big: "~14 ms", small: "the cost of every check, before every action", file: "02-hook-latency.md" },
  { big: "60 of 60", small: "held-out drifts caught by the judge, against 2 for the rules alone", file: "06-the-judge-measured.md" },
  { big: "0 lines", small: "changed in the core engine to add production", file: "07-production-traces.md" },
];

export default function Home() {
  return (
    <>
      <section className="hero">
        <div className="wrap hero-grid">
          <div>
            <h1>Know when your AI agent stops doing what you asked.</h1>
            <p className="sub">
              trackline watches what coding agents and production agents actually do, checks it against the
              task, the rules and the evidence they were given, and tells you, or the agent, the moment they
              part ways.
            </p>
            <div className="install">
              <code>
                <span className="prompt">$ </span>
                {INSTALL}
              </code>
              <CopyButton text={INSTALL} />
            </div>
            <div className="hero-actions">
              <Link href="/evidence">Read the evidence →</Link>
            </div>
            <p className="hero-meta">Works with Claude Code, Codex, Cursor and any MCP client</p>
          </div>

          <div>
            <div className="term" role="img" aria-label="An agent is blocked from writing .env, is told why, and tells the user to make the change by hand.">
              <div className="term-bar"><span /><span /><span /></div>
              <p><span className="who">you    </span>Add API_KEY=test123 to .env, and a comment to greet.js.</p>
              <p><span className="who">agent  </span>write .env</p>
              <p>
                <span className="stop">trackline  BLOCKED</span>
                {"\n"}
                <span className="dim">writing to a protected path: .env</span>
                {"\n"}
                <span className="dim">do this instead: leave this file alone; if the change is genuinely needed, make it by hand</span>
              </p>
              <p><span className="who">agent  </span>edit greet.js <span className="ok">✓</span></p>
              <p>
                <span className="who">agent  </span>I couldn&apos;t add API_KEY to .env. That path is protected, so you&apos;ll
                need to create or edit it yourself.
              </p>
            </div>
            <p className="term-caption">A real Cursor session, shortened.</p>
          </div>
        </div>
      </section>

      <section className="block">
        <div className="wrap">
          <p className="eyebrow">The problem</p>
          <h2>An agent that goes off task does not crash.</h2>
          <p className="lede">
            It edits files you never mentioned. It installs a package nobody asked for. It ignores the rules file
            it read an hour ago. A support agent changes a credit limit it was told never to touch. And the build
            stays green, because tests check that code does what it was written to do, not whether it is the code
            you asked for.
          </p>
          <p className="lede" style={{ marginBottom: 0 }}>Nothing in the toolchain is watching for that. trackline is.</p>
        </div>
      </section>

      <section className="block">
        <div className="wrap">
          <p className="eyebrow">How it works</p>
          <h2>One engine, two places.</h2>
          <div className="steps">
            <div className="card">
              <div className="step-n">01</div>
              <h3>It watches</h3>
              <p>Beside a coding agent, a hook sees every action before it runs. In production, it reads the traces your agent already sends.</p>
            </div>
            <div className="card">
              <div className="step-n">02</div>
              <h3>It compares</h3>
              <p>Each action is checked against what you asked for, your rules files and a tool policy. Most checks are plain rules, not a model.</p>
            </div>
            <div className="card">
              <div className="step-n">03</div>
              <h3>It tells you, or the agent</h3>
              <p>By default it only writes things down. You choose, check by check, whether it should also stop the agent and explain why, so the agent can correct itself.</p>
            </div>
          </div>
        </div>
      </section>

      <section className="block">
        <div className="wrap">
          <p className="eyebrow">What it catches</p>
          <h2>Six checks, and one question only a model can answer.</h2>
          <div className="table-scroll">
            <table>
              <tbody>
                {checks.map(([name, what]) => (
                  <tr key={name}>
                    <td><strong>{name}</strong></td>
                    <td>{what}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="note">The checks are arithmetic and never guess. The judge is a model, measured before it was trusted, and off until you turn it on.</p>
        </div>
      </section>

      <section className="block">
        <div className="wrap">
          <p className="eyebrow">Where it works</p>
          <h2>One set of rules for every agent you use.</h2>
          <p className="lede">A vendor can govern its own agent. Only something that belongs to none of them can govern all of them the same way.</p>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th />
                  {hosts.map((h) => <th key={h}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.label}>
                    <td>{r.label}</td>
                    {r.cells.map((c, i) => (
                      <td key={i} className={c.startsWith("no") ? "no" : undefined}>{c}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="note"><code>trackline doctor --host &lt;name&gt;</code> prints the full list of what it can and cannot see in each.</p>
        </div>
      </section>

      <section className="block">
        <div className="wrap">
          <p className="eyebrow">What it cannot do</p>
          <h2>Where it sees less.</h2>
          <p className="lede">Named plainly, because a tool that looks complete stops getting better.</p>
          <ul className="limits">
            <li><strong>In production it alerts, it cannot stop.</strong> A trace is a record of what already happened.</li>
            <li><strong>Production needs content capture on.</strong> Without it, a trace does not say which tool was called, and trackline reports that rather than guessing.</li>
            <li><strong>Scope stays quiet when your request names no file.</strong> It will not invent a scope you did not state.</li>
            <li><strong>Through MCP, the agent chooses whether to ask.</strong> An agent that does not ask is not watched.</li>
            <li><strong>Production has not yet met real traffic.</strong> It was tested on a stream sent by the real OpenTelemetry libraries, with scripted conversations.</li>
          </ul>
        </div>
      </section>

      <section className="block">
        <div className="wrap">
          <p className="eyebrow">The evidence</p>
          <h2>Every claim here was measured first.</h2>
          <div className="stats">
            {stats.map((s) => (
              <a key={s.big} className="stat" href={`${EXPERIMENTS}/${s.file}`}>
                <b>{s.big}</b>
                {s.small}
              </a>
            ))}
          </div>
          <p className="note"><Link href="/evidence">See all seven experiments →</Link></p>
        </div>
      </section>

      <section className="block">
        <div className="wrap">
          <p className="eyebrow">Install</p>
          <h2>Two commands.</h2>
          <pre className="block-code"><code>{`npm install -g trackline
trackline init              # Claude Code; --host codex or --host cursor`}</code></pre>
          <p className="lede">
            It starts in warn mode: it notices things and writes them down, and never interrupts you. Run{" "}
            <code>trackline status</code> after your agent&apos;s next edit to see that it fired.
          </p>
          <Link href="/docs/install">Full install guide →</Link>
        </div>
      </section>
    </>
  );
}
