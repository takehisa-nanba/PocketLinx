# Definitionからの環境再現・実行：実装仕様と検証報告

2026-09-23。作業ブランチ：`codex/environment-bundles`。mainへの変更・マージなし。

## 結果と範囲

Linux/amd64の実験用CLI `plx-env run DIRECTORY` を追加した。保存前・復元後とも、保存されたDefinitionだけからcommand、workdir、env、uid/gid、source・volume配置を再現する。実行時に設定値を引数で補う仕組みは設けていない。

Alpineの使い捨てLinuxコンテナで、保存→復元→実行→sourceとvolumeの変更→再保存→再復元→再実行が通った。実行プロセスによるsource/volumeへの書き込みも再保存に含まれることを確認した。ネイティブLinuxホスト、Windows↔Linux、PocketLinxのWSLアダプターは未検証・未実装。

## 変更ファイル

| ファイル | 変更内容 |
| --- | --- |
| `pkg/environment/format.go` | Definition v2のsource/volume配置と検証 |
| `pkg/environment/definition.go`（新規） | 厳格なDefinition読み取り、準備済み環境の検査 |
| `pkg/environment/bundle.go` | Definition読み取りを共通関数に委譲。実行処理は追加しない |
| `pkg/envrun/run.go`（新規） | Backend契約、Definitionからの実行計画、ロックと事前検査 |
| `pkg/envrun/chroot_linux_amd64.go`（新規） | 交換可能な実験用chroot Backendと専用ヘルパー |
| `pkg/envrun/chroot_unsupported.go`（新規） | 未対応OS/CPUの明示的エラー |
| `pkg/envrun/run_linux_amd64_test.go`（新規） | 設定・異常系・ロック・非特権テスト |
| `pkg/envrun/integration_linux_amd64_test.go`（新規） | Alpineでの実行・往復・権限・引数等の統合テスト |
| `cmd/plx-env/main.go` | runと内部ヘルパー起動、終了コードの伝播 |
| `examples/environment/environment.json`, `hello.sh` | v2配置、非root実行、Definitionから渡されたデータパスの利用 |
| `scripts/verify-environment.sh` | 手動chroot・コピー・実行設定を排除して統合テストを呼ぶ |
| `.github/workflows/environment.yml` | 新パッケージのraceテスト、実行・権限不足のCIジョブ |
| `docs/environment-bundle-v1.md` | 更新した形式と実行契約 |
| `docs/environment-execution-report.md`（本書） | 実装・結果・制限 |

既存のGUI、Compose、ネットワーク、Backend、`plx`は変更していない。既存作業のREADME変更とreview-specification.mdは今回の変更に含めない。

## Definitionと配置の契約

アーカイブ構造（manifest.json、environment.json、rootfs、source、volumes）とSHA-256方式は変更しない。新しいDefinitionは `version: 2`。追加項目は `source: {target}` と `volumes: [{name, target}]`。sourceは必須、volumesは省略可能。ホスト絶対パスは格納しない。従来v1は保存・復元可能だが実行不可。自動的な暗黙の配置補完は行わない。

`source` → source.target、`volumes/<name>` → volume.targetを実行時に接続する。配置先はrootfs内の既存の空ディレクトリでなければならない。リンク経由、重複、親子関係、ルート、proc/sys/devへの配置を拒否する。宣言されたvolumeは必須。未宣言のvolumeは保存するが接続しない。

uid/gidはv2で明示必須。環境内部の数値IDとして使い、ホストアカウントへ読み替えない。ファイルの所有権は保存・復元契約に従う。書き込み権限は環境の作成側が用意し、runによる自動chownはしない。

## 実行手順

1. 環境ディレクトリを正規化し、Saveと共通のflockを取得する。
2. 全体の構造、リンク、属性、定義、配置元・配置先、実行ファイル、workdirを検査する。未知JSONフィールドも拒否する。
3. DefinitionからPlanを生成し、Backendへ渡す。保存パッケージはBackendを参照しない。
4. 実験用Backendは新しいmount/PID名前空間を持つ専用ヘルパーを起動する。ヘルパーでも環境とロック対象を再確認し、mount伝播をprivateにする。
5. rootfsは読み取り専用・nosuid・nodev、source/volumesは読み書き可能・nosuid・nodevでbind mountする。コピーや書き戻しはしない。
6. 子プロセスへchroot、uid/gid、空の補助グループ、workdir、argv、envを適用し実行する。適用失敗はエラーであり、別ユーザーへフォールバックしない。
7. 終了時に専用名前空間と子孫プロセスを終了し、ロックを解放する。非ゼロ終了コードはCLIまで伝播する。

command[0]は環境内の正規化済み絶対パスに限定し、ホストPATHを検索しない。command配列をそのままexecへ渡す。envはDefinitionにある値のみを渡し、空の場合もホストenvを継承しない。シェル展開や文字列連結はしない。Definitionが `/bin/sh` を明示した場合に限り、その指定プログラムとしてシェルが動く。

## 実行基盤の制限

