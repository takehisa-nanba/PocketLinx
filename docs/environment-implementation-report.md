# 保存・復元の初期実装：結果と未検証事項

実施日：2026-09-23

作業ブランチ：`codex/environment-bundles`

比較元：`1e503a0`。既存のGoコード、go.mod、go.sum、GUI、Composeは変更していない。作業中に別途存在したREADMEの文言変更と、前回の `docs/review-specification.md` は今回の実装に含めない。

## 追加ファイル

| ファイル | 内容 |
| --- | --- |
| `pkg/environment/format.go` | 共通の定義、マニフェスト、検証、制限値 |
| `pkg/environment/bundle.go` | 保存、復元、ハッシュ検証、失敗時の後片付け |
| `pkg/environment/platform_linux_amd64.go` | Linuxの所有者、属性、マウント確認、ロック、上書きしない確定処理 |
| `pkg/environment/platform_unsupported.go` | 未対応OS/CPUの明示的なエラー |
| `pkg/environment/bundle_linux_amd64_test.go` | 正常系・異常系・安全性の回帰テスト |
| `pkg/environment/fuzz_linux_amd64_test.go` | 不正なアーカイブ入力のファジング |
| `pkg/environment/benchmark_linux_amd64_test.go` | 同じ条件で繰り返せる保存・復元ベンチマーク |
| `cmd/plx-env/main.go`, `main_test.go` | 独立CLIと引数検証テスト |
| `examples/environment/environment.json`, `hello.sh` | Alpineの小さなシェルサンプル |
| `scripts/verify-environment.sh` | 使い捨てAlpine環境での実行確認・簡易測定 |
| `.github/workflows/environment.yml` | Linuxテスト、race検査、ファジング、クロスビルド、Alpine検証のCI定義 |
| `.gitattributes` | 追加した2つのシェルスクリプトだけをLF改行に固定 |
| `docs/environment-bundle-v1.md` | 形式と保存・復元の契約、使用方法 |
| `docs/runtime-evaluation.md` | 既存ランタイムとruncの比較、採用保留の理由 |
| 本書 | 実施結果と後続課題 |

## 実装内容

完全保存の非圧縮tarに、manifest.json、environment.json、rootfs、source、volumesを格納する。通常ファイルのSHA-256、属性、サイズ、エントリー数を検証する。

保存元は読み取りだけで処理し、既存の出力ファイルを上書きしない。復元は専用の一時ディレクトリで行い、全検証後にRENAME_NOREPLACEで確定する。失敗時は元の環境と既存の保存先を保持する。

停止済みであることは呼び出し元の責任。既存Backendの停止・PID管理と接続していないため、CLIの `--stopped` は申告であり稼働判定機能ではない。

現時点ではLinux/amd64、対応属性の範囲内で利用する実験機能。任意のrootfsを透過的に移行する完成品ではない。絶対シンボリックリンク、xattr/ACL、setuid/setgid、特殊ファイルはエラーになる。詳細は形式仕様を参照。

## 実施した検証

ホストはWindows 11 Home（10.0.26200）、AMD Ryzen 5 7530U、物理メモリ16,486,756,352 bytes。

GoはホストのPATHになかったため、既に起動していたDocker DesktopのLinuxエンジン29.8.0をテストに使用した。Linuxテスト環境はGo 1.23.12、Alpine 3.22.1、amd64、カーネル6.6.114.1-microsoft-standard-WSL2。

使用イメージ：`golang:1.23-alpine@sha256:383395b794dffa5b53012a212365d40c8e37109a626ca30d6151c8348d380b5f`。イメージのローカルサイズは370,003,695 bytes。これは開発・テスト用の依存であり、保存・復元製品の実行依存ではない。Goモジュールの追加依存は導入していない。

| 検証 | 結果 |
| --- | --- |
| `go test ./...` | 成功。既存パッケージにはテストなし |
| `go vet ./...` | 成功 |
| Linux版の既存CLI・新CLIビルド | 成功 |
| Windows/amd64への既存CLI・新CLIクロスビルド | 成功。保存・復元のWindows対応を意味しない |
| 保存→復元の内容・属性比較 | 成功 |
| 変更・削除・永続データ更新後のローカル再保存 | 成功 |
| 保存元の内容・属性の不変性 | 成功。atimeは比較対象外 |
| 破損、欠落、終端切断、末尾追加、ヘッダー不一致 | 拒否し、保存先を公開しない |
| パストラバーサル、絶対パス、リンク連鎖、リンクを親にした書き込み | 拒否 |
| 既存ファイル・ディレクトリ・確定時に作られた保存先 | 上書きしない |
| 所有者、実行権限、sticky directory、長いファイル名、日本語、相対リンク | 成功 |
| 非特権UID/GID 1000の同所有者での保存・復元 | 成功 |
| 非特権ユーザーによる復元時のchown失敗 | 失敗を返し、一時領域を削除 |
| Linux/amd64パッケージのカバレッジ | root実行で82.2%。全分岐の検証を意味しない |
| ファジング | 10秒の試行と、正常パッケージseed追加後の5秒試行が成功。短時間のスモーク検証のみ |
| CI定義のリモート実行 | 未実施。workflow追加のみ |
| race検査 | ローカル未実施。CIに定義 |

