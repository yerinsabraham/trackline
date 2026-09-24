import Link from "next/link";
import { notFound } from "next/navigation";
import { page, pages } from "@/lib/guide";

export function generateStaticParams() {
  return pages().map((p) => ({ slug: p.slug }));
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }) {
  const p = page((await params).slug);
  return p ? { title: p.title, description: p.description } : {};
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
    <div className="wrap docs">
      <nav className="docs-nav" aria-label="Docs">
        {all.map((x) => (
          <Link key={x.slug} href={`/docs/${x.slug}`} className={x.slug === slug ? "active" : undefined}>
            {x.title}
          </Link>
        ))}
      </nav>
      <article className="prose">
        <h1>{p.title}</h1>
        {p.description && <p className="desc">{p.description}</p>}
        <div dangerouslySetInnerHTML={{ __html: p.html }} />
        <div className="pager">
          <span>{prev && <Link href={`/docs/${prev.slug}`}>← {prev.title}</Link>}</span>
          <span>{next && <Link href={`/docs/${next.slug}`}>{next.title} →</Link>}</span>
        </div>
      </article>
    </div>
  );
}
