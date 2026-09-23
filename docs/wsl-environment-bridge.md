# Windows／WSL接続：仕様・操作・検証報告

2026-09-23。ブランチ `codex/environment-bundles`。Definition実行機能を再実装せず、既存の `pkg/environment` と `pkg/envrun` を利用するWindows側入口を追加した。

## 完成した範囲

実際のWindowsから独立したAlpine WSL2内のLinux CLIを呼び出し、専用サンプルの保存→Windowsへの転送→WSLへの転送・復元→Definitionによる実行→source/volume変更→再保存→Windows転送→再復元・実行を確認した。Docker Desktop内部ディストリビューションは実行先に使用していない。

対象は信頼できるAlpine専用サンプル。chroot Backendは引き続き検証用であり、任意の配布環境を安全に実行する機能ではない。ネイティブLinux実機との双方向受け渡し、本番隔離、Dockerより軽いという評価は今回の完成条件に含めない。

## 追加・変更ファイル

| ファイル | 責務 |
| --- | --- |
| `pkg/wslbridge/adapter.go` | WSL2・明示ディストリビューション・CLIプロトコル確認、転送と呼び出し |
| `pkg/wslbridge/receiver.go` | ダウンロードのバイナリ受信・SHA-256・サイズ検証 |
| `pkg/wslbridge/system_windows.go`, `system_other.go` | Windowsシステムのwsl.exe呼び出し／未対応OSエラー |
| `pkg/wslbridge/adapter_test.go` | 引数保持、バイナリ転送、破損拒否、選択エラー、上書き防止 |
| `cmd/plx-env/wsl.go` | Windows用の実験的 `wsl` サブコマンド |
| `cmd/plx-env/bridge_linux.go`, `bridge_other.go` | Linux側転送ヘルパーと共通保存・復元・実行への委譲 |
| `cmd/plx-env/bridge_linux_test.go` | 非Linuxファイルシステム・リンク・破損アップロードの拒否 |
| `cmd/plx-env/main.go` | 新サブコマンドへのルーティングだけを追加 |
| `scripts/wsl-fixture/main.go` | 専用Alpineサンプルを構築・変更する検証用ツール |
| `scripts/verify-wsl.ps1` | 実Windows／WSLの再現可能な往復・異常系試験 |
| `.github/workflows/environment.yml` | Windowsアダプター単体テストとLinuxテスト対象追加 |
| `docs/wsl-environment-bridge.md` | 本仕様・操作・検証報告 |

READMEなど既存の別変更は対象外。GUI、Compose、ネットワーク管理、既存PocketLinx Backend、保存形式、Definition、envrunは変更しない。

## 責務分離と保存契約

Windowsはディストリビューション選択、wsl.exeの利用可否、WSL2であること、Linux CLIのプロトコル版を確認する。ディストリビューション名を省略して既定のものを使う処理はない。初期実装で選択可能な名前・ユーザー名は英数字・アンダースコアで始まる英数字・`_ . -` に限定する。`docker-desktop`で始まる名前は拒否する。

`wsl.exe --distribution NAME --user USER --exec /absolute/plx-env ...` へ引数配列を直接渡す。sh/cmd/PowerShellのコマンド文字列を生成しない。旧 `pkg/wsl` の文字列入力・改行変換経路は利用しない。WindowsパスはWindowsプロセスが開き、Linuxへパス変換して渡さない。

Linuxはバイナリ転送の受け渡しと、既存のSave/Restore/Runを担当する。rootfs/source/volumesはLinux側にのみ展開する。格納先は現在ext4/xfs/btrfsに限定し、実際に検証したものはWSLのext4。DrvFS/9p/NTFS、シンボリックリンク経由の格納先は拒否する。Linuxパスは正規化済み絶対パス、親ディレクトリは事前に存在することが必要。Windows側のパッケージ出力先はハードリンクによる非上書き確定が可能なファイルシステムが必要（実測はNTFS）。

Definition v1の保存・復元とv2の実行契約を維持する。ホストパス、ディストリビューション名、Windowsユーザー名はDefinitionに書き込まない。実行時のcommand/env/workdir/uid/gidや配置は既存のDefinitionからのみ取得する。`--user`はWSLヘルパーを起動するLinuxユーザーであり、Definitionの実行UID/GIDの上書きではない。

## バイナリ転送

