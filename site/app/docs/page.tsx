import Link from "next/link";
import { pages } from "@/lib/guide";

export const metadata = { title: "Docs" };

export default function DocsIndex() {
  return (
    <div className="wrap" style={{ padding: "48px 20px 20px" }}>
      <h1 style={{ fontSize: 40, letterSpacing: "-0.02em", margin: "0 0 8px" }}>Docs</h1>
      <p className="lede">Everything trackline does, one page per task.</p>
      <div className="index-grid">
        {pages().map((p) => (
          <Link key={p.slug} href={`/docs/${p.slug}`} className="card">
            <h3>{p.title}</h3>
            <p>{p.description}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
