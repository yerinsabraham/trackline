import Link from "next/link";
import { experiments } from "@/lib/experiments";
import { pageMeta } from "@/lib/seo";

export const metadata = pageMeta({
  path: "/about",
  title: "About",
  description: "Who builds trackline, where it came from, and why it belongs to no agent vendor.",
});

// Facts only, each one also stated in the repository's README. A founder story
// that cannot be checked is worse than none.
export default function About() {
  return (
    <div className="wrap inner">
      <header className="page-head">
        <p className="eyebrow">About</p>
        <h1>A neutral layer for every AI agent.</h1>
        <p>
          trackline checks whether an agent is still doing what it was asked, the same way in every agent a team uses.
        </p>
      </header>

      <div className="about">
        <section>
          <h2>Where it came from</h2>
          <p>
            trackline grew out of the evaluation harness for{" "}
            <a className="text-link" href="https://liraintelligence.com">Lira Intelligence</a>, a production AI support
            agent with retrieval over customer knowledge bases and risk-tiered tool calling. The principles it carries
            came from there: everything that can be checked by code is checked by code, safety rules get no tolerance,
            and a check that errors fails loudly instead of passing quietly.
          </p>
        </section>

        <section>
          <h2>Why neutral</h2>
          <p>
            A vendor can govern its own agent. Only something that belongs to none of them can govern all of them the
            same way. trackline is neutral by construction, not by promise: the engine is open source, it runs on your
            machine, it treats Claude Code, Codex, Cursor and any MCP client alike, and no vendor&apos;s model sits in
            the path. Without an account it sends nothing anywhere.
          </p>
        </section>

        <section>
          <h2>How it is built</h2>
          <p>
            Every claim on this site was measured before it was built on, and the{" "}
            <Link className="text-link" href="/evidence">{experiments().length} experiments</Link> are published with their method and
            limits. The part that runs before every action is written in Go, because it runs on every tool call:
            starting Node cost 88 ms a call against 6.5 ms for Go, and the whole check runs in 11 to 14 ms.
          </p>
        </section>

        <section>
          <h2>Who builds it</h2>
          <p>
            trackline is built by <a className="text-link" href="https://yerinsabraham.com">Yerins Abraham</a>. The
            longer story of the problem and the research behind it is in{" "}
            <a className="text-link" href="https://yerinsabraham.com/engineering/nothing-notices-when-an-agent-drifts">
              Nothing notices when an agent drifts
            </a>.
          </p>
          <p>
            Questions, or running it across a team: <b>hello@trackline.dev</b>
          </p>
        </section>
      </div>
    </div>
  );
}
