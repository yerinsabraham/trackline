import { pageMeta } from "@/lib/seo";

export const metadata = pageMeta({
  path: "/terms",
  title: "Terms",
  description: "The terms for using trackline and its optional account.",
});

export default function Terms() {
  return (
    <div className="wrap docs">
      <div />
      <article className="prose">
        <h1>Terms</h1>
        <p className="desc">The terms for using trackline and its optional account. Last updated 24 September 2026.</p>

        <h2>The software</h2>
        <p>
          The trackline command-line tool is open source under the{" "}
          <a href="https://github.com/yerinsabraham/trackline/blob/main/LICENSE">Apache License 2.0</a>. Those terms govern the
          software; nothing here narrows them.
        </p>

        <h2>The account</h2>
        <ul>
          <li>An account is optional. You need a GitHub or Google account to create one.</li>
          <li>You are responsible for the machines you connect and for what they send. Connect only projects you are allowed to share this information about.</li>
          <li>Do not use the service to attack it or others, to get around its limits, or to reach accounts that are not yours.</li>
          <li>You can delete your account at any time. We may close an account that breaks these terms, and will say why.</li>
        </ul>

        <h2>What trackline is, and is not</h2>
        <p>
          trackline watches what AI agents do and tells you when it looks off. It is an aid, not a guarantee. It can miss
          things, and the <a href="/docs/limits">limits page</a> says where it sees less. It does not replace reviewing what an
          agent did before you rely on it.
        </p>

        <h2>No warranty</h2>
        <p>
          The service is provided as it is, without warranties of any kind. To the extent the law allows, we are not liable for
          losses arising from using it or from relying on what it reports.
        </p>

        <h2>Paid plans</h2>
        <p>There are none yet. When there are, their terms will be shown before you pay, and nothing free today becomes paid without notice.</p>

        <h2>Changes</h2>
        <p>
          If these terms change in a way that matters, we will say so on this page and, if you have an account, tell you before
          the change applies.
        </p>

        <h2>Contact</h2>
        <p><a href="mailto:hello@trackline.dev">hello@trackline.dev</a></p>
      </article>
    </div>
  );
}
