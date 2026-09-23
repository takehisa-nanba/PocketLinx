# 環境パッケージ v1：形式と保存・復元の契約

状態：実験的なLinux/amd64向け実装。Windows/WSL連携、既存ランタイムとの統合、配布プロトコルは未実装。

## 目的と範囲

停止済みの単一環境について、構成情報、rootfs、ソース、明示した永続データを完全保存し、新しいディレクトリへ復元する。Go共通パッケージ `pkg/environment` と独立CLI `cmd/plx-env` で提供する。既存の `plx`、GUI、Compose、Backendは変更しない。

既存Backendが安全に環境を停止し、以下の準備済みディレクトリを提供する連携は後続作業。今回の実装はDocker、runc、常駐サービスを必要としない。既存shimを正式な実行基盤として承認するものでもない。

## 入力ディレクトリ

```text
environment/
  environment.json
  rootfs/                 Linuxユーザーランド
  source/                 受け渡すソース
  volumes/                受け渡すデータだけを置く
    data/
```

4項目は必須。sourceとvolumesは空でもよい。他のトップレベル項目はエラーとする。既存コンテナの保存ディレクトリを直接指定する形式ではない。

`environment.json` の例：

```json
{
  "version": 2,
  "os": "linux",
  "arch": "amd64",
  "command": ["/bin/sh", "/workspace/hello.sh"],
  "workdir": "/workspace",
  "uid": 1000,
  "gid": 1001,
  "env": {"GREETING": "hello", "DATA_FILE": "/data/message.txt"},
  "source": {"target": "/workspace"},
  "volumes": [{"name": "data", "target": "/data"}]
}
```

コマンドは引数配列のまま保持する。環境変数はシェル評価しない。workdirはコンテナ内の絶対パス。UID/GIDは環境内の実行ユーザーであり、ホストアカウントを表さない。IP、PID、ホスト絶対パス、ホストの環境変数を自動収集しない。

Definition v2では `source.target` と `volumes[].name/target` に実行時の配置を明示する。ホスト側のパスは保存しない。sourceは必須、volumesは省略可能。uid/gidは明示的な数値が必須で、省略・nullは拒否する。旧Definition v1の保存・復元は維持するが、runでは拒否する。アーカイブの識別子は引き続き `pocketlinx.environment.v1` であり、Definitionのバージョンとは独立する。旧実装はDefinition v2を拒否するため、受信側にも更新が必要。

配置先は正規化済みの絶対パスとし、ルート、`/proc`・`/sys`・`/dev`とその配下、重複・親子関係を拒否する。volume名は英数字で始まる英数字・ピリオド・アンダースコア・ハイフンのみとし、重複を拒否する。未指定volumeも保存対象だが、実行時には接続しない。

実行は独立した `pkg/envrun` が担当する。rootfs内の配置先には事前に空の実ディレクトリを用意する。リンク経由や空でない配置先、欠けたvolumeは実行前に拒否し、自動作成・上書きしない。実験用Backendは専用mount/PID名前空間内でrootfsを読み取り専用、sourceと指定volumeを読み書き可能として接続する。書き込みは元のsource/volumesへ直接残る。実行UID/GIDに必要なファイル権限は構築時に用意し、runで所有者を自動変更しない。

## 保存形式

拡張子の推奨は `.plxenv`。内容は**非圧縮tar**とし、差分・外部ベース参照を持たない。圧縮CPU負荷と追加依存を避け、サイズや展開上限を明確にする初期設計である。圧縮形式より配布容量が大きい点は測定対象とする。

1. 最初のtarメンバーは `manifest.json`。
2. マニフェストは `format: "pocketlinx.environment.v1"` と `entries` を持つ。
3. entriesの順にペイロードを格納し、親ディレクトリは子より先とする。
4. 最後にtarの終端ゼロブロック2個を置く。追加データや連結アーカイブを拒否する。

各エントリー：

| フィールド | 意味 |
| --- | --- |
| path | パッケージ内の正規化済み相対パス。区切りは `/` |
| type | `directory` / `file` / `symlink` |
| mode | 数値のパーミッション。ディレクトリのsticky bitを含む |
| uid, gid | Linuxファイルの数値所有者 |
| mtime | Unix秒単位の更新時刻 |
| size | 通常ファイルのバイト数 |
| sha256 | 通常ファイル全体のSHA-256、小文字16進数 |
| link | シンボリックリンクの元の相対ターゲット |

実行設定も `environment.json` の通常ファイルとしてハッシュ対象にする。tarヘッダーとマニフェストのパス・型・属性が一致しない場合は復元しない。

チェックサムは破損・不整合の検出用であり、配布者の認証や悪意ある作成者の検出を保証しない。署名は後続検討。インポート処理はコード・フック・スクリプトを実行しない。

## 対応属性と制限

