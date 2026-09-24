import { releases } from "@/lib/changelog";
import { pageMeta } from "@/lib/seo";

export const metadata = pageMeta({
  path: "/changelog",
  title: "Changelog",
  description: "What changed in each release of trackline.",
});

const day = (d: string) => new Date(d + "T12:00:00Z").toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric" });

export default function Changelog() {
  return (
    <div className="wrap inner">
      <header className="page-head">
        <p className="eyebrow">Changelog</p>
        <h1>What changed in each release.</h1>
        <p>
          Newest first. Update with <code>npm install -g trackline@latest</code>. Follow it by <a className="text-link" href="/changelog.xml">RSS</a>.
        </p>
      </header>
      <div className="releases">
        {releases().map((r) => (
          <section key={r.version} className="release" id={`v${r.version}`}>
            <div className="release-meta">
              <h2>{r.version}</h2>
              <p>{day(r.date)}</p>
            </div>
            <div className="prose" dangerouslySetInnerHTML={{ __html: r.html }} />
          </section>
        ))}
      </div>
    </div>
  );
}
