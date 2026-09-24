import { EXPERIMENTS, evidence } from "@/lib/site";

export const metadata = { title: "Evidence" };

export default function Evidence() {
  return (
    <div className="wrap" style={{ padding: "48px 20px 20px" }}>
      <h1 style={{ fontSize: 40, letterSpacing: "-0.02em", margin: "0 0 8px" }}>Evidence</h1>
      <p className="lede">
        trackline&apos;s design rests on a few claims that would be expensive to get wrong. Each was measured
        before it was built on. Where a result has limits, the write-up names them next to the number.
      </p>
      <div className="evidence">
        {evidence.map((e) => (
          <div key={e.n} className="card ev">
            <div className="num">{String(e.n).padStart(2, "0")}</div>
            <div>
              <h3>{e.question}</h3>
              <p className="ans">{e.answer}</p>
              <p className="method">{e.method}</p>
              <a href={`${EXPERIMENTS}/${e.file}`}>Read the write-up →</a>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
