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

### 前提条件
- Windows 10/11
- WSL2 が有効であること

### 使い方

#### 1. セットアップ
環境を初期化します。Alpine Linuxのrootfsをダウンロードし、WSLディストリビューションを登録します。

```powershell
go run main.go setup
```

#### 2. コマンド実行 (Run)
隔離されたコンテナ内でコマンドを実行します。実行ごとに、新しい一時的な環境（使い捨て）が作成されます。

```powershell
go run main.go run uname -a
go run main.go run ps aux
```

---

## 🇺🇸 English

### Project Philosophy
- **Lightweight & Simple**: No heavy daemons. Just a binary and a rootfs.
- **Portable**: Works on any Windows machine with WSL2.
- **Deep Isolation**: Uses Linux Namespaces (PID, Mount, etc.) and `chroot` for isolation.

### Prerequisites
- Windows 10/11
- WSL2 enabled

### Usage

#### 1. Setup
Initialize the environment. This downloads the Alpine rootfs and registers the WSL distribution.

```powershell
go run main.go setup
```

#### 2. Run Commands
Execute commands in an isolated container. Each run creates a fresh, ephemeral environment.

```powershell
go run main.go run uname -a
go run main.go run ps aux
```

## Internal Architecture
- **Host CLI (Go)**: Manages container lifecycle, UUID generation, and WSL interaction.
- **Backend (WSL2)**: Uses a custom `u-container` distro based on Alpine Linux.
- **Isolation**:
  - **Filesystem**: Each container gets a unique copy of the rootfs in `/var/lib/pocketlinx/containers/<UUID>`.
  - **Namespace**: `unshare` is used to isolate PID, Mount, UTS, and IPC namespaces.

## License
MIT
