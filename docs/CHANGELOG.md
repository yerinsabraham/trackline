# Changelog

What changed in each release of `trackline`, newest first. The website's
changelog page is built from this file.

## 0.15.0 · 2026-11-02

- **An update says so.** The app tells you when a newer trackline is out, and
  when a project is missing hooks a release added, with the one command that
  fixes it. Nothing is changed on your computer without you.
- **The phone runner picks up an update by itself.** After
  `npm install -g trackline@latest`, the runner started at login restarts on
  the new release once no agent it started is still working. Before, it kept
  the old one until you next logged in.
- `trackline status` names any agent in the project set up by an older release.

## 0.14.0 · 2026-11-01

Run `trackline init` in each project to pick up the new hooks, then
`trackline remote enable` there. Codex asks you to trust its hooks again.

- **Your message stays yours.** When an agent's memory filled up, the summary it
  writes for itself replaced your last message in the chat, and the checks read
  it as your request. They now ignore it, and old chats hide it.
- **A chat keeps up with the computer.** A message you send in Claude Code or
  Codex shows as soon as you send it, and you can read what the agent is saying
  while it works, not only when it finishes.
- **Steps say what they do.** A command now shows the agent's own line about it
  where Claude Code writes one, instead of "Bash". The full command stays on your
  computer unless you run `trackline share commands on` in a project.
- **Waiting for you.** When an agent stops on a permission prompt, the chat says
  so and your phone tells you.
- **Quiet while you are looking.** A chat you have open does not also send a
  notification.
- **A phone message can reach a Claude Code chat that is open.** Run
  `trackline remote enable --open-chats`. The message waits for the agent's turn
  to end, or wakes the chat if it is idle, and runs with that chat's own
  permissions. It cannot reach a chat that has not finished a turn since this
  was installed.
- **Long sessions stopped slowing every step.** The hook read the whole session
  log before each action, about a second on a 178 MB log. It now reads the end.

## 0.13.0 · 2026-10-31

- **Agents are individuals.** Every reply and chat shows its agent's name in
  bold, with the model that answered. Tap a name to rename it, add it to your
  team, or move a chat to someone already on it.
- **Chats, Team, Settings.** Home and Sessions are now one list, with what
  needs you first. Team holds the agents you chose to keep, each with a profile
  of what it has actually done. Helpers show as one line in the chat.
- **Positions.** Give a team member a title and a description, such as CTO, and
  it holds that job across a resumed chat in Claude Code and Codex. It reaches
  the agent with messages sent from trackline, and it never changes what the
  agent may do.
- **One agent can ask another.** An agent you started from trackline can ask a
  team member to do something. You read the whole request and approve it with
  your passkey, and the result can go back to the asker once. A handoff that
  changes the files telling agents how to behave fails, however it did so.
- **A reply never lands on a chat open on your computer.** A Claude Code chat
  that is open stays yours: the phone says so and sends nothing. A Codex chat
  open in a terminal can take the message if you run
  `trackline remote enable --open-chats`. It then runs with that chat's own
  permissions, not the narrower ones of a phone run.

## 0.12.0 · 2026-10-31

- **Reply from your phone in every project.** Pair your phone once, during
  `trackline connect`, and every project you connect takes messages, with no
  command per project. Every message still needs your passkey, and your laptop
  still checks it. **On a laptop already paired, run `trackline remote enable`
  once in a terminal after updating**; you get one email listing the projects.
  `trackline remote disable` turns a project off, and it stays off.
- **The reply box is never locked.** You can always type, and a line underneath
  says what would let it send.
- **One conversation.** A reply from your phone continues on the same screen,
  with the history above it, and a notification opens there. Your messages stay
  in place, including ones an agent answered in words alone. Long chats load
  earlier messages as you scroll up, and coming back opens at a "New messages"
  line. Any running task has a Stop button.
- **Sessions have names.** A session takes the agent's own title, or its first
  real request, and keeps it. Rename, pin and archive from the three dots.
- **Delete and clear.** Swipe left or use the three dots, with six seconds to
  undo. Hold a row to choose several. Clearing a chat keeps its name. Both remove
  the words from trackline only, never from your laptop, and your on-track
  numbers do not change.
