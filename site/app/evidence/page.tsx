import { EXPERIMENTS, evidence } from "@/lib/site";

export const metadata = { title: "Evidence", alternates: { canonical: "/evidence" } };

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
        {evidence.map((e) => (
          <article key={e.n} className="ev">
            <div className="num">{String(e.n).padStart(2, "0")}</div>
            <div>
              <h3>{e.question}</h3>
              <p className="ans">{e.answer}</p>
              <p className="method">{e.method}</p>
            </div>
            <a className="read" href={`${EXPERIMENTS}/${e.file}`}>Read the write-up →</a>
          </article>
        ))}
      </div>
    </div>
  );
}
