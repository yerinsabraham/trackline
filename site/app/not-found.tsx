import Link from "next/link";

export const metadata = { title: "Not found", robots: { index: false } };

export default function NotFound() {
  return (
    <div className="wrap inner not-found">
      <p className="eyebrow">404</p>
      <h1>This page went off track.</h1>
      <p>There is nothing at this address. It may have moved, or the link may be wrong.</p>
      <div className="actions">
        <span className="frame"><Link className="btn btn-quiet" href="/docs"><span className="plus">+</span>Docs</Link></span>
        <span className="frame"><Link className="btn btn-signal" href="/"><span className="plus">+</span>Home</Link></span>
      </div>
    </div>
  );
}
