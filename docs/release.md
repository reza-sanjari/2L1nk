# Releasing

Binaries are never committed to git. They are uploaded as assets to a GitHub Release, which is attached to a git tag.

## Steps

```bash
# 1. Commit source code
git add .
git commit -m "chore: bump version to vX.X.X"
git push

# 2. Tag the commit
git tag vX.X.X
git push origin vX.X.X

# 3. Build static binaries
make build-static

# 4. Create release and upload binaries
gh release create vX.X.X \
  bin/linux/2L1nk-static \
  bin/windows/2L1nk-static.exe \
  bin/darwin/2L1nk-static-amd64 \
  bin/darwin/2L1nk-static-arm64 \
  --title "2L1nk vX.X.X" \
  --notes "..."
```

---

## Sample — Beta Release

```bash
git add .
git commit -m "chore: bump version to v1.0.0-beta.1"
git push
git tag v1.0.0-beta.1
git push origin v1.0.0-beta.1

make build-static

gh release create v1.0.0-beta.1 \
  bin/linux/2L1nk-static \
  bin/windows/2L1nk-static.exe \
  bin/darwin/2L1nk-static-amd64 \
  bin/darwin/2L1nk-static-arm64 \
  --title "2L1nk v1.0.0-beta.1" \
  --notes "Beta release — expect bugs. Do not use in production." \
  --prerelease
```

> `--prerelease` marks it as a pre-release on GitHub so it does not show as the latest stable version.

---

## Sample — Stable Release (v1.0.0)

```bash
git add .
git commit -m "chore: bump version to v1.0.0"
git push
git tag v1.0.0
git push origin v1.0.0

make build-static

gh release create v1.0.0 \
  bin/linux/2L1nk-static \
  bin/windows/2L1nk-static.exe \
  bin/darwin/2L1nk-static-amd64 \
  bin/darwin/2L1nk-static-arm64 \
  --title "2L1nk v1.0.0" \
  --notes "First stable release."
```

Users can always find the latest release at:
```
https://github.com/yourname/2L1nk/releases/latest
```
