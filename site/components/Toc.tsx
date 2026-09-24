import type { Heading } from "@/lib/markdown";

export default function Toc({ toc }: { toc: Heading[] }) {
  if (toc.length < 2) return <aside className="toc" />;
  return (
    <aside className="toc" aria-label="On this page">
      <p className="eyebrow">On this page</p>
      {toc.map((h) => (
        <a key={h.id} href={`#${h.id}`} className={h.depth === 3 ? "sub" : undefined}>{h.text}</a>
      ))}
    </aside>
  );
}
