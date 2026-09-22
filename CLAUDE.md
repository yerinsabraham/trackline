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