- Windows→WSL：Windowsで開いた通常ファイルのSHA-256を計算し、同じハンドルを先頭に戻して標準入力へ送る。Linuxは親ディレクトリの一時ファイルへバイト列のまま受信し、SHA-256一致後に既存Restoreを呼ぶ。成功時のハッシュ応答もWindowsで照合する。
- WSL→Windows：Linuxが共通Saveで停止済み環境を一時パッケージ化する。標準出力は64文字のSHA-256＋LF＋パッケージ本体。Windowsはヘッダーと本文を分離して本文を一時ファイルに書き、ハッシュ一致・sync後にハードリンクで非上書き確定する。エラーは標準エラーへ出す。
- パッケージ本文の文字コード・改行変換は行わない。転送上限9 GiB、既存の展開上限8 GiB等は維持する。転送SHA-256は完全性確認であり配布元認証ではない。
- 既存環境・出力ファイルは置き換えない。通常のエラーでは一時ファイルを削除する。成功した復元後に通信が切れた場合、復元先が既に存在する可能性がある。再試行でも上書きせずエラーにする。強制終了・電源断後の一時ファイル回収は未実装。
- Linuxプロセスの非ゼロ終了コードをWindowsまで保持する。実サンプルのexit 37がWindowsでも37となることを確認した。

## ディストリビューションの準備

既存環境にはdocker-desktopだけがあり、PocketLinx用の独立ディストリビューションはなかった。そのため新規 `PocketLinx-Verify` を追加した。既存ディストリビューションの削除・初期化・設定変更はしていない。

