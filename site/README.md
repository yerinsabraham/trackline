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

Currently at https://trackline-iota.vercel.app, until there is a domain.