特権所有者の復元試験はrootで、権限不足の試験はUID 1000で実行した。片方の権限で実行できない試験はその実行でスキップし、もう片方で確認した。

Alpine実行ハーネスでは、同じAlpine環境のBusyBoxとmuslローダーから最小rootfsを構築した。保存前と復元後の使い捨て実行コピーでシェルを動かし、両方が `hello|/workspace|persistent-data` を返した。変更後の再保存・再復元では更新済みデータと追加出力を確認した。

ハーネスは既知のテストデータだけをchrootで実行する。既存のPocketLinxコンテナ起動や、本番向けの安全なランタイム統合の検証ではない。

## 軽量性の参考値

### Linuxテストコンテナ内

`scripts/verify-environment.sh` の測定結果。BusyBox timeの小数2桁表示であり、0.00秒はゼロ時間を意味しない。以下は1回分の参考値で、性能保証やDockerに対する優位性の証明ではない。

| 操作・対象 | 値 |
| --- | --- |
| 旧CLIの `version` | 表示0.00秒、プロセス最大RSS 5,120 KiB |
| 新CLIの保存 | 表示0.01秒、プロセス最大RSS 3,456 KiB |
| 新CLIの復元 | 表示0.02秒、プロセス最大RSS 3,072 KiB |
| 旧Linux CLIバイナリ | 10,187,285 bytes |
| 新Linux保存・復元CLIバイナリ | 3,886,723 bytes |
| サンプルパッケージ | 1,482,752 bytes |
| 保存元／復元先の使用ブロック量 | 各1,480 KiB（du -sk） |

新CLIの方が機能範囲が小さいため、旧CLIとのバイナリサイズ・RSSの差を機能同等の比較として扱わない。

1 MiBペイロードの保存＋復元ベンチマーク（5回）：平均18,712,365 ns/op、56.04 MB/s、239,480 B/op、775 allocs/op。B/opはGoの割り当て量であり、プロセスRSSやWSL全体のメモリではない。出力の削除処理は計測時間外。小規模データの温間参考値であり、多数ファイル・大容量環境の評価は今後必要。

### Windowsホストの旧CLI

変更していない旧GoコードをWindows/amd64へビルドし、`plx-before.exe version` のプロセス起動から終了までを.NET Stopwatchで5回測定した。

- バイナリサイズ：9,907,712 bytes。
- 1回目：3,820.12 ms。
- 2〜5回目：44.27 / 34.27 / 34.00 / 31.41 ms。
- 2〜5回目の中央値：約34.14 ms。

初回の遅延原因は未調査。測定器・OSのキャッシュ等も含まれる。これはCLIのversion経路だけで、WSL・コンテナの起動時間ではない。Windowsのプロセスメモリ、WSL VM全体のメモリ、VHDX容量は未測定。

### 測定できていない項目

- 既存PocketLinxの実コンテナ起動時間・稼働時メモリ：利用可能なWSLディストリビューションはdocker-desktopのみ。pocketlinx環境は構築せず、既知の安全性問題がある旧実行経路を実ホストで動かしていない。
- ネイティブLinuxホスト上の測定：ホストなし。Docker内のLinux測定で代用済みとは扱わない。
- 新旧の環境起動の比較：今回の新機能には実行基盤を統合していない。
- Dockerとの機能同等比較：未実施。Dockerを試験実行に利用したことは、Dockerとの性能比較ではない。
- WSL2・Docker Desktop・常駐プロセスを含む総リソース：未測定。

後続では同じホスト・同じAlpine環境・同じスクリプトとデータで、冷間／温間、停止後アイドル、インポート、保存、起動を分けて複数回測定する。WSL/Dockerの設定とバージョン、他ワークロードの有無、メモリ上限、ホスト増分、ピーク一時ディスクを記録する。総リソースの結果が出るまでは軽量化達成と判定しない。

## 後続作業

1. ネイティブLinuxホストで同じテストを実行し、ファイルシステム・カーネル・権限差を確認する。
2. 既存Backendに停止確認・保存ロック・準備済みディレクトリへの変換を接続する。既存の危険な削除・シェル展開を残したまま接続しない。
3. Windows CLIからWSL内のGo処理を呼ぶアダプターと、バイナリを改行変換しない転送を実装する。
4. Windows→Linux→Windows、Linux→Windows→Linuxの実機往復試験を行う。
5. 絶対リンク、追加属性、秘密情報の外部注入、配布元の検証などを必要性と安全性を確認して拡張する。
6. 既存実行処理とruncを同じ条件で比較し、正式な実行基盤を選ぶ。
7. 大容量・多数ファイルでの測定後に、圧縮、重複排除、キャッシュ改善を検討する。差分配布はその後とする。
