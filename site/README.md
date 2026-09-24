# site

The product site: home, docs and evidence. Static Next.js, deployed to Vercel.

```bash
npm install
npm run dev          # http://localhost:3000
```

The docs are not written here. They are read from `../docs/guide/` at build
time, so the site and GitHub show the same text. Edit the Markdown there.

## Deploy

Built locally and uploaded as output, because the build reads `../docs`, which
a Vercel build started from this folder would not have:

```bash
vercel build --prod
vercel deploy --prebuilt --prod
```

At https://trackline.dev. The old https://trackline-iota.vercel.app address still
resolves, so prompts pasted before the domain existed keep working.
If the address changes, change `SITE` in `lib/site.ts`: the agent prompt and
`/install.md` both read it. The full checklist is in `AGENTS.md`.