- 通常ファイル、ディレクトリ、同じコンポーネント内に収まる相対シンボリックリンクに対応。
- リンクを実際にたどってファイルを書き込まない。リンク連鎖と `..` を仮想的に解決して範囲を確認し、ループを拒否する。
- 絶対シンボリックリンクとコンポーネント外へのリンクは拒否する。従って、任意の既存Linux rootfsをそのまま保存できるとは限らない。Alpine最小ユーザーランドのサンプルは対応範囲に合わせて構築する。
- ハードリンクは通常ファイルとして内容を複製する。リンクによるinode共有は保存しない。入力tarのハードリンクメンバーは拒否する。
- UID/GID、パーミッション、秒単位mtimeを復元する。所有者を設定する権限がない場合は失敗し、黙って変更しない。
- setuid/setgid、デバイス、FIFO、ソケット、xattr、ACLは拒否する。マイクロ秒以下のmtime、atime、ctime、スパース配置、ファイルシステム固有フラグは保存対象外。
- rootfs/proc、rootfs/sys、rootfs/devは空でなければ拒否する。入力ディレクトリ内のマウントポイントは `/proc/self/mountinfo` で検出して拒否する。
- メタデータに未知のフィールド・未知の形式バージョンを含む場合は拒否する。
- 初期BackendはLinux/amd64のみ。その他のOS/CPUは明示的な未対応エラー。

標準上限は通常ファイル合計8 GiB、100,000エントリー、manifest 16 MiB、environment.json 1 MiB。展開前に検証する。APIではLimitsを指定可能、CLIでは `--max-bytes` と `--max-entries` を指定可能。

## 保存の契約

1. 呼び出し元が全プロセスと外部の書き込みを停止する。`SaveOptions.Stopped` / `--stopped` はその明示的な申告であり、稼働プロセスの自動検出ではない。
2. sourceと保存先の親ディレクトリは、呼び出し元が管理する信頼できるディレクトリとする。処理中に別プロセスが移動・交換しない。
3. sourceディレクトリをflockでロックする。協調しない外部ライターを強制停止できる仕組みではない。
4. source内は変更しない。読み取りによってファイルシステムのatimeが更新される可能性はある。
5. インベントリとハッシュを作成し、書き出し時・書き出し後に変更を検査する。ファイルは複数回読むため、保存時間とI/O量は最適化の余地がある。
6. source外の保存先と同じファイルシステムに一時ファイルを作る。全検証とsync成功後、ハードリンクで上書きなしに公開する。
7. 既存出力は上書きしない。失敗時は一時ファイルを削除し、成功パッケージを公開しない。

呼び出し元による停止が前提であり、変更検査だけで任意の並行書き込みに対するトランザクション整合性を保証しない。DB等は後続で停止またはアプリ固有の整合性確保を追加する。

## 復元の契約

1. 指定先は未存在で、親は既存の信頼できるディレクトリであること。親パスにシンボリックリンクがあれば拒否する。
2. マニフェスト全体、形式、パス、型、親子関係、リンク、サイズ上限を確認する。
3. 保存先と同じ親に0700の一時ディレクトリを作り、ファイルを0600で作成する。
4. 通常ファイルを書きながらハッシュを確認する。指定外エントリー、重複、途中切断、末尾追加、ヘッダーの不一致を拒否する。
5. 全ペイロードと実行設定の検証後にリンクを作成し、子から順に属性を反映する。
6. Linux `renameat2(RENAME_NOREPLACE)` で一時ディレクトリを確定する。検証中に指定先が作られた場合も上書きしない。未対応カーネル／ファイルシステムでは失敗する。
7. 通常のエラーでは一時領域を削除する。既存環境へ上書き復元する機能は設けない。

電源断やSIGKILL後の残留一時ファイルの自動回収・完全なクラッシュ耐久性は未実装。処理は常駐しないため、運用上は `.plx-save-*` / `.plx-restore-*` の残留を確認できるようにする。公開済み環境を自動削除しない。

## 秘密情報

専用の準備済みディレクトリを明示的な保存対象とする。ホームやホスト環境変数、隣接ファイルを自動取得しない。ただし、指定したrootfs/source/volumesやenvironment.jsonの値に秘密が含まれていれば保存される。任意ファイル内の秘密検出や自動除外は未実装。秘密を含む環境の配布を本試作の用途にしない。外部秘密注入のランタイム連携は後続作業。

## 操作例

Linux/amd64、Go 1.23以上で：

```sh
go build -o /tmp/plx-env ./cmd/plx-env
/tmp/plx-env save --stopped /path/to/prepared-environment /path/to/example.plxenv
/tmp/plx-env restore /path/to/example.plxenv /path/to/new-environment
# 信頼できるサンプル専用。Linuxのmount/chroot/setuid/setgid権限が必要。
/tmp/plx-env run /path/to/new-environment
```

環境を停止して変更したファイルを再保存すると、変更・削除を含む完全パッケージが作られる。同じLinux上での再保存をテストしているが、Windows↔Linuxの往復は未検証。

テスト：

```sh
go test ./...
go vet ./...
go test ./pkg/environment -run '^$' -fuzz FuzzRestore -fuzztime 10s -parallel 2
go test ./pkg/environment -run '^$' -bench BenchmarkSaveRestore -benchmem
# 使い捨てのAlpine/amd64テスト環境のみ。rootと名前空間・mount等の権限が必要。
sh scripts/verify-environment.sh
```

実行テストのchrootは既知の小さなテスト用スクリプトを動かすためのハーネスであり、正式なコンテナ隔離実装ではない。

実行契約・検証結果・未検証事項は [環境実行の実装報告](environment-execution-report.md) を参照。
