# PocketLinx

**PocketLinx** は、どこでも一貫したLinux開発環境を持ち運べるように設計された、軽量でポータブルなコンテナランタイムです。
Go言語とWSL2を利用して、Docker Desktopのオーバーヘッドなしに、隔離されたAlpine Linuxコンテナを作成します。

**PocketLinx** is a lightweight, portable container runtime environment designed to carry a consistent Linux development environment anywhere.
Using Go and WSL2, it creates isolated Alpine Linux containers without the overhead of Docker Desktop.

---

## 🇯🇵 日本語 (Japanese)

### プロジェクトの哲学
- **軽量 & シンプル**: 重いデーモンは不要。バイナリ1つとrootfsだけで動きます。
- **ポータブル**: WSL2が有効なWindowsマシンならどこでも動作します。
- **深い隔離**: Linux Namespace (PID, Mount等) と `chroot` を使用して環境を隔離します。

### 前提条件 (現在の実装)
- **Windows 10/11** + **WSL2** (バックエンドとして利用)
- *※将来的に Linux / macOS へのネイティブ対応も視野に入れた設計を目指しています。*

###Usage / 使い方
#### 1. セットアップ
環境を初期化します。現在はWindows/WSL2環境をターゲットに、Alpine Linux環境を構築します。

```powershell
go run cmd/plx/main.go setup
```

#### 2. コマンド実行 (Run)
隔離されたコンテナ内でコマンドを実行します。実行ごとに、新しい一時的な環境（使い捨て）が作成されます。

```powershell
go run cmd/plx/main.go run uname -a
go run cmd/plx/main.go run ps aux
```

---

## 🇺🇸 English

### Project Philosophy
- **Lightweight & Simple**: No heavy daemons. Just a binary and a rootfs.
- **Portable**: Works on any Windows machine with WSL2.
- **Deep Isolation**: Uses Linux Namespaces (PID, Mount, etc.) and `chroot` for isolation.

### Prerequisites (Current Implementation)
- **Windows 10/11** with **WSL2** enabled.
- *Goal: Native support for Linux and macOS in future iterations.*

### Usage
#### 1. Setup
Initialize the environment. Currently targets Windows/WSL2 to build the Alpine-based environment.

```powershell
go run cmd/plx/main.go setup
```

#### 2. Run Commands
Execute commands in an isolated container. Each run creates a fresh, ephemeral environment.

```powershell
go run cmd/plx/main.go run uname -a
go run cmd/plx/main.go run ps aux
```

## Internal Architecture
- **`cmd/plx/`**: CLI entrypoint and subcommand routing.
- **`pkg/wsl/`**: Abstraction layer for WSL interaction (exec, path conversion).
- **`pkg/container/`**: Business logic for provisioning and namespace isolation.
- **`pkg/shim/`**: Management of the container boot script.

## License
MIT
