# Releasing

Binaries are never committed to git. They are uploaded as assets to a GitHub Release, which is attached to a git tag.

## Download table

| Architecture | Linux | Windows | macOS |
|---|---|---|---|
| **x86-64** (64-bit) | [`2L1nk-linux-x86-64`](https://github.com/LBS-Eibiswald-APC/Team9_SanjariReza_KirschFlorian_2L1nk/releases/download/vX.X.X/2L1nk-linux-x86-64) | [`2L1nk-windows-x86-64.exe`](https://github.com/LBS-Eibiswald-APC/Team9_SanjariReza_KirschFlorian_2L1nk/releases/download/vX.X.X/2L1nk-windows-x86-64.exe) | [`2L1nk-darwin-x86-64`](https://github.com/LBS-Eibiswald-APC/Team9_SanjariReza_KirschFlorian_2L1nk/releases/download/vX.X.X/2L1nk-darwin-x86-64) |
| **ARM64** (64-bit) | [`2L1nk-linux-arm64`](https://github.com/LBS-Eibiswald-APC/Team9_SanjariReza_KirschFlorian_2L1nk/releases/download/vX.X.X/2L1nk-linux-arm64) | [`2L1nk-windows-arm64.exe`](https://github.com/LBS-Eibiswald-APC/Team9_SanjariReza_KirschFlorian_2L1nk/releases/download/vX.X.X/2L1nk-windows-arm64.exe) | [`2L1nk-darwin-arm64`](https://github.com/LBS-Eibiswald-APC/Team9_SanjariReza_KirschFlorian_2L1nk/releases/download/vX.X.X/2L1nk-darwin-arm64) |

> **x86-64** — most desktops, laptops, and cloud VMs (Intel Core, AMD Ryzen, etc.)
> **ARM64** — Apple Silicon (M1/M2/M3), AWS Graviton, Raspberry Pi 4+, Windows on ARM

### Examples

| Architecture | Linux | Windows | macOS |
|---|---|---|---|
| **x86-64** | Ubuntu server, Debian VPS, any Intel/AMD PC running Linux | Windows 11 on a Dell/HP/Lenovo laptop | MacBook Pro (Intel, pre-2020) |
| **ARM64** | Raspberry Pi 4/5, AWS Graviton EC2 instance | Surface Pro X, Snapdragon-based Windows laptop | MacBook Air/Pro with M1, M2, M3, or M4 |

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
  bin/linux/2L1nk-linux-x86-64 \
  bin/linux/2L1nk-linux-arm64 \
  bin/windows/2L1nk-windows-x86-64.exe \
  bin/windows/2L1nk-windows-arm64.exe \
  bin/darwin/2L1nk-darwin-x86-64 \
  bin/darwin/2L1nk-darwin-arm64 \
  --title "2L1nk vX.X.X" \
  --notes "..."
```
