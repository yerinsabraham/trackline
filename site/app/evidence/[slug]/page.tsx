import Link from "next/link";
import { notFound } from "next/navigation";
import CodeCopy from "@/components/CodeCopy";
import DocTools from "@/components/DocTools";
import Toc from "@/components/Toc";
import { experiment, experiments } from "@/lib/experiments";
import { pageMeta } from "@/lib/seo";
import { GITHUB } from "@/lib/site";

export const dynamicParams = false;

export function generateStaticParams() {
  return experiments().map((e) => ({ slug: e.slug }));
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }) {
  const e = experiment((await params).slug);
  return e ? pageMeta({ path: `/evidence/${e.slug}`, title: e.title, description: `${e.answer}. ${e.method}` }) : {};
}

const day = (d: string) => new Date(d + "T12:00:00Z").toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric" });

export default async function Experiment({ params }: { params: Promise<{ slug: string }> }) {
  const e = experiment((await params).slug);
  if (!e) notFound();
  const all = experiments();
  const i = all.findIndex((x) => x.slug === e.slug);

  return (
    <div className="wrap docs has-toc">
      <nav className="docs-nav" aria-label="Experiments">
        <p className="eyebrow">Evidence</p>
        {all.map((x) => (
          <Link key={x.slug} href={`/evidence/${x.slug}`} className={x.slug === e.slug ? "active" : undefined}>
            {String(x.n).padStart(2, "0")} · {x.question}
          </Link>
        ))}
      </nav>
      <article className="prose">
        <p className="eyebrow">Experiment {String(e.n).padStart(2, "0")} · Published {day(e.published)}</p>
        <h1>{e.title}</h1>
        <p className="desc">{e.answer}. {e.method}</p>
        <DocTools md={`/evidence/${e.slug}.md`} title={e.title} />
        <div dangerouslySetInnerHTML={{ __html: e.html }} />
        <CodeCopy />
        <p className="source-note">
          The code and raw data behind this result are in the repository:{" "}
          <a href={`${GITHUB}/tree/main/docs/experiments`}>docs/experiments</a>.
        </p>
        <div className="pager">
          {all[i - 1] && <Link href={`/evidence/${all[i - 1].slug}`}><small>Previous</small>← {all[i - 1].question}</Link>}
          {all[i + 1] && <Link href={`/evidence/${all[i + 1].slug}`} className="next"><small>Next</small>{all[i + 1].question} →</Link>}
        </div>
      </article>
      <Toc toc={e.toc} />
    </div>
  );
}
