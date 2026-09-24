import Link from "next/link";
import { agents } from "@/lib/hosts";
import { pageMeta } from "@/lib/seo";

export const metadata = pageMeta({
  path: "/agents",
  title: "Agents",
  description: "What trackline can and cannot do in Claude Code, Codex, Cursor, any MCP client and production traces, as measured.",
});

const yes = (b: boolean) => (b ? "yes" : "no");

export default function Agents() {
  const all = agents();
  return (
    <div className="wrap inner">
      <header className="page-head">
        <p className="eyebrow">Agents</p>
        <h1>One set of rules for every agent you use.</h1>
        <p>
          A vendor can govern its own agent. Only something that belongs to none of them can govern all of them the
          same way. Each page says what trackline sees in that agent, and where it sees less.
        </p>
      </header>
      <div className="index-grid">
        {all.map((a) => (
          <Link key={a.slug} href={`/agents/${a.slug}`}>
            <h3>{a.title}<span>→</span></h3>
            <p>{a.summary}</p>
            <p className="facts">
              Stops an action: <b>{yes(a.blocks)}</b> · Tells the agent why: <b>{yes(a.reasonReachesAgent)}</b>
            </p>
          </Link>
        ))}
      </div>
    </div>
  );
}
