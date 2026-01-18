# PocketLinx (plx)

**PocketLinx** は、どこでも一貫した Linux 開発環境を持ち運べるように設計された、超軽量でポータブルな次世代のコンテナランタイムです。
Windows 環境において、Docker Desktop のような重いデーモンを必要とせず、WSL2 のパワーを最大限に引き出した隔離環境を瞬時に提供します。

---

## 🚀 主な機能 (Features)

- **シングルバイナリ**: `plx.exe` ひとつで動作。複雑な依存関係はありません。
- **インスタント・セットアップ**: `plx setup` 一発で Linux 環境（Alpine/Ubuntu）が整います。
- **マルチ OS サポート**: 複数の Linux ディストリビューションを瞬時に切り替え可能。
- **ディープ・アイソレーション**: Linux Namespace (PID, Mount, UTS 等) を活用した本格的な隔離。
- **ポータブル・データ**: イメージや設定は `%USERPROFILE%\.pocketlinx` で一元管理。
- **プロジェクト・コンフィグ**: `plx.json` でプロジェクトごとの環境設定を自動化。

---

## 🛠️ 使い方 (Usage)

### 1. インストール (Global Install)
バイナリをビルドした後、システム PATH に追加してどこからでも呼び出せるようにします。

```powershell
go build -o plx.exe cmd/plx/main.go
.\plx.exe install
# ターミナルを再起動すると 'plx' コマンドが有効になります
```

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
実行履歴の確認や、不要になった環境の削除が可能です。

```powershell
plx ps
plx rm <container_id>
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

## 🏗️ 内部アーキテクチャ (Internal Architecture)

- **`cmd/plx/`**: CLI エントリポイントおよびサブコマンドのルーティング。
- **`pkg/wsl/`**: WSL インタラクション（実行、パス変換）の抽象化レイヤー。
- **`pkg/container/`**: プロビジョニング、名前空間の隔離、データ管理のビジネスロジック。
- **`pkg/shim/`**: コンテナ起動スクリプト (container-shim) の管理。

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