[Microsoftのimport手順](https://learn.microsoft.com/en-us/windows/wsl/use-custom-distro)に従い、Alpine 3.22.1 x86_64 minirootfsを公式配布元から取得し、公式SHA-256ファイルと照合してWSL2へimportした。

```text
https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/x86_64/alpine-minirootfs-3.22.1-x86_64.tar.gz
SHA-256: 0e5cc5702ad72a4e151f219976ba946d50161c3acce210ef3b122a529aba1270
```

検証用インストール先：`%LOCALAPPDATA%\PocketLinxVerification\distro`。アーカイブも同じ親に保持した。WSL2は既に有効だったためWindows機能の追加や昇格操作は不要だった。新規PCでWSL2が未導入の場合は別途インストール・必要に応じた管理者権限や再起動が必要。

再現用準備例（未使用のディストリビューション名・インストール先を選ぶ）：

```powershell
wsl --import PocketLinx-Verify C:\WSL\PocketLinx-Verify C:\Downloads\alpine-minirootfs-3.22.1-x86_64.tar.gz --version 2
wsl --distribution PocketLinx-Verify --user root --exec /bin/mkdir -p /opt/pocketlinx /usr/local/bin
```

Go 1.23以上でWindows版 `go build -o plx-env.exe ./cmd/plx-env` とLinux/amd64版を作る。Linux向けビルド時は `GOOS=linux GOARCH=amd64 CGO_ENABLED=0` を指定する。Linux版を独立ディストリビューションの `/usr/local/bin/plx-env` へ配置し755にする。検証用fixtureも同じターゲットで `./scripts/wsl-fixture` からビルドし `/usr/local/bin/plx-wsl-fixture` へ配置する。初回の実行ファイル配置にWindowsドライブを読むことはあっても、環境の展開先には使用しない。

実験用runにはLinux root、mount・chroot・setuid/setgid・名前空間の権限が必要。アダプターはWSLユーザーを既定でrootと明示して起動し、自動sudoや権限不足時のフォールバックは行わない。追加Linuxパッケージのインストールは不要だった。

## Windowsからの操作

フラグは操作名より前に指定する。

```powershell
.\plx-env.exe wsl --distro PocketLinx-Verify check
.\plx-env.exe wsl --distro PocketLinx-Verify restore C:\Bundles\sample.plxenv /opt/pocketlinx/restored
.\plx-env.exe wsl --distro PocketLinx-Verify --trusted-sample run /opt/pocketlinx/restored
.\plx-env.exe wsl --distro PocketLinx-Verify --stopped save /opt/pocketlinx/restored C:\Bundles\changed.plxenv
```

`--trusted-sample`は専用サンプルに限定する操作上の確認であり、パッケージの安全性を自動判定するものではない。`--stopped`も既存と同じ停止申告。既存実行処理とのflock排他は働くが外部ライターは停止しない。信頼済みのディレクトリと停止済みの外部ライターを前提とする。

専用サンプルの一括試験：

```powershell
.\scripts\verify-wsl.ps1 -Distro PocketLinx-Verify -WindowsCLI .\plx-env.exe -LinuxSample /mnt/c/path/to/PocketLinx/examples/environment
```

fixtureはサンプルDefinitionを読んで配置・所有者を準備する開発用ツール。runの設定補完や製品の環境ビルダーではない。毎回新規の検証ディレクトリを作り、確認用成果物を保持する。既存ディストリビューションやデータを削除する処理はない。

## 実測テスト結果

環境：Windows、WSL2カーネル `6.6.114.1-microsoft-standard-WSL2`、独立Alpine 3.22.1/amd64、Go 1.23.12。

| 項目 | 結果 |
| --- | --- |
| 指定WSL2のチェック、Linux CLI起動 | 成功 |
| 専用サンプルの保存・Windows転送・WSL復元・実行 | 成功。Definitionのhello/workdir/uid=1000/gid=1001を再現 |
| source/volume更新・再保存・Windows転送・再復元 | 成功。実行時の永続データ更新とsource生成ファイルも保持 |
| 両方向SHA-256 | 一致。下記参照 |
| 空白・単引用符入りWindows/WSLパス | 成功。シェル文字列の再評価なし |
| 存在しないディストリビューション | 明示エラー |
| nobodyでの実行・復元 | 権限エラー。別ユーザーへフォールバックしない |
| Windowsドライブ内復元、親のない転送先 | 明示エラー |
| 既存パッケージ・環境への上書き | 拒否。既存パッケージのハッシュ、元環境の出力を維持 |
| Linuxのexit 37 | Windowsで37を取得 |
| Linuxヘルパーの壊れた転送ハッシュ・壊れたarchive | 実WSLで拒否、復元先や一時ファイルが残らないことを確認 |
| アダプター単体テスト | Linuxと実Windowsで成功。NUL/非UTF8/CRLF/分割受信、破損、選択エラーを含む |
| `go test ./...`, `go vet ./...` | 成功 |
| environment/envrun/wslbridge/CLIのraceテスト | 成功 |
| 既存Alpineコンテナ統合テスト | 成功。CAP_SETUID不足テストは専用CIステップで継続 |

最終の往復試験で両端が確認した値：

```text
初回: 38f93e8556b67d39dc233db24e4e42ed31462fccdcf8f5c04f133d2fde5f4fd2
再保存: 24312b8f1962a3b666060dbcf51b67db4b5a4bb601d1cc40ab511ec7f69026c0
```

検証成果物：WSL内 `/opt/pocketlinx/verify-637126310af7460b8204e51c7a343086`、Windows `%TEMP%\plx-wsl-b87be2d5e0a140cb8381e0d0802d2230`。最後の終了コード試験ではback環境のスクリプトだけをexit 37へ変更している。再保存したWindowsパッケージはその変更前の往復確認済みデータである。

GitHub ActionsにはLinuxの既存試験とWindowsの単体試験を定義した。Windows CIのビルド・単体試験は実WSL検証の代わりに扱わない。pushしたコミットのCI結果は作業完了報告で別途示す。

## 依存関係・測定・制限

新規Goモジュール依存、アプリ専用常駐プロセスはない。Windows側の必要条件はWSL2と独立ディストリビューション。Linux側は既存CLIとカーネル機能を利用する。WSLのVM／ディストリビューション自体の起動・メモリ・ディスク消費は存在し、アダプターは自動終了や他ディストリビューションの停止を行わない。

| 測定 | 値と解釈 |
| --- | --- |
| Windowsからrun、起動済みWSLで5回 | 323.75 / 280.78 / 283.11 / 285.41 / 273.56 ms、中央値283.11 ms。WSLチェックとCLI呼び出しを含む。コールド起動ではない |
| WSL内CLI単体run、BusyBox time -v | wall約0.03秒、最大RSS 3,456 KiB。プロセス計測でありVM全体や同時合計メモリではない |
| 往復・異常系検証スクリプト全体 | 約5.58秒。製品の単一操作の起動時間ではない |
| Windows CLIサイズ | 4,304,384バイト |
| Linux CLIサイズ | 4,267,652バイト。前段階4,209,914から57,738バイト増加 |
| 専用サンプルのLinuxディスク使用量 | duで1,488 KiB |
| 新規WSL VHDXの測定時ファイル長 | 113,246,208バイト。仮想容量・正確な物理割当量とは異なり、検証データの増加で変化する |

Linux一時パッケージとWindows受信用一時ファイルを使うため、転送中にパッケージ分の追加ディスクが両側に必要。ストリーム全体をメモリへ保持しない。既存runの全ファイル検証のI/Oコストは維持される。

WSL無効化・未インストール状態、WSL1実環境、別ディストリビューション、別アーキテクチャ、容量不足・通信中断・強制終了の実機試験は未実施。WSL利用不可とWSL1選択は単体テストでエラー経路を確認した。WSL全体のメモリ増分、コールド起動、Dockerとの同条件比較は未測定。

次段階ではネイティブLinux実機との往復、ファイル所有権・カーネル差・実際の開発ツールの互換性を検証する。本番隔離Backendと権限モデル、停止判定、障害時回収、総リソース比較は別途必要。runc正式採用、rootfs書き込み対応、保存形式拡張は行っていない。
