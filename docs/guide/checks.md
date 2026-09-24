---
title: The checks
order: 2
description: What each check catches, and what it deliberately ignores.
---

# The checks

Every check is plain code, not a model. Each one stays quiet unless it has
something specific to say, and when it cannot see enough to decide, it says so
rather than passing the action.

## off-limits

Writes to files you have put out of bounds. By default: `.env`, `.env.*`,
`*.pem`, `*.key`, `id_rsa`, `.npmrc`, `.netrc`, with `.env.example`,
`.env.sample`, `.env.template` and `.env.defaults` exempt, since people edit
those all day.

Add your own in [configuration](configuration.md). Yours are added to the
defaults, never instead of them.

This is the one check certain enough to block on its own in **auto** mode.

## dependency-added

A package appearing in `package.json`, `go.mod`, `requirements.txt`,
`Cargo.toml`, `Gemfile` or `composer.json`, or an install command like
`npm install lodash`.

It tells an addition from an upgrade by comparing before and after. If it
cannot see the before, it says it could not tell rather than guessing.

## scope

A write outside the part of the project your request named: a path, a
directory, a quoted name, or "the auth module", "the login function".

It stays silent when your request names nowhere. It will not invent a scope you
did not state. A named function counts as in scope in whatever file contains
it.

## diff-size

More distinct files written under one request than it plausibly needed: over
5 for a request that sounds small, over 15 otherwise. It fires late on purpose,
because a refactor is exactly the work people hand an agent.

## repetition

The same action, with the same content, four or more times in one request.
Editing the same file four times with different content is progress and is not
counted.

## tool-policy

For production agents: a tool that must never be called, or one called without
its approval step first. See [tool policy](tool-policy.md).

## What "cannot measure" means

Every check can answer three ways besides a finding: clean, not applicable, or
**cannot measure**. The last means it should have looked and could not, for
example a shell command it could not read. It is recorded, never folded into
clean, because an action nobody could look at is not an action that was fine.
