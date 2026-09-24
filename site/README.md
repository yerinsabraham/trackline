# site

The product site: home, docs, agents, evidence, changelog. Static Next.js,
deployed to Vercel.

```bash
npm install
npm run dev          # http://localhost:3000
```

Little is written here. The site reads, at build time:

| What | From | So |
|---|---|---|
| Docs | `../docs/guide/*.md` | the site and GitHub show the same text |
| Evidence articles | `../docs/experiments/*.md` | the same |
| Changelog and its RSS feed | `../docs/CHANGELOG.md` | one record of releases |
| Per-agent pages | `data/hosts.json`, a copy of `engine/internal/hosts` | the site cannot claim more than the engine does |

`data/hosts.json` is checked by a Go test and rewritten by it:

```bash
cd ../engine && go test ./internal/hosts -update-site
```

Generated at build time, for agents and link previews: `/llms.txt`, a Markdown
copy of every docs page and experiment (`/docs/<slug>.md`, served from `/md/`
by a route in `vercel.json`, which also answers `Accept: text/markdown`), `/search.json` for the ⌘K search, and a share
card per page under `/og/`.

## Deploy

Built locally and uploaded as output, because the build reads `../docs`, which
a Vercel build started from this folder would not have:

```bash
vercel build --prod
vercel deploy --prebuilt --prod
```

At https://trackline.dev. The old https://trackline-iota.vercel.app address
redirects there with the path kept (`vercel.json`; it uses `routes`, not
`rewrites`, because rewrites run only after a static file fails to match), so prompts pasted before the
domain existed keep working.
If the address changes, change `SITE` in `lib/site.ts`: the agent prompt and
`/install.md` both read it. The full checklist is in `AGENTS.md`.
