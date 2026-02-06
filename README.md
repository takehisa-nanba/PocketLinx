# PocketLinx (plx)

<p align="center">
  <strong>Portable, Instant, and Clean Container Runtime for WSL2.</strong>
  <br>
  <em>Windows is just a remote control. Linux does the heavy lifting.</em>
</p>

---

### 🇯🇵 日本語 (Japanese)
**PocketLinx (v0.3.0)** は、WSL2 (Windows Subsystem for Linux) の性能をネイティブに引き出す次世代のコンテナランタイムです。

「Windows上で開発すると遅い…」「環境構築でWindowsが汚れる…」そんな悩みを解決します。
**「Windowsはただのリモコンとして使い、重たい処理はすべてWSL2の中にお任せ」** という設計により、高速かつクリーンな開発環境を提供します。Gitやnpm installも驚くほど速くなります。

### 🇺🇸 English
**PocketLinx (v0.3.0)** is a next-generation container runtime designed to leverage the native performance of WSL2.
It flips the script on Windows development: **"Windows is just the remote control."** All building, downloading, and execution happens entirely within the high-speed Linux filesystem (ext4) inside WSL2, bypassing the slow NTFS IO bottleneck. This delivers blazing fast performance compared to traditional Windows-based workflows while keeping your host OS clean.

---

## 🚀 Features (主な機能)

- **⚡ WSL-Native Architecture**: すべてWSL上のext4ファイルシステムで動作。NTFSの遅さとは無縁です。
- **📦 Build Cache (v0.3.0)**: 一度ビルドしたレイヤーはキャッシュされ、2回目以降は爆速になります。
- **💾 Managed Volumes (v0.3.0)**: コンテナのデータをWSL内に永続保存。高速なDBデータ領域などに最適です。
- **🌐 Simple Networking (v0.3.0)**: コンテナ同士を名前で呼べます（例: `app` から `db` にアクセス）。
- **📊 Dashboard (v0.3.0)**: コマンドが苦手でも大丈夫。ブラウザからクリック一つでコンテナ管理ができます。

---

## 🛠️ Installation (インストール)

1.  **インストール (Install)**:
    最新の `plx.exe` をダウンロードし、以下のコマンドでPATHに追加します。
    ```powershell
    .\plx.exe install
    ```
    （完了後、ターミナルを再起動してください）

2.  **初期セットアップ (Setup)**:
    必要なLinux環境を自動で準備します。
    ```powershell
    plx setup
    ```

---

## 📖 Usage (使い方)

### 1. 🏃 Run (コンテナを動かす)
まずは基本。Linuxのコマンドを隔離された環境で実行します。
```powershell
# Alpine Linuxで uname コマンドを実行
plx run alpine uname -a
```

### 2. 🔨 Build (イメージを作る)
Dockerfileからあなたのアプリをビルドします。**キャッシュ機能**により、コード以外の変更がなければ一瞬で完了します。
```powershell
# カレントディレクトリ (.) のDockerfileを使って my-app という名前でビルド
plx build -t my-app .
# 2回目はキャッシュが効いて速い！
plx build -t my-app .
```
キャッシュを消したいときは：
```powershell
plx prune
```

### 3. 💾 Data Volumes (データを保存する)
コンテナを消してもデータを残したい場合（データベースなど）は、**ボリューム**を使います。
```powershell
# 1. ボリュームを作成 "my-db-data"
plx volume create my-db-data

# 2. ボリュームをマウントして実行 (-v 名前:パス)
plx run -d -v my-db-data:/data alpine sh -c "echo '大切なデータ' > /data/file.txt"

# 3. 別のコンテナで確認（データが残っている！）
plx run -v my-db-data:/mnt alpine cat /mnt/file.txt
```

### 4. 🌐 Networking (コンテナ同士をつなぐ)
名づけられたコンテナ同士は、お互いの名前で通信できます。
```powershell
# 1. "db" という名前でコンテナを起動
plx run -d --name db alpine sleep 1000

# 2. 別のコンテナから "db" にpingを打つ
plx run alpine ping db
# -> 127.0.0.1 (db) から応答があります！
```

### 5. 📊 Dashboard (ダッシュボード)
コマンド操作に疲れたら、ダッシュボードを開きましょう。
```powershell
plx dashboard
```
ブラウザが開き、コンテナ一覧が表示されます。「Stop」や「Remove」もボタン一つです。

---

## 🏗️ Architecture: "Windows as Remote Control"

### v0.2.0 -> v0.3.0 Evolution
- **v0.2.0**: 基本的な実行とビルドをWSLネイティブ化し、高速化を実現。
- **v0.3.0**: キャッシュ、永続化ボリューム、ネットワーク、GUIといった「実用的な開発に必要な機能」をフル装備。

---

## 🛣️ Roadmap (ロードマップ)

| Phase | Feature | Status |
| :--- | :--- | :--- |
| **Phase 1** | **Foundation** (WSL2 Backend, Native Speed) | ✅ Done |
| **Phase 2** | **Core Features** (Cache, Volume, Network) | ✅ Done (v0.3.0) |
| **Phase 3** | **Experience** (Dashboard, Interactive UI) | ✅ Done (v0.3.0) |
| **Phase 4** | **Ecosystem** (Compose support, Plugins) | 🚧 Planned |

---

## 🛡️ License & Partnership

### 🇯🇵 ビジネスパートナー募集
**「この技術で世界を変えたい」**
作者（@takehisa-nanba）は技術に特化していますが、これをビジネスとして広めるためのパートナー（マーケティング、商品化戦略、資金調達など）を真剣に探しています。もし PocketLinx に可能性を感じていただけたなら、ぜひご連絡ください。

### 🇺🇸 Call for Partners
I am actively looking for **business partners**! While I focus on engineering the best possible container technology, I need collaborators with expertise in growth, marketing, and monetization strategies. If you see a business opportunity here, let's build something big together.

**License**: MIT (Free for personal/OSS use. Commercial use requires agreement.)
