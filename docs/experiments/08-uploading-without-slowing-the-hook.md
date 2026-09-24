# Can the hook upload without getting slower?

**Yes, by never uploading from the hook. On a machine that is not connected the
hook is unchanged. On a connected one it costs about 1ms more, for writing one
small file. The network is someone else's problem: a separate process sends,
in batches, after the hook has already answered.**

Connecting a machine to a trackline account (`trackline connect`) sends what
the checks conclude to a dashboard. The hook runs before every tool call, and
[experiment 2](02-hook-latency.md) is why it is written in Go at all. An upload
that made it slower would be a bad trade.

---

## The design

The hook writes each event to an outbox on disk: one file per event, written
under a temporary name and renamed into place, so parallel tool calls never
interleave and a half-written event is never read. If nobody is sending, it
starts `trackline sync` in the background, with its output going nowhere,
because a host waits for the hook's stdout and stderr to close. Then it
answers.

`trackline sync` sends the outbox oldest first, and deletes a file only once
the server has accepted it. Every event carries an id the server deduplicates
on, so a sync killed at any point loses nothing and doubles nothing. Offline,
events wait and the next attempt backs off (30 seconds, doubling, at most 15
minutes), so the hook does not start a doomed sender on every call.

## Method

The same `Write` tool call, fed to three hooks, interleaved so that load on the
machine falls on all three alike: the hook from before this change, the new
hook on a machine that is not connected, and the new hook inside a connected
project, sending to a local server. 10 warm-up runs each, then 200 rounds.
Code in [`code/upload-latency/`](code/upload-latency/).

## Two things the first measurement found

**Linking network code into the hook cost 3.4ms on every start**, before it
did anything. The first version reached the account through one package that
held both the saved credential and the HTTP client. The hook only reads the
credential, but Go initialises everything linked in. Splitting the two, so the
hook links no network code at all, took the unconnected cost back to noise. A
test now fails if `net` or `crypto/tls` ever reaches the hook's imports.

| First version | median | p95 |
|---|---|---|
| before | 11.2ms | 13.5ms |
| not connected | 14.6ms | 16.9ms |
| connected | 16.9ms | 19.1ms |

**210 events went up in 120 requests.** The sender waits two seconds before
sending, so that a burst of tool calls travels as one batch. It took its lock
after the wait, not before, so every hook during those two seconds saw nobody
sending and started another sender. With the lock held through the wait, the
same 210 events went up in 8 requests.

## Result

After both fixes, three runs. The machine was under heavy load throughout
(load average 14 to 17), which moves every number; the interleaving is what
keeps the comparison fair.

| Run | before | not connected | connected |
|---|---|---|---|
| 1 | 16.45ms | 16.90ms | 17.55ms |
| 2 | 22.13ms | 22.29ms | 23.14ms |
| 3 | 28.46ms | 28.63ms | 29.59ms |

Medians. Not connected is within 0.5ms of before in every run, and the p95s
cross over. Connected is 0.7 to 1.1ms slower in every run: the outbox write.
That is the cost, and only a machine that chose to connect pays it.

In the same runs every event arrived: 210 queued, 210 stored, none left in the
outbox.

## What else was checked

Against a local copy of the backend, through the real CLI and hook:

- Paths arrive relative to the project. `/etc/passwd` and `~/.ssh/id_rsa`, read
  by a command in the same session, were dropped before leaving.
- No file content, command text or token reached the database. The stored rows
  were searched for the content written, the key in `.env` and the home
  directory; none were there.
- Times first went up cut to the second, and four calls in one second came back
  from the database in reverse. They now carry milliseconds.

The outbox is tested for going offline and coming back (all events, in order,
once), a sender killed after the server stored a batch but before it heard so
(nothing lost, nothing doubled), a sender that died holding the lock, a revoked
machine, and a batch the server refuses. Each test was checked by breaking the
code it guards and watching it fail. One of those breaks, a sender that never
removed what it sent, looped until the machine ran out of disk; the test's
stand-in server now stops after 5,000 requests.

## What this does not show

Real sessions in Claude Code, Codex and Cursor uploading to a deployed backend.
That is the exit test for this phase, and it has not been run yet.
