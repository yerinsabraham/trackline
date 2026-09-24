import Link from "next/link";
import { notFound } from "next/navigation";
import CodeCopy from "@/components/CodeCopy";
import DocTools from "@/components/DocTools";
import Toc from "@/components/Toc";
import { page, pages } from "@/lib/guide";
import { pageMeta } from "@/lib/seo";

export const dynamicParams = false;

export function generateStaticParams() {
  return pages().map((p) => ({ slug: p.slug }));
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }) {
  const p = page((await params).slug);
  return p ? pageMeta({ path: `/docs/${p.slug}`, title: p.title, description: p.description }) : {};
}

export default async function DocPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const all = pages();
  const i = all.findIndex((p) => p.slug === slug);
  if (i < 0) notFound();
  const p = all[i];
  const prev = all[i - 1];
  const next = all[i + 1];

  return (
    <div className="wrap docs has-toc">
      <nav className="docs-nav" aria-label="Docs">
        <p className="eyebrow">Docs</p>
        {all.map((x) => (
          <Link key={x.slug} href={`/docs/${x.slug}`} className={x.slug === slug ? "active" : undefined}>
            {x.title}
          </Link>
        ))}
      </nav>
      <article className="prose">
        <h1>{p.title}</h1>
        {p.description && <p className="desc">{p.description}</p>}
        <DocTools md={`/docs/${p.slug}.md`} title={p.title} />
        <div dangerouslySetInnerHTML={{ __html: p.html }} />
        <CodeCopy />
        <div className="pager">
          {prev && (
            <Link href={`/docs/${prev.slug}`}>
              <small>Previous</small>← {prev.title}
            </Link>
          )}
          {next && (
            <Link href={`/docs/${next.slug}`} className="next">
              <small>Next</small>{next.title} →
            </Link>
          )}
        </div>
      </article>
      <Toc toc={p.toc} />
    </div>
  );
}
