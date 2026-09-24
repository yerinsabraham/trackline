import Link from "next/link";
import { pages } from "@/lib/guide";

export const metadata = { title: "Docs" };

export default function DocsIndex() {
  return (
    <div className="wrap inner">
      <header className="page-head">
        <p className="eyebrow">Docs</p>
        <h1>Everything trackline does.</h1>
        <p>One page per task, from the first install to watching an agent in production.</p>
      </header>
      <div className="index-grid">
        {pages().map((p) => (
          <Link key={p.slug} href={`/docs/${p.slug}`}>
            <h3>{p.title}<span>→</span></h3>
            <p>{p.description}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
