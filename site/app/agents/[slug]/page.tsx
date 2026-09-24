import Link from "next/link";
import { notFound } from "next/navigation";
import CopyButton from "@/components/CopyButton";
import { agent, agents } from "@/lib/hosts";
import { pageMeta } from "@/lib/seo";

export const dynamicParams = false;

export function generateStaticParams() {
  return agents().map((a) => ({ slug: a.slug }));
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }) {
  const a = agent((await params).slug);
  return a ? pageMeta({ path: `/agents/${a.slug}`, title: `trackline in ${a.title}`, description: a.summary }) : {};
}

// Everything in the table comes from the engine's own record of each host
// (engine/internal/hosts), so this page cannot claim more than trackline does.
export default async function AgentPage({ params }: { params: Promise<{ slug: string }> }) {
  const a = agent((await params).slug);
  if (!a) notFound();
  const all = agents();
  const setup = a.setup.join("\n");

  return (
    <div className="wrap docs">
      <nav className="docs-nav" aria-label="Agents">
        <p className="eyebrow">Agents</p>
        {all.map((x) => (
          <Link key={x.slug} href={`/agents/${x.slug}`} className={x.slug === a.slug ? "active" : undefined}>{x.title}</Link>
        ))}
      </nav>
      <article className="prose">
        <h1>trackline in {a.title}</h1>
        <p className="desc">{a.summary}</p>

        <div className="table-card fact-table">
          <table>
            <tbody>
              <tr><td>Can stop an action before it happens</td><td className={a.blocks ? "yes" : "no"}>{a.blocks ? "yes" : "no"}</td></tr>
              <tr><td>Tells the agent why</td><td className={a.reasonReachesAgent ? "yes" : "no"}>{a.reasonReachesAgent ? "yes" : "no"}</td></tr>
              <tr><td>Knows what you asked</td><td>{a.intent}</td></tr>
              <tr><td>Established by</td><td>{a.evidence}</td></tr>
            </tbody>
          </table>
        </div>

        <h2 id="setup">Set it up</h2>
        <div className="setup-block">
          <pre><code>{setup}</code></pre>
          <CopyButton text={setup} />
        </div>
        <p>
          Or paste the prompt from the <Link href="/#install">install section</Link> into your agent. The full steps are
          in <Link href={`/docs/${a.guide}`}>the guide</Link>.
        </p>

        <h2 id="limits">Where it sees less</h2>
        <ul>
          {a.limits.map((l) => <li key={l}>{l[0].toUpperCase() + l.slice(1)}.</li>)}
        </ul>
        <p>
          Named plainly, because a tool that looks complete stops getting better.{" "}
          {a.key !== "production" && (
            <><code>trackline doctor --host {a.key}</code> prints this list on your own machine.</>
          )}
        </p>
      </article>
    </div>
  );
}
