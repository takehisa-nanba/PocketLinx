# PocketLinx (plx)

<p align="center">
  <strong>Portable, Instant, and Clean Container Runtime for WSL2.</strong>
  <br>
  <em>Windows is just a remote control. Linux does the heavy lifting.</em>
</p>

---

### 🇯🇵 日本語 (Japanese)
**PocketLinx (v0.2.0)** は、WSL2 (Windows Subsystem for Linux) の性能をネイティブに引き出す次世代のコンテナランタイムです。
従来の「Windowsファイルシステム上で開発する」という常識を覆し、**「Windowsはただのリモコンとして使い、ビルドも実行もすべてWSL2内部の高速なLinuxファイルシステムで完結させる」** というアーキテクチャを採用しました。これにより、Git for Windows対比で数十倍のディスクI/O速度と、完全に隔離されたクリーンな開発環境を実現します。

### 🇺🇸 English
**PocketLinx (v0.2.0)** is a next-generation container runtime designed to leverage the native performance of WSL2.
It flips the script on Windows development: **"Windows is just the remote control."** All building, downloading, and execution happens entirely within the high-speed Linux filesystem (ext4) inside WSL2, bypassing the slow NTFS IO bottleneck. This delivers blazing fast performance compared to traditional Windows-based workflows while keeping your host OS clean.

---

## 🚀 Features (主な機能)

- **WSL-Native Architecture**: No more slow NTFS mounts. Builds and Runs happen on ext4.
- **Single Binary**: One `plx.exe` rules them all. No complex dependencies.
- **Instant Setup**: `plx setup` gets you a full Linux environment in seconds.
- **Project Config**: `plx.json` automates environment setup for teams.
- **Zero Bloat**: Keeps your Windows host clean. Everything lives in WSL.

---

## 🛠️ Installation (インストール)

1.  **Build** the binary:
    ```powershell
    go build -o plx.exe cmd/plx/main.go
    ```
2.  **Install** (Add to PATH):
    ```powershell
    .\plx.exe install
    ```
    *(Restart your terminal after this)*

3.  **Setup** environment:
    ```powershell
    plx setup
    ```

---

## 📖 Usage (使い方)

### 1. Basic Run
Run a command in an isolated container. (Images are stored in WSL, not Windows!)
```powershell
plx run alpine uname -a
# Linux pocketlinx ... x86_64 Linux
```

### 2. Native Build (v0.2.0 New!)
Build an image from a Dockerfile. The source code is temporarily streamed to WSL, built there, and the result is saved directly into WSL storage (`/var/lib/pocketlinx/images`). No heavy `tar.gz` is ever written to Windows.
```powershell
plx build -t my-app .
```

### 3. Managed Volumes (Coming Soon)
Instead of mounting slow Windows folders, plan to use managed volumes that live in WSL.
```powershell
# (Proposed)
plx volume create my-deps
plx run -v my-deps:/app/node_modules ...
```

---

## 🏗️ Architecture: "Windows as Remote Control"

### v0.1.0 (Old)
- **Flow**: Download to Windows -> Convert to WSL path -> Run.
- **Bottleneck**: Heavy I/O traffic across the Windows/WSL boundary.

### v0.2.0 (New)
- **Flow**: `plx` command (Windows) -> Signal WSL -> **Download/Build/Run inside WSL**.
- **Result**: Zero heavy files on Windows. Native Linux speed.

---

## 🛣️ Roadmap (ロードマップ)

| Phase | Feature | Status |
| :--- | :--- | :--- |
| **Phase 1** | **Foundation** (WSL2 Backend, Stable Engine) | ✅ Done |
| **Phase 2** | **Lifecycle** (`start`, `stop`, `ps`, `rm`) | ✅ Done |
| **Phase 3** | **Architecture v2** (WSL-Native Storage) | ✅ Done (v0.2.0) |
| **Phase 4** | **Ecosystem** (Managed Volumes, Networks) | 🚧 Planned |

---

## 🛡️ License & Partnership

### 🇯🇵 ビジネスパートナー募集
**「この技術で世界を変えたい」**
作者（@takehisa-nanba）は技術に特化していますが、これをビジネスとして広めるためのパートナー（マーケティング、商品化戦略、資金調達など）を真剣に探しています。もし PocketLinx に可能性を感じていただけたなら、ぜひご連絡ください。

### 🇺🇸 Call for Partners
I am actively looking for **business partners**! While I focus on engineering the best possible container technology, I need collaborators with expertise in growth, marketing, and monetization strategies. If you see a business opportunity here, let's build something big together.

**License**: MIT (Free for personal/OSS use. Commercial use requires agreement.)