- **Search** names and replies, filter by project, and see every session rather
  than the latest 50.
- **Feels like an app.** A phone opens on your chats, with what needs you first
  and unread ones marked. Tabs change instantly, the app follows your phone's
  dark mode, the keyboard stays out of the way, and the icon shows a count.
  A turn's work reads as one line, like "Edited 4 files, Ran 2 commands", and
  opens to the steps. With no signal the app still opens on the last list.
- **Live.** The app updates the moment something changes, and shows what the
  agent is saying while it works. It uses less of the server, too.
- **A sleeping laptop no longer loses your message.** A message sent while the
  laptop is asleep runs when it wakes, for up to 12 hours, once, and Stop cancels
  it. A message with images still needs the laptop awake. While a task runs, a
  Mac stays awake unless its lid is closed.
- **Write while the agent works.** Your message waits in the chat and Face ID
  sends it when the agent finishes.
- **Longer replies.** The limit goes from 8,000 to 32,000 bytes, and a reply cut
  at the limit says so.
- **A project in use at your desk no longer pauses after 30 days.**
- **`trackline remote enable` needs a real terminal.** It accepts changed
  settings and lifts a stop made from the site, so only a person may run it. A
  stop made from the site now also survives the runner restarting at login.
- **`trackline connect` never shares without you.** Run by an agent with no
  terminal, it keeps the answer you gave before, or does not share.

## 0.11.0 · 2026-09-27

- **Send images from your phone.** Attach up to four screenshots or photos to
  a task or a reply; the agent on your laptop looks at them first. Each
  image's hash is signed with your passkey, and the laptop takes only images
  that match, so the server that carries them cannot swap one. They are saved
  in the project under `.trackline/attachments/`, out of git, and the server
  keeps them only until the laptop has them.

## 0.10.0 · 2026-09-26

- **Cursor from your phone.** Tasks, replies, and Allow once or Keep blocked
  now work with Cursor as they do with Claude Code and Codex. It runs headless
  with its sandbox on and never with `--force`.
- **A task from your phone cannot write outside its project**, whatever the
  agent. Measured: Cursor's file editor, unlike its shell, is not confined by
  its sandbox, and a mistyped path wrote a file elsewhere on the laptop.
- **Fix:** an approval now matches a project reached through a symbolic link,
  such as `/tmp` on macOS.

## 0.9.0 · 2026-09-26

- **Deployed agents on the dashboard.** `trackline serve --connect` sends
  each checked conversation to your account, so a production agent appears
  under Production beside your coding agents, and a broken policy or an
  incident alerts your phone. What is sent is what trackline concluded: tool
  names, findings, incidents and counts. Never what a customer or the model
  said. `trackline traces --connect` sends a recorded stream the same way.

## 0.8.0 · 2026-09-26

- **A new check: `workaround`.** After trackline blocks a file, the same file
  reached another way: a script that writes it, a build step, a command. The
  finding shows the blocked call and the route taken after it, side by side.
- **`test-weakened` shows what changed.** `trackline show` lists the test's
  lines as they were and as they are, so a test that really needed updating
  can be told from one that was made to stop failing.

## 0.7.0 · 2026-09-26

- **A new check: `test-weakened`.** A test skipped, or its assertions removed
  or loosened, instead of the code being fixed. It reads the change to the
  test itself, so it catches the two-line edit that scope and diff size miss.
- **Remote, hardened after a security review.** Pairing a phone can no longer
  be attacked by a compromised server, which could have found the code in the
  answer it relays. The laptop now records trackline's settings and hook
  wiring when you turn remote on, and refuses a task from your phone if they
  have changed since; an agent started from your phone cannot edit them, and
  approvals from your phone are kept outside the project. A conversation can
  only be continued in the project it happened in. Turning remote off stops
  any agent it started.
- **After updating, run `trackline remote enable` again** in each project
  that takes tasks from your phone, and pair again if asked. Earlier
  versions cannot pair with the site any more.

## 0.6.0 · 2026-09-26

