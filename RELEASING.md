# Releasing

Publishing happens from CI and nowhere else. `npm publish` from a laptop
succeeds, looks identical, and silently produces **no provenance** — the signed
attestation tying a published tarball to the commit that built it. Nothing warns
you, and the first published version is the one that sets expectations.

Six packages ship together: `trackline`, plus one per platform.

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

### 2. A token, scoped as narrowly as it goes

[npmjs.com/settings/~/tokens](https://www.npmjs.com/settings/~/tokens) →
**Generate New Token** → **Granular Access Token**.

| Field | Value |
|---|---|
| Expiration | 7 days — it is needed once |
| Packages and scopes | Read and write |
| Select packages | *All packages* (the six do not exist yet, so they cannot be named) |
| Organizations | No access |

Copy it. npm shows it once.

### 3. Give it to the repository

```bash
gh secret set NPM_TOKEN --repo yerinsabraham/trackline
```

Paste when prompted. It never touches a file.

### 4. Rehearse

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
package's optional dependencies resolve the moment it is installable, and the
workflow afterwards installs from the registry to confirm `LICENSE`, `NOTICE`
and `README` are in what npm actually served.

---

## Then switch to trusted publishing and throw the token away

Now the packages exist, so they can be configured to publish without one. This
is worth doing: a long-lived token in repository secrets is a credential that
can leak, and trusted publishing has none.

For **each** of the six packages, at
`npmjs.com/package/<name>/access` → **Trusted Publisher**:

| Field | Value |
|---|---|
| Publisher | GitHub Actions |
| Organization or user | `yerinsabraham` |
| Repository | `trackline` |
| Workflow filename | `release.yml` |
| Environment | leave empty |

The packages:

```
trackline
trackline-darwin-arm64
trackline-darwin-x64
trackline-linux-arm64
trackline-linux-x64
trackline-win32-x64
```

Then **delete the token** at
[npmjs.com/settings/~/tokens](https://www.npmjs.com/settings/~/tokens).

No workflow change is needed. `permissions: id-token: write` is already there,
and npm uses OIDC automatically when no token is present.

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
