# PocketLinx (plx)

<p align="center">
  <img src="pkg/api/logo.png" alt="PocketLinx Logo" width="200">
  <br>
  <strong>Portable, Instant, and Clean Container Runtime for WSL2.</strong>
  <br>
  <em>Windows is just a remote control. Linux does the heavy lifting.</em>
</p>

---

## 🌟 Overview / 概要

**PocketLinx (v0.6.4)** is a next-generation container runtime designed for the native performance of WSL2. It embraces the design ideal of **"Minimal Interaction"**: The "heavy door" of provisioning and network setup is opened once, and you work efficiently inside. No more waiting for extraction every time you run a command.

**PocketLinx (v0.6.4)** は、WSL2の性能を最大限に引き出す設計思想をさらに前進させました。「重い扉（プロビジョニングやネットワーク設定）を一度開けたら、その中で効率的に作業する」という **"Minimal Interaction"** を実現。コマンドを叩くたびに展開を待つ必要はもうありません。

---

## 🚀 Features / 主な機能

- **⚡ WSL-Native Architecture**
  - Operates entirely on the WSL ext4 filesystem. No more NTFS slowness.
  - すべてWSL上のext4で動作。NTFSの遅さとは無縁です。

- **🚀 Loopback IP per Container (v0.5.0)**
  - Each container gets its own unique loopback IP (127.0.0.x) on Windows. No more port conflicts!
  - コンテナごとに固有のループバックIP（127.0.0.x）を自動割当。ポートの衝突を根本から解消しました。

- **🏠 Branded Host Auto-Discovery (v0.6.0)**
  - Containers can automatically reach the Windows host via `host.plx.internal`. No manual IP lookup needed.
  - コンテナから Windows ホストへ `host.plx.internal` で自動接続。IP アドレスを手動で調べる手間をなくしました。

- **🚪 Persistent Named Containers & `exec` (v0.6.4 - Updated)**
  - Use `--name` to keep your environment alive. Join instantly with `plx exec` for zero-overhead development. Fixed namespace isolation issues for reliable file access and networking.
  - `--name` で環境を固定。`plx exec` で「開いた扉」の中へ瞬時に入り込みます。名前空間の分離レベルを最適化し、ファイルアクセスとネットワークの信頼性を向上させました。

- **🎛️ Compose Support (v0.4.0)**
  - Orchestrate multiple containers using `plx-compose.yml`.
  - YAMLファイル一つで、複数のコンテナをワンタップで一括管理・連携。

- **📊 Premium Dashboard (v0.5.0)**
  - Glassmorphism design with real-time logs and **Smart Tab Management** (re-uses existing browser tabs).
  - リアルタイムログ視聴、タブの重複を防ぐスマート管理機能を備えた美しいGUI。

- **📦 Build Cache & Managed Volumes**
  - Layer caching and persistent data storage within WSL.
  - レイヤーキャッシュによる高速ビルドと、WSL内へのデータ永続化。

---

## 🛠️ Installation / インストール

### Install / インストール
Download `plx.exe` and add it to your PATH:
```powershell
.\plx.exe install
```
*(Restart your terminal to apply PATH changes / インストール後、ターミナルを再起動してください)*

### Setup / 初期セットアップ
Initialize the Linux environment:
```powershell
plx setup
```

---

## 📖 Usage / 使い方

### 1. Persistent Workflow / 継続的な作業
Start a container once:
```powershell
plx run -d --name my-dev-env alpine sleep infinity
```
Work inside instantly (snappy!):
```powershell
plx exec my-dev-env ls /
```

### 2. Ephemeral Run / 単発実行
```powershell
plx run alpine uname -a
```

### 2. Compose / 複数コンテナの管理
```powershell
plx compose up
```

### 3. Dashboard / ダッシュボード
```powershell
plx dashboard
```

---

## 🛡️ License & Partnership / ライセンスと提携

### 🤝 Call for Partners / ビジネスパートナー募集

I am actively looking for **business partners**! While I focus on engineering, I need collaborators with expertise in growth, marketing, and monetization. If you see a business opportunity here, let's build something big together.

作者（@takehisa-nanba）は技術に特化していますが、これをビジネスとして広めるためのパートナー（マーケティング、商品化戦略、資金調達など）を真剣に探しています。PocketLinx に可能性を感じていただけたなら、ぜひご連絡ください。

**License**: MIT (Free for personal/OSS use. Commercial use requires agreement.)
