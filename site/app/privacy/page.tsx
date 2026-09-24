import { pageMeta } from "@/lib/seo";

export const metadata = pageMeta({
  path: "/privacy",
  title: "Privacy",
  description: "What trackline collects, what it never collects, and how to delete it.",
});

// Written from what the product actually does. If behaviour changes, this page
// changes in the same commit.
export default function Privacy() {
  return (
    <div className="wrap docs">
      <div />
      <article className="prose">
        <h1>Privacy</h1>
        <p className="desc">What trackline collects, what it never collects, and how to delete it. Last updated 24 September 2026.</p>

        <h2>The short version</h2>
        <ul>
          <li>The trackline command-line tool runs on your machine and sends nothing anywhere unless you connect it to an account.</li>
          <li>This website sets no tracking cookies and runs no analytics.</li>
          <li>If you connect an account, trackline sends what it <em>noticed</em> about your agent&apos;s actions. Never your code, never your file contents, never the text of your commands.</li>
        </ul>

        <h2>Using trackline without an account</h2>
        <p>
          Everything stays on your computer, in a <code>.trackline</code> folder inside each project. Nothing is sent to us or to
          anyone else. If you turn on the optional judge, trackline sends your request and a one-line description of each action to
          the model provider <em>you</em> chose; that goes directly from your machine to them, not through us.
        </p>

        <h2>If you create an account</h2>
        <p>Accounts are optional and add a live dashboard. When you sign in we receive, from GitHub or Google:</p>
        <ul>
          <li>your name, email address and profile picture;</li>
          <li>an identifier for your GitHub or Google account, so you can sign in again.</li>
        </ul>
        <p>We never receive your GitHub or Google password, and we do not ask for access to your repositories.</p>

        <h2>What a connected machine sends</h2>
        <p>Only after you run <code>trackline connect</code>, and only for projects you choose:</p>
        <table>
          <thead><tr><th>Sent</th><th>Never sent</th></tr></thead>
          <tbody>
            <tr><td>the project&apos;s folder name, and a random id for it</td><td>file contents, diffs or patches</td></tr>
            <tr><td>which agent, and a session id</td><td>the text of shell commands</td></tr>
            <tr><td>the kind of each action: edit, write, delete, read, command, tool call</td><td>tool arguments</td></tr>
            <tr><td>file paths, relative to the project</td><td>anything outside the project</td></tr>
            <tr><td>names of packages installed</td><td>environment variables</td></tr>
            <tr><td>what each check concluded, and why</td><td></td></tr>
            <tr><td>what you asked your agent, if you agree when connecting</td><td></td></tr>
          </tbody>
        </table>
        <p>
          <strong>File paths can say more than they seem to.</strong> A name like <code>clients/acme/merger.md</code> is sent as
          written. Do not connect projects whose file names you would not want stored.
        </p>
        <p>
          <strong>What you ask your agent</strong> is sent only if you say yes when connecting, and you can turn it off at any time
          with <code>trackline connect --no-requests</code>.
        </p>

        <h2>Production traces</h2>
        <p>
          If you connect <code>trackline serve</code>, it sends results only: tool names, their outcomes, findings, counts and
          timings. It never sends the traces themselves, your customers&apos; messages, model replies, tool arguments or system
          prompts, unless your team explicitly turns that on.
        </p>

        <h2>Where it is kept, and for how long</h2>
        <ul>
          <li>Account data and events are stored on servers in the United States.</li>
          <li>Events are kept for 30 days on the free plan, then deleted.</li>
          <li>The website is hosted by Vercel. Sign-in is provided by GitHub and Google under their own privacy policies.</li>
          <li>We do not sell your data, share it with advertisers, or use it to train models.</li>
        </ul>

        <h2>Deleting it</h2>
        <p>
          Deleting your account from the account page deletes your account and every event we hold for it. You can disconnect a
          machine at any time, from the dashboard or with <code>trackline disconnect</code>, and it stops sending immediately.
        </p>

        <h2>Contact</h2>
        <p>
          Questions, or a request to see or delete your data: <a href="mailto:hello@trackline.dev">hello@trackline.dev</a>.
        </p>
      </article>
    </div>
  );
}
