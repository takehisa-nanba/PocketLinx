# PocketLinx (plx)

<p align="center">
  <strong>Portable, Instant, and Clean Container Runtime for WSL2.</strong>
  <br>
  <em>Windows is just a remote control. Linux does the heavy lifting.</em>
</p>

---

## 🌟 Overview / 概要

**PocketLinx (v0.3.0)** is a next-generation container runtime designed to leverage the native performance of WSL2. It flips the script on Windows development: **"Windows is just the remote control."** All building, downloading, and execution happens entirely within the high-speed Linux filesystem (ext4) inside WSL2, bypassing the slow NTFS IO bottleneck.

**PocketLinx (v0.3.0)** は、WSL2 の性能をネイティブに引き出す次世代のコンテナランタイムです。「Windowsはただのリモコンとして使い、重たい処理はすべてWSL2の中にお任せ」という設計により、NTFSのボトルネックを解消し、Gitやnpm installが驚くほど速くなるクリーンな開発環境を提供します。

---

## 🚀 Features / 主な機能

- **⚡ WSL-Native Architecture**
  - Operates entirely on the WSL ext4 filesystem. No more NTFS slowness.
  - すべてWSL上のext4で動作。NTFSの遅さとは無縁です。

- **📦 Build Cache (v0.3.0)**
  - Layer caching makes subsequent builds blazing fast.
  - レイヤーキャッシュにより、2回目以降のビルドが高速化されます。

- **💾 Managed Volumes (v0.3.0)**
  - Persistent data storage within WSL, ideal for databases.
  - コンテナデータをWSL内に永続保存。高速なDB領域などに最適です。

- **🌐 Simple Networking (v0.3.0)**
  - Connect containers by name (e.g., app to db).
  - コンテナ同士を名前で呼び合えます。

- **📊 Dashboard (v0.3.0)**
  - Manage containers via a browser with a single click.
  - ブラウザから直感的にコンテナを管理できるGUIを提供します。

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

### 2. Build / イメージをビルド
```powershell
plx build -t my-app .
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
