# The upload contract

`ingest-v1.schema.json` is everything a connected machine may send to a
trackline account, and nothing else. Every object in it is closed: a field it
does not list is refused.

It exists because the privacy promise ("what trackline noticed, never your
code") is only as good as the code that builds an upload. So both ends are held
to this one file:

- **The CLI** builds uploads in `engine/internal/cloud/payload`, one field at a
  time from the schema, never by serialising the engine's own event. Its tests
  run real recorded sessions from Claude Code, Codex and Cursor through the
  builder and fail if anything off the list leaves: command text, file
  contents, evidence values, the raw hook payload, an absolute path, the home
  directory, Cursor's account email, or a token-shaped string.
- **The backend** validates every upload against the same schema and has
  columns only for these fields.

What is never sent has no field here at all: file contents, diffs, command
text, tool arguments, environment variables, anything outside the project.

Changing it means changing both ends in step, and bumping the version for
anything that is not purely additive.
