# PocketLinx (plx)

**PocketLinx** は、どこでも一貫した Linux 開発環境を持ち運べるように設計された、超軽量でポータブルな次世代のコンテナランタイムです。
Windows 環境において、Docker Desktop のような重いデーモンを必要とせず、WSL2 のパワーを最大限に引き出した隔離環境を瞬時に提供します。

---

## 🚀 主な機能 (Features)

- **シングルバイナリ**: `plx.exe` ひとつで動作。複雑な依存関係はありません。
- **インスタント・セットアップ**: `plx setup` 一発で Linux 環境（Alpine/Ubuntu）が整います。
- **WSL2 安定化エンジン**: 起動エラーの自動修復、DNS固定化、時刻同期を統合。
- **マルチ OS サポート**: 複数の Linux ディストリビューションを瞬時に切り替え可能。
- **ポータブル・データ**: イメージや設定は `%USERPROFILE%\.pocketlinx` で一元管理。
- **プロジェクト・コンフィグ**: `plx.json` でプロジェクトごとの環境設定を自動化。

## 🌟 現在のステータス (Status)

- [x] **WSL2 バックエンド基盤 (Phase 1)**: 安定したディストリビューション管理とエラー解決。
- [x] **コンテナ実行**: 標準的な Rootfs の実行と隔離。
- [x] **ネットワーク**: DNS 設定の永続化とホストとの通信。
- [x] **ライフサイクル管理 (Phase 2)**: `ps`, `stop`, `rm` コマンドの実装。
- [x] **バックグラウンド実行 (Phase 3)**: デタッチモード（`-d`）とログ閲覧。
- [ ] **パフォーマンス最適化 (Phase 4)**: 起動速度とファイル I/O の向上。

---

## 🛠️ 使い方 (Usage)

### 1. インストール (Global Install)
バイナリをビルドした後、システム PATH に追加してどこからでも呼び出せるようにします。

```powershell
go build -o plx.exe cmd/plx/main.go
.\plx.exe install
# ターミナルを再起動すると 'plx' コマンドが有効になります
```

> **注意**: `plx run` を正常に動作させるには、必ず `install` サブコマンドを実行してシステムの PATH に登録・更新する必要があります。

### 2. 環境の初期化 (Setup)
コンテナ実行に必要なバックエンドを自動構築します（デフォルトで Alpine が取得されます）。

```powershell
plx setup
```

### 3. コンテナの実行 (Run)
隔離された空間でコマンドを実行します。

```powershell
# 基本実行
plx run uname -a

# 環境変数の設定 (-e)
plx run -e DATABASE_URL=postgres://localhost:5432 alpine printenv DATABASE_URL

# ポートフォワーディング (-p)
# ホストの 8080 をコンテナの 80 に繋ぐ
plx run -p 8080:80 alpine busybox httpd -f -p 80

# インタラクティブ・シェル（コンテナの中に入る）
plx run -it --image ubuntu bash

# ホストのディレクトリをマウントして実行
plx run -v C:\project:/app --image alpine ls /app
```

### 4. イメージ管理 (Image Management)
好きなディストリビューションをダウンロードして管理できます。

```powershell
# Ubuntu イメージを取得
plx pull ubuntu

# ダウンロード済みイメージの一覧
plx images
```

### 5. コンテナ管理 (Lifecycle)
実行履歴の確認や、不要になった環境の停止・削除が可能です。

```powershell
plx ps
plx stop <container_id>
```powershell
plx rm <container_id>
```

### 6. セルフホスティング (Self-Hosting)
PocketLinx 自身を使って PocketLinx を開発することができます。
開発環境には Native Linux Backend が使用され、コンテナの入れ子実行（Docker-in-Dockerのような構成）が可能です。

```powershell
# 1. 開発用イメージのビルド
plx build .

# 2. 開発環境の起動
plx run -it --image . bash

# --- ここからコンテナ内 ---
# 3. Linux用バイナリのビルド
go build -o plx_linux ./cmd/plx

# 4. セットアップ（Native Linux Backendが自動選択されます）
./plx_linux setup

# 5. 入れ子コンテナの実行
./plx_linux run -it alpine /bin/sh
```

---

## 📦 プロジェクト設定 (plx.json)
プロジェクトのルートに `plx.json` を置くことで、オプションを省略できます。

```json
{
  "image": "ubuntu",
  "mounts": [
    { "Source": ".", "Target": "/app" }
  ]
}
```
このファイルがあるフォルダで `plx run bash` を叩くと、自動的に Ubuntu で起動し、カレントディレクトリが `/app` にマウントされます。

---

## 🛣️ ロードマップ (Roadmap)

1.  **Phase 1: Foundation (Done)**
    - WSL2 基盤の安定化、OS起動エラーの完全解消、ネットワーク設定の固定化。
2.  **Phase 2: Management (In Progress)**
    - `stop`, `ps`, `rm` の実装によるコンテナ・ライフサイクルの完全制御。
3.  **Phase 3: Daemon & Logs**
    - コンテナのバックグラウンド実行と、切り離された環境のログ監視機能。
4.  **Phase 4: Polish**
    - エラーメッセージの洗練、ドキュメントの充実、CLI UX の向上。

## 🏗️ 内部アーキテクチャ (Internal Architecture)

- **`cmd/plx/`**: CLI エントリポイントおよびサブコマンドのルーティング。
- **`pkg/container/`**: プロビジョニング、名前空間の隔離、データ管理のビジネスロジック。
    - `wsl_backend.go`: WSL2 固有の実装（`mknod`, `unshare`, ネットワーク設定）。
    - `df_parser.go`: Dockerfile パーサー。
- **`pkg/shim/`**: コンテナ起動スクリプト (container-shim) の管理。`WORKDIR` 変更や環境変数の注入を担当。

---

## 🛡️ ライセンス & ビジネスパートナー募集 (License & Partnership)

### 🇯🇵 日本語
#### 個人・非営利利用
個人での学習、オープンソースプロジェクト、および非営利目的での利用に関しては、**MIT ライセンス**に基づき、自由に無償でご利用いただけます。

#### 商用利用 & ビジネスパートナー
法人での業務利用や、本ソフトウェアを用いた収益活動については、別途合意が必要です。作者（@takehisa-nanba）は「最高の技術を作る」ことに情熱を注いでいますが、同時に**「この技術をいかに市場に広め、価値を最大化するか」という知見をお持ちのビジネスパートナーを募集しています。**

「PocketLinx でこんなビジネスができる」「面白いマネタイズのアイディアがある」という方は、ぜひ GitHub の Issue やメールでコンタクトしてください。相乗り、大歓迎です！

---

### 🇺🇸 English
#### For Individuals & OSS Developers
For personal learning, open-source projects, and non-commercial use, this software is released under the **MIT License**. Feel free to use, modify, and explore!

#### Commercial Use & Business Partnership
For commercial or enterprise use, or if you intend to generate revenue using PocketLinx, prior agreement is required. 

**I am actively looking for business partners!** While my focus and passion lie in engineering the best possible container technology, I am eager to collaborate with those who have expertise in **growth, marketing, and monetization strategies.** 

If you see a business opportunity here or have a brilliant plan to scale PocketLinx, let's talk. I'm looking for partners who want to build something big together. Reach out via GitHub Issues or email!

---

## 🛡️ 免責事項 (Disclaimer)
MIT License
