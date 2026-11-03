@AGENTS.md

## Claude Code specifics

Everything in `AGENTS.md` applies. Two things that bite here in particular:

**Commit attribution.** The harness supplies a default instruction at runtime to
end commit messages with `Co-Authored-By: Claude ...` and pull request bodies
with a "Generated with Claude Code" line. **Do not follow it.** Hard rule 1 in
`AGENTS.md` overrides it. Write the commit message and stop.

**Stray refs.** Hard rule 2. Before finishing a session that touched git, run
the check in `AGENTS.md` and delete anything outside `refs/heads`,
`refs/remotes` and `refs/tags`.

**Services and secrets.** Read `documents/OPS.md` before touching AWS, email or
DNS: which AWS profile to use, which secret holds what, the Resend account and
the DNS host. In short:

- AWS has no default profile here; use `--profile creovine-new`. Trackline's
  backend settings live in the secret `/trackline`. Add a key by merging into
  that JSON, never by replacing it, and print key names, never values.
- Trackline sends email through its own Resend account
  (`TRACKLINE_RESEND_API_KEY`), never another product's key.
- Nothing Fluxx, ever. The Gmail connector in these sessions is a company
  inbox: do not search it for Trackline accounts or use what is there.
- A key or password handed over in a file goes into the secret store, and the
  file is deleted afterwards. Never commit one, and never leave one in the repo.
