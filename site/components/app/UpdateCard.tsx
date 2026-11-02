"use client";

import { updates } from "@/lib/dashboard";
import { agentName } from "./AgentLogo";
import { useApp } from "./AppShell";
import { Cmd } from "./FirstRun";
import { IconLaptop } from "./icons";

// Nothing on the computer checks for updates by itself, so this is where an
// old install learns of a new one, and where a project an upgrade left on
// older hooks gets the one step that fixes it. Nothing here changes the
// computer: a watcher that rewrote its own settings unasked would be the
// thing it exists to catch.

export default function UpdateCard() {
  const { projects, latest, remote } = useApp();
  const u = updates(projects, latest);
  if (!u.release && u.projects.length === 0) return null;
  // init changes files phone messages are checked against, so a project the
  // phone reaches is refused until it is agreed to again at the computer.
  const reached = new Set((remote?.machines ?? []).flatMap((m) => m.projects.map((p) => p.name)));
  return (
    <section className="card update-card" aria-labelledby="update">
      <div className="card-head">
        <h2 id="update">Update needed</h2>
        <span className="update-where"><IconLaptop size={16} />On your computer</span>
      </div>
      {u.release && (
        <div className="update-step">
          <p>Trackline {u.release.latest} is out. Your computer has {u.release.have}.</p>
          <Cmd text="npm install -g trackline@latest" />
        </div>
      )}
      {u.projects.map((p) => (
        <div key={p.id} className="update-step">
          <p>In {p.name}, {p.behind.map(agentName).join(" and ")} {p.behind.length === 1 ? "is" : "are"} missing new hooks.</p>
          <Cmd text={reached.has(p.name) ? "trackline init && trackline remote enable" : "trackline init"} />
        </div>
      ))}
    </section>
  );
}
