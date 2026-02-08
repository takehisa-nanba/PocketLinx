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

**PocketLinx (v0.7.1)** is a next-generation container runtime designed for the native performance of WSL2. It embraces the design ideal of **"Minimal Interaction"**: The "heavy door" of provisioning and network setup is opened once, and you work efficiently inside. No more waiting for extraction every time you run a command.

**PocketLinx (v0.7.1)** は、WSL2の性能を最大限に引き出す設計思想をさらに前進させました。「重い扉（プロビジョニングやネットワーク設定）を一度開けたら、その中で効率的に作業する」という **"Minimal Interaction"** を実現。コマンドを叩くたびに展開を待つ必要はもうありません。

---

## 🚀 Features / 主な機能

- **⚡ WSL-Native Architecture**
  - Operates entirely on the WSL ext4 filesystem. No more NTFS slowness.
  - すべてWSL上のext4で動作。NTFSの遅さとは無縁です。

- **🚀 Loopback IP per Container (v0.5.0)**
  - Each container gets its own unique loopback IP (127.0.0.x) on Windows. No more port conflicts!
  - コンテナごとに固有のループバックIP（127.0.0.x）を自動割当。ポートの衝突を根本から解消しました。

- **💨 Blazing Fast Build with `.plxignore` (v0.7.1 - NEW)**
  - Skip heavy folders like `.git` or `.plx_env` during build. No more waiting for hash calculations.
  - `.plxignore` で巨大なフォルダをスキップ。ビルド前のハッシュ計算待ちを解消し、瞬時に実行を開始します。

- **🏠 Branded Host Auto-Discovery (v0.7.0 - Enhanced)**
  - Containers can automatically reach the Windows host via `host.plx.internal`. No manual IP lookup needed.
  - コンテナから Windows ホストへ `host.plx.internal` で自動接続。IP アドレスを手動で調べる手間をなくしました。

- **📲 Automatic ADB Bridge (v0.7.0 - NEW)**
  - Debug Android devices from inside containers instantly. `ANDROID_ADB_SERVER_ADDRESS` is automatically injected for seamless `adb` and `flutter` connectivity.
  - コンテナ内からホスト側の Android 実機を即座にデバッグ可能。環境変数を自動注入し、`adb` や `flutter` の透過的な接続を実現しました。

- **🚪 Rock-solid `exec` & `start` (v0.7.0 - Updated)**
  - Fixed namespace isolation and rootfs (deleted) issues for reliable access across all distros. Use `plx start` to revive stopped containers instantly.
  - `nsenter` と rootfs 名前空間の問題を修正し、あらゆるディストロで安定した接続が可能に。`plx start` で停止中の環境も瞬時に復帰できます。

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
