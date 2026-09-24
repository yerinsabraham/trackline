# Releasing

Publishing happens from CI and nowhere else. `npm publish` from a laptop
succeeds, looks identical, and silently produces **no provenance** — the signed
attestation tying a published tarball to the commit that built it. Nothing warns
you, and the first published version is the one that sets expectations.

Six packages ship together: `trackline`, plus one per platform under the
`@trackline` scope.

---

## First release: the chicken and the egg

npm's trusted publishing is configured **on a package's settings page**, which
means the package has to exist first. A brand-new name cannot use it. So the
first release uses a token, and every release after it does not.

The token still publishes **from CI**, so the first version gets provenance like
every other.

### 1. An npm account

[npmjs.com/signup](https://www.npmjs.com/signup), if there isn't one already.
Enable two-factor authentication — publishing without it is a bad habit for
anything other people will install.

Then create the **`trackline` organization** (free for public packages) at
[npmjs.com/org/create](https://www.npmjs.com/org/create). The platform packages
publish under it.

They were unscoped at first, `trackline-darwin-arm64` and so on. npm's spam
filter published four of them and refused the fifth: five near-identical names
from a new account in under a minute. The scope removes that, and it reserves
every `@trackline/*` name for this project.

### 2. A token, scoped as narrowly as it goes

[npmjs.com/settings/~/tokens](https://www.npmjs.com/settings/~/tokens) →
**Generate New Token** → **Granular Access Token**.

| Field | Value |
|---|---|
| Expiration | 7 days — it is needed once |
| Packages and scopes | Read and write |
| Bypass two-factor authentication | **Ticked.** Without it, CI fails with `EOTP`: npm asks for a code from your phone, and a workflow has no phone. It cannot be changed after the token is created. |
| Select packages | *All packages* (the six do not exist yet, so they cannot be named) |
| Organizations | No access |

Copy it. npm shows it once.

### 3. Give it to the repository

```bash
gh secret set NPM_TOKEN --repo yerinsabraham/trackline
```

Paste when prompted. It never touches a file.

### 4. Rehearse

Locally first: `npm run smoke` rebuilds every binary, packs, installs into an
empty directory and runs the result. The binaries under `packages/*/bin` are
gitignored build output, so testing without rebuilding can run a stale one.

Then in CI:

Actions → **Release** → **Run workflow**, leaving *dry run* ticked.

It runs the full suite, builds all five platforms, checks every package version
agrees, installs from packed tarballs and runs the CLI — then publishes nothing.
Getting a green run here means the only untested thing left is the upload.

### 5. Ship

```bash
git tag v0.1.0
git push origin v0.1.0
```

The tag triggers the real run. Platform packages publish first so the main
package's optional dependencies resolve the moment it is installable. Anything
already on npm is skipped, so a run that fails partway can simply be re-run.
The workflow afterwards installs from the registry to confirm `LICENSE`, `NOTICE`
and `README` are in what npm actually served.

### 6. Record the platform packages in the lockfile

```bash
npm install --package-lock-only
git commit -am "Record the published platform packages in the lockfile"
```

Until the platform packages exist on npm, the lockfile cannot hold entries for
them, and npm 11 (Node 24) refuses `npm ci` without them. CI's Node 24 job is
red from the moment `optionalDependencies` is added until this runs. Node 20 and
22 do not check, which is why the release itself is unaffected.

---

## Then switch to trusted publishing and throw the token away

Now the packages exist, so they can be configured to publish without one. This
is worth doing: a long-lived token in repository secrets is a credential that
can leak, and trusted publishing has none.

From the terminal, logged in with `npm login`. npm asks for browser approval on
each:

```bash
for p in trackline @trackline/darwin-arm64 @trackline/darwin-x64 \
         @trackline/linux-arm64 @trackline/linux-x64 @trackline/win32-x64; do
  npm trust github "$p" --file release.yml \
    --repo yerinsabraham/trackline --allow-publish -y
done
```

`--allow-publish` is required; without a permission flag npm refuses. Then
check all six, because a missed one only shows up as a failed release later:

```bash
for p in trackline @trackline/darwin-arm64 @trackline/darwin-x64 \
         @trackline/linux-arm64 @trackline/linux-x64 @trackline/win32-x64; do
  echo "== $p"; npm trust list "$p"
done
```

Every package should name `yerinsabraham/trackline` and `release.yml`. The same
thing can be done by hand at `npmjs.com/package/<name>/access` → **Trusted
Publisher**, with the environment left empty.

Then delete the token in both places: `npm token list` and
`npm token revoke <id>`, and
`gh secret delete NPM_TOKEN --repo yerinsabraham/trackline`.

The workflow needs `permissions: id-token: write` (it has it) and **npm 11.5.1
or newer**, which is what uses trusted publishing. Node 22 ships npm 10, which
does not: with npm 10 and no token, publishing fails with `ENEEDAUTH`. That is
how the first v0.2.0 run failed, publishing nothing. The workflow now upgrades
npm before it publishes.

### Confirming it worked

The next release should publish with an empty `NPM_TOKEN`. On
npmjs.com each package should show a **provenance** badge naming the commit it
was built from.

---

## Every release after the first

```bash
# bump every package together, or the shim loads binaries of another version
npm version 0.2.0 --no-git-tag-version
node -e '
  const fs=require("fs"),v=require("./package.json").version;
  for(const d of fs.readdirSync("packages")){
    const p=`packages/${d}/package.json`, j=JSON.parse(fs.readFileSync(p));
    j.version=v; fs.writeFileSync(p, JSON.stringify(j,null,2)+"\n");
  }
  const m=JSON.parse(fs.readFileSync("package.json"));
  for(const k of Object.keys(m.optionalDependencies)) m.optionalDependencies[k]=v;
  fs.writeFileSync("package.json", JSON.stringify(m,null,2)+"\n");
'

git commit -am "Release 0.2.0"
git tag v0.2.0
git push origin main --tags
```

The workflow refuses to publish if the versions disagree, because a shim
loading binaries of a different version is a package whose optional
dependencies can never resolve.
