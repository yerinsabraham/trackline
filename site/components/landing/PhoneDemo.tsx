import { ClaudeLogo } from "@/components/HostLogos";

// The phone app, drawn: a task sent, the agent at work, one action stopped
// and the choice it leaves you. Drawn rather than a screenshot so it stays
// sharp and carries nobody's data. Every piece is something the app shows.

export default function PhoneDemo() {
  return (
    <div className="pd" aria-label="The trackline app on a phone: a task, the agent's steps, one stopped action with Allow once and Keep blocked, and the agent's reply" role="img">
      <div className="pd-screen">
        <div className="pd-head">
          <span className="pd-logo"><ClaudeLogo /></span>
          <span className="pd-title"><b>Fix the contact form</b><i>Claude Code · working</i></span>
          <span className="pd-stop">Stop</span>
        </div>
        <div className="pd-body">
          <div className="pd-me">Make the email required, and show the error under the field.</div>
          <div className="pd-term">
            <div className="pd-term-head"><span>Activity</span><span className="pd-live">live</span></div>
            <div className="pd-line"><span className="ok">Read</span>ContactForm.tsx</div>
            <div className="pd-line"><span className="ok">Edit</span>lib/validate.ts <span className="ok">+8</span></div>
            <div className="pd-line"><span className="bad">Blocked</span>.env.local</div>
          </div>
          <div className="pd-stopped">
            <b>Stopped: writing to a protected path</b>
            <span>Secrets are off limits from the phone. Claude Code was told why.</span>
            <div className="pd-actions"><span className="pd-allow">Allow once</span><span className="pd-keep">Keep blocked</span></div>
          </div>
          <div className="pd-reply">
            <span className="pd-logo small"><ClaudeLogo /></span>
            <p>The email is now <b>required</b>, and the error shows under it. I left <code>.env.local</code> alone.</p>
          </div>
        </div>
        <div className="pd-compose"><span>Reply to Claude Code…</span><i>↑</i></div>
      </div>
    </div>
  );
}
