# PocketLinx (plx)

<p align="center">
  <strong>Portable, Instant, and Clean Container Runtime for WSL2.</strong>
  <br>
  <em>Windows is just a remote control. Linux does the heavy lifting.</em>
</p>

---

## 🌟 Overview / 概要

**PocketLinx (v0.5.0)** is a next-generation container runtime designed to leverage the native performance of WSL2. It flips the script on Windows development: **"Windows is just the remote control."** All building, downloading, and execution happens entirely within the high-speed Linux filesystem (ext4) inside WSL2, bypassing the slow NTFS IO bottleneck.

**PocketLinx (v0.5.0)** は、WSL2 の性能をネイティブに引き出す次世代のコンテナランタイムです。「Windowsはただのリモコンとして使い、重たい処理はすべてWSL2の中にお任せ」という設計により、NTFSのボトルネックを解消し、Gitやnpm installが驚くほど速くなるクリーンな開発環境を提供します。

---

## 🚀 Features / 主な機能

- **⚡ WSL-Native Architecture**
  - Operates entirely on the WSL ext4 filesystem. No more NTFS slowness.
  - すべてWSL上のext4で動作。NTFSの遅さとは無縁です。

- **🚀 Loopback IP per Container (v0.5.0 - NEW!)**
  - Each container gets its own unique loopback IP (127.0.0.x) on Windows. No more port conflicts!
  - コンテナごとに固有のループバックIP（127.0.0.x）を自動割当。ポートの衝突を根本から解消しました。

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

### 1. Run / コンテナを実行
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
