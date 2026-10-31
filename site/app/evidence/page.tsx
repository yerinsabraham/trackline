import Link from "next/link";
import { experiments } from "@/lib/experiments";
import { LIMITS } from "@/lib/limits";
import { pageMeta } from "@/lib/seo";

export const metadata = pageMeta({
  path: "/evidence",
  title: "Evidence",
  description: `${experiments().length} experiments behind trackline's design, each measured before it was built on, with the method and the limits.`,
});

const day = (d: string) => new Date(d + "T12:00:00Z").toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });

export default function Evidence() {
  return (
    <div className="wrap inner">
      <header className="page-head">
        <p className="eyebrow">Evidence</p>
        <h1>Every claim here was measured first.</h1>
        <p>
          trackline&apos;s design rests on a few claims that would be expensive to get wrong. Each was measured
          before it was built on. Where a result has limits, the write-up names them next to the number.
        </p>
      </header>
      <div className="evidence">
        {experiments().map((e) => (
          <article key={e.n} className="ev">
            <div className="num">{String(e.n).padStart(2, "0")}</div>
            <div>
              <h3>{e.question}</h3>
              <p className="ans">{e.answer}</p>
              <p className="method">{e.method}</p>
              <p className="dated">Published {day(e.published)}</p>
            </div>
            <Link className="read" href={`/evidence/${e.slug}`}>Read the write-up →</Link>
          </article>
        ))}
      </div>
      <section id="limits" className="evidence-limits">
        <h2>What it cannot do yet.</h2>
        <p>Named plainly, because a tool that looks complete stops getting better.</p>
        <dl>
          {LIMITS.map(([what, why]) => (
            <div key={what}><dt>{what}</dt><dd>{why}</dd></div>
          ))}
        </dl>
      </section>
    </div>
  );
}