このchroot Backendは信頼できる小さなテスト専用ハーネスであり、本番の隔離境界ではない。rootとmount/chroot/setuid/setgid等の権限が必要。ネットワーク隔離、能力の完全な剥奪、seccomp、cgroups等は実装していない。rootの任意プログラムを安全に閉じ込める保証はない。runcの正式採用は未決定。

処理中に外部からディレクトリやファイルを交換・変更しない信頼済み環境が前提。flockは協調的な排他であり、悪意ある並行変更を防がない。run前検査は現在の内容を検査するもので、復元後のファイルが元のbundleと同一であることを保証する認証処理ではない。実行後の意図した変更は許容する。

絶対リンク、xattr、ACL、setuid/setgid、特殊ファイル等の既存制限は維持。サンプルのBusyBoxとmuslでは拡張の必要はなかった。一般的なrootfsの適合性は未検証。

## 実施した検証

検証環境：Windows上のDocker Desktop Linuxエンジン、WSL2カーネル6.6.114.1、Alpine 3.22.1/amd64、Go 1.23.12。これは独立したネイティブLinuxホストの検証ではない。

| 検証 | 結果 |
| --- | --- |
| `go test ./...` | 成功。既存の保存・復元テストを含む |
| `go vet ./...` | 成功 |
| `go test -race ./pkg/environment ./pkg/envrun ./cmd/plx-env` | 成功 |
| `FuzzRestore`（10秒指定、2 worker） | 成功。53回実行、約11秒。長時間ファジングの代替とはしない |
| 非root（1000:1000）のenvironment/envrun/CLIテスト | 成功。rootが必要な実行は明示エラー |
| Linux既存plxビルド、Windows/amd64のplx-envクロスビルド | 成功。Windowsでの実行成功を意味しない |
| Definition往復統合テスト | 成功。保存前後同一出力、source/volume更新が再保存後にも残る |
| 配置先変更・引数・env・ID | 成功。別名・空白入り配置、空引数、引用符・シェル記号を保持。ホストenvを継承せず、補助グループも残さない |
| CAP_SETUIDを除いた実行 | 成功（期待どおり失敗）。別IDで実行しない |
| rootfs書き込み・終了コード・後始末 | 成功。rootでも読み取り専用のため書き込み失敗、exit 37を伝播、失敗後の保存も成功 |
| 異常系の事前検査 | 成功。command/workdir欠落、不正・重複・親子target、volume欠落、未知フィールド、uid/gid省略・null、不完全環境、リンク・空でない配置先を拒否 |

異常系ではBackendが呼ばれず、ホストの検査用ファイルが変わらないことを確認した。実行テストの設定はサンプルDefinitionから取得する。ハーネスにGREETING値やworkspace/data配置、実行コマンドを補完する処理はない。

CI定義は更新したが、今回の変更は未pushのためGitHub Actionsで未実行。既存コミット `d2d1f605ee5929cf1cfc26fd333bc612205b14b7` の [前回CI](https://github.com/takehisa-nanba/PocketLinx/actions/runs/35862402516) は成功しているが、今回の変更のCI結果としては扱わない。

## 依存関係と軽量性

新規Goモジュール依存はない。Go標準ライブラリとLinuxカーネル機能を使用する。Dockerは今回の検証設備であり、製品の実行依存ではない。テストのrace用gcc/musl-devも製品依存に含めない。

常駐プロセスは追加しない。runごとにGoヘルパーを1プロセス追加する。source/volumeのコピーはしないが、検証のため親・ヘルパーでそれぞれ環境全体を読み取りハッシュ計算するため、データサイズに比例した起動I/Oが発生する。大きい環境への最適化は後続。

同じGo/Alpine環境でCLIサイズは前回3,886,723バイトから4,209,914バイトへ323,191バイト（約8.3%）増加。今回の統合スイート測定はwall 1.98秒、peak RSS 97,280 KiB。ただしgo testのコンパイル等を含むスイート全体の値であり、PocketLinx実行の起動時間・メモリとはみなさない。WSL2/実行基盤を含むメモリ・ディスク総量やDockerとの同条件比較は未実施。軽量化を達成したとは判定していない。

## 次段階と未検証事項

- ネイティブLinuxで名前空間・mount・資格情報・ロック・終了処理を実機確認する。異なるカーネル、ファイルシステム、LSM、シグナルや強制終了条件も検証する。
- 本番用Backendの隔離契約と権限モデルを決め、既存実行処理とruncを比較する。検証用chrootをそのまま正式採用しない。
- Windows/WSL側でLinux内の配置・所有者・パスを保つアダプターを実装してから双方向往復を検証する。
- 実際の停止判定と排他、既存Backend/CLI接続、ホストとのUID/GID対応を設計する。
- 同一ホスト・同一環境で起動時間、総メモリ、総ディスクを測り、旧PocketLinxとDockerと比較する。
- 大規模環境、rootfsの書き込みを必要とする開発ツール、追加のファイル属性の必要性を調べる。今回の読み取り専用rootfsでは環境内のパッケージ追加はできない。