- **Multi-turn evals.** `trackline-gate` gains a fourth suite: conversations
  scored turn by turn, with `memorySafety` for turns that are only correct
  because of an earlier constraint or refusal. Optional: a project without
  `datasets/multi-turn.jsonl` reports it as skipped.
- **A one-row regression now fails the gate** in fixture mode. Live runs and
  the judge keep the 0.05 tolerance.
- **Any OpenAI-compatible judge.** `EVAL_JUDGE_BASE_URL` and
  `EVAL_JUDGE_API_KEY`, with retries on rate limits and dropped connections.
- **Faster live runs.** `--concurrency=N`, default 4.
- **`trackline-gate init` starts empty.** The fintech sample is
  `trackline-gate init --example=fintech-support`.

## 0.5.0 · 2026-09-26

- **Allow once, or keep it blocked, from your phone.** When trackline stops
  something in a session started from your phone, the app offers Allow once
  and Keep blocked. Allow once lets that one action through, in that session,
  once, and the agent retries it; Keep blocked tells the agent not to try
  again. Both are signed with your passkey.
- **Fix:** turning remote off with a test or development config folder no
  longer stops the real runner.

## 0.4.0 · 2026-09-25

- **Reply from your phone.** A task or a session can be continued from the
  app: the reply goes to the same Claude Code or Codex session on your laptop,
  with the same permissions and sandbox as a new one. Works for sessions you
  started in the terminal too.
- **macOS permission prompts.** When an agent started from your phone seems
  to be held by a Files and Folders prompt on the Mac (Documents, Desktop,
  Downloads, iCloud Drive), the app says so, so a task does not look frozen.

## 0.3.0 · 2026-09-25

- **Remote prompts.** Send a task from your phone to Claude Code or Codex on
  your own laptop, and watch it run. `trackline remote enable` turns it on for
  one project and pairs your passkey by a code shown on the laptop. Every
  message is signed by your passkey and checked on the laptop, so the server
  that carries it cannot write one. Remote sessions block writes to secrets and
  new dependencies, and run only where trackline is watching.
  `trackline remote disable --all` turns it off everywhere and forgets the
  passkeys.
- **An optional account.** `trackline connect` links a project to a trackline
  account and a live dashboard: what the agent is doing, its replies, and
  alerts. `trackline account` and `trackline disconnect`. Nothing uploads
  without it.
- **Setup.** `trackline init` sets up every installed agent when not told
  which, and `trackline status` says which agent is set up but has never
  reported, and why.

## 0.2.0 · 2026-09-24

- **Cursor.** `trackline init --host cursor` wires the hook into Cursor. It sees
  the agent once the agent is started inside the project.
- **Any MCP client.** `trackline mcp` runs an MCP server with `check_action` and
  `get_rules`, so an agent can ask before it acts. It advises. It cannot block.
- **Production traces.** `trackline traces` checks OTLP exports, protobuf or JSON.
  `trackline serve` receives them live and can post an alert. Both check tool
  policy (tools that must never be called, and approvals that must come first),
  outages, loops and drift. In production it alerts and never stops anything.
- **The judge, per action.** The optional judge now rules on each action rather
  than the work as a whole. `trackline review --json` gives machine-readable
  output, and the judge runs through Claude, Codex or Cursor's CLI.
- **Codex.** Reads Codex transcripts, including codex-cli 0.155's Bash tool and
  headless requests, instead of reading an unknown format as silence.
- **Fixes.** Commands that can write anywhere are no longer reported as reads. An
  unreadable rules file is reported instead of read as no rules. Every
  subcommand answers `--help` instead of running.

## 0.1.0 · 2026-09-23

The first release, for Claude Code and Codex.

- **The hook.** Runs before every action the agent takes, in about 14 ms.
- **The checks.** Off-limits paths, new dependencies, scope, diff size and
  repetition.
- **Modes.** Warn, ask or auto, set per check, with remembered approvals:
  `trackline allow`, `allowed` and `revoke`.
- **Commands.** `init`, `status`, `doctor`, `show` and `review`.
- **The judge.** Optional and off by default, for the one question counting
  cannot answer: whether the work serves the request.
- **Install.** `npm install -g trackline`, with a prebuilt binary for each
  platform.
