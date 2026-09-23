# 独立Linuxとの双方向受け渡し：手順・検証報告

## 結論（2026-09-23）

**Windows／WSL2 ↔ 独立Linuxの実際の双方向試験は未検証であり、今回の最終完成条件は未達成。** 利用者から独立Linuxマシン・VMを利用できない旨の回答を受けた。代替環境を独立Linuxと見なさず、受け渡し手順と検証スクリプトを実装した。

実際に使用したものは既存Windows、既存の独立ディストリビューション `PocketLinx-Verify`（Alpine 3.22.1、WSL2 6.6.114.1-microsoft-standard-WSL2、amd64）、開発テスト用Docker Alpineコンテナ。物理Linux実機・WSLと独立したLinux VMは使用していない。既存ディストリビューションの削除・初期化・再構築はしていない。

WSL内の別ディレクトリを相手側として、Windows起点・Linuxスクリプト起点の両経路を**手順の動作確認（smoke）**として実行した。これは異なるカーネルやLinuxホスト間の互換性の証明ではない。

## 変更ファイルと責務

- `scripts/roundtrip-check/main_linux.go`：属性・内容のスナップショット比較、Definitionによる実行結果検証、変更・削除用マーカー、権限不足試験、ホスト種別記録。
- `scripts/roundtrip-check/main_other.go`：検証ツールの対象プラットフォーム表示。
- `scripts/roundtrip-check/main_linux_test.go`：内容・実行権限・削除を検出する単体テスト。
- `scripts/roundtrip-linux.sh`：Linuxでstart/exchange/finishを実行するオフライン試験手順。
- `scripts/roundtrip-windows.ps1`：既存wslbridgeを使ったWindows側の同じ3段階。
- `scripts/verify-roundtrip-smoke.sh`：使い捨てAlpineコンテナ内での手順動作確認。独立Linux検証ではない。
- `.github/workflows/environment.yml`：新スクリプトをトリガーに追加し、smoke試験を追加。
- `.gitattributes`：追加シェルスクリプトのLFを固定。
- `docs/independent-linux-roundtrip.md`：本書。

`pkg/environment`、`pkg/envrun`、`pkg/wslbridge`、Definition、保存形式、製品CLI、既存Backendは変更しない。ネットワーク転送機能・追加モジュール依存・常駐サービスは追加していない。既存の別作業のREADMEとreview-specification.mdはコミット対象外。

## 検証環境の準備条件

独立Linuxは最初はAlpine/amd64の物理実機または独立VMを推奨する。既存fixtureが `/bin/busybox` と `/lib/ld-musl-x86_64.so.1` から専用rootfsを構築するため、これらが存在する環境が必要。Ubuntu等へそのまま適用したとは扱わない。相手側の設定を変更して無理に同じ挙動へ合わせず、差異は記録する。

必要条件：Linux/amd64、mount/PID名前空間、private bind mount、chroot、setuid/setgid、flock、保存・復元に必要な所有者設定、renameat2(RENAME_NOREPLACE)、ハードリンク。root権限で手順を実行する。実際のサンプルプロセスはDefinitionの非root UID/GIDで動く。環境はLinuxのファイルシステム上（初期検証はext4）、成果物用ディレクトリと環境の親は既存で信頼済み・外部ライターなしとする。

Linuxには既存の `plx-env`、`scripts/wsl-fixture` からビルドした `plx-wsl-fixture`、今回の `scripts/roundtrip-check` からビルドした `plx-roundtrip-check` を `/usr/local/bin/` に配置する。Go 1.23以上、Linux向けは `GOOS=linux GOARCH=amd64 CGO_ENABLED=0` でビルド可能。fixtureとcheckerは開発用ツールであり製品依存ではない。CLI実行ファイルは権限不足試験のため非rootユーザーにも読み取り・実行可能にする。

Windowsには既存のWindows版plx-envと独立WSL2ディストリビューションを使う。WSL側も同じ3バイナリを配置する。ディストリビューション作成は本試験スクリプトでは行わない。サンプルはリポジトリの `examples/environment` を使う。

`host linux-physical` / `host linux-vm` はカーネル名やコンテナマーカー等からWSL／コンテナを検出すると拒否する。`smoke` は手順確認用であり独立環境の証明には使えない。物理／VMの区別は操作者の申告であり、検出だけで完全に証明できるわけではない。VM製品・ゲスト設定、物理マシン情報、カーネル・ファイルシステム情報を試験報告にも記録する。

## 受け渡す成果物

各送信段階が出力するディレクトリをUSB、ファイル共有、成果物ダウンロード等でバイト列のままコピーする。新しいSSH機能等は不要。

```text
package.plxenv              既存形式の環境パッケージ
package.plxenv.sha256       送信時パッケージのSHA-256
package.plxenv.files.json   送信時のファイル属性・内容の検証証跡
package.plxenv.files.sha256 証跡ファイルのSHA-256
package.plxenv.state        original または returned（試験段階のみ）
host.txt                   送信側の役割・ホスト名・カーネル
output.txt                 Definitionから実行した出力
permission-error.txt       権限不足時のエラー
result.txt                 段階ごとの結果
```

ホスト情報は証跡ファイルにだけ保存し、.plxenvやDefinitionには埋め込まない。ホスト名を含む証跡は必要な範囲で共有する。受信スクリプトはパッケージと証跡のSHA-256をそれぞれ照合する。ハッシュ一致は転送中の完全性確認であり配布者認証ではない。意図した編集後は新しいパッケージとなるため、初回と再保存のSHA-256が等しいことは要求しない。

snapshotはrootfs/source/volumes/environment.jsonの通常ファイルSHA-256、Unixファイル種別・mode・UID/GID、相対リンク文字列、パス集合を保存し、復元後かつ実行前に完全比較する。Definitionファイル自体も比較対象であり、command/workdir/env/UID/GID/配置が同じことを確認する。mtime等は今回の追加比較の対象外（既存保存・復元テストで扱う）。環境最上位ディレクトリはパッケージメンバーではないため比較に含めない。

## Windows→独立Linux→Windows

各ROOT・ARTIFACTSは未存在のものを選ぶ。PowerShell 7で、例のパスを実際の配置へ置き換える。

1. Windowsで作成・実行・保存する。

```powershell
.\scripts\roundtrip-windows.ps1 -Stage start -Distro PocketLinx-Verify -WindowsCLI .\plx-env.exe -Root /opt/pocketlinx/w-start -Artifacts C:\Evidence\w-start -Sample /mnt/c/path/to/PocketLinx/examples/environment
```

2. `C:\Evidence\w-start` 全体を独立Linuxの `/evidence/w-start` へコピーする。Linuxで復元・比較・実行・source/volume編集・削除・再保存する。物理Linuxの場合は `linux-vm` を `linux-physical` に置き換える。

```sh
sh scripts/roundtrip-linux.sh exchange linux-vm /opt/pocketlinx/l-received /evidence/l-return /path/to/PocketLinx/examples/environment /evidence/w-start/package.plxenv
```

3. `/evidence/l-return` 全体をWindowsの `C:\Evidence\l-return` へコピーし、戻った環境を復元・検証・実行する。

```powershell
.\scripts\roundtrip-windows.ps1 -Stage finish -Distro PocketLinx-Verify -WindowsCLI .\plx-env.exe -Root /opt/pocketlinx/w-back -Artifacts C:\Evidence\w-back -Sample /mnt/c/path/to/PocketLinx/examples/environment -InputBundle C:\Evidence\l-return\package.plxenv
```

## 独立Linux→Windows→独立Linux

1. 独立Linuxで新しいサンプルを作成する。

```sh
sh scripts/roundtrip-linux.sh start linux-vm /opt/pocketlinx/l-start /evidence/l-start /path/to/PocketLinx/examples/environment
```

2. `/evidence/l-start` 全体をWindowsへコピーする。Windowsで受信して編集・再保存する。

```powershell
.\scripts\roundtrip-windows.ps1 -Stage exchange -Distro PocketLinx-Verify -WindowsCLI .\plx-env.exe -Root /opt/pocketlinx/w-received -Artifacts C:\Evidence\w-return -Sample /mnt/c/path/to/PocketLinx/examples/environment -InputBundle C:\Evidence\l-start\package.plxenv
```

3. `C:\Evidence\w-return` 全体を独立Linuxへ戻す。

```sh
sh scripts/roundtrip-linux.sh finish linux-vm /opt/pocketlinx/l-back /evidence/l-back /path/to/PocketLinx/examples/environment /evidence/w-return/package.plxenv
```

受信側のexchangeでは元のサンプルソースを変更し、volumeデータを変更し、両コンポーネントに作成しておいた削除用マーカーを削除する。変更後実行がsource/generated.txtとvolumeへの書き込みを残し、戻った側で内容と削除状態を検証する。実行コマンドやenvを試験側で補完せず、既存CLIのrunに環境ディレクトリだけを渡す。

各段階は既存パッケージの上書き拒否、受信時は既存復元先の上書き拒否も確認する。checkerは補助試験としてホストUID/GID 65534でCLI起動を試し、明示的な権限エラーと環境不変を確認する。この値をDefinitionのUID/GIDへ代入しない。

## 停止申告と実際の停止確認の責務

- Windows公開CLIの `--stopped` は操作者の停止申告。WSLアダプターはフラグを確認して内部bridge saveを呼ぶが、プロセス一覧や稼働状態を確認しない。
- Linux内部 `bridge save` の `SaveOptions{Stopped:true}` は、そのWindows側申告を共通Saveへ伝えるアダプター上の契約であり、Linux側で停止検出に成功した意味ではない。内部コマンドを直接呼んでも停止検出は追加されない。
- Linux公開CLIの `save --stopped` も同じ申告。この試験スクリプトは専用サンプルを同期実行し、runの終了後にのみ保存する。操作者は外部ライターがないことを確認して手順を開始する。
- SaveとRunのflockは協調するPocketLinx操作間の排他。任意の外部プロセス、DB、エディターの書き込みを停止する機能ではない。ハッシュ変更検出も実際の停止保証ではない。
- 既存Backendへの正式接続時は、実行プロセスと子孫の停止完了、外部ライターの扱い、排他の保持期間、DB等の整合性確保を別途実装・検証する必要がある。

## 実施結果と未実施事項

| 試験 | 実際の結果 |
| --- | --- |
| Windows→独立Linux→Windows | 未検証。利用できる独立Linuxなし |
| 独立Linux→Windows→独立Linux | 未検証。同上 |
| Windows起点の3段階（相手側も同じWSL） | 手順動作確認成功 |
| Linuxスクリプト起点の3段階（同じWSL） | 手順動作確認成功 |
| ファイル内容・実行権限・UID/GID・相対リンク・Definition比較 | WSL内のsmokeで一致 |
| 非root UID=1000/GID=1001でのサンプル実行 | WSL内で成功 |
| source/volume編集・実行時書き込み・マーカー削除 | 再保存・再復元後に保持（WSL内） |
| 既存保存先・既存復元先上書き | 拒否、元データ不変を確認 |
| 権限不足 | 明示エラー、環境不変を確認 |
| WSLをlinux-vmとして申告 | 拒否を確認 |
| Go全体テスト・go vet | 成功 |
| Docker上の新しいオフライン手順smoke | 成功。独立Linux検証には数えない |

今回のWSL内smokeで転送前後が一致したSHA-256（独立Linuxとの実測値ではない）：

```text
Windows起点・初回
569f5754c3f683b3da5a13aa72dcc19ad858708544382453103b8f8f2d76249d
同・受信側編集後
6c80d3388fb992748ebdcc2763fe2d848dc2285b8efe348e2052de459b949b18
Linuxスクリプト起点・初回
a2384e35147cc84e60416544b13944d2555a126a52a27b661d2ff7e2ffd2d656
同・受信側編集後
d6c68f627c7a9da3c596f184a540d72182e5a67f58b62e517e861f23fc4d7c8b
```

証跡はローカルの `images/offline-final-*`、`images/reverse-final-*` とWSL内 `/opt/pocketlinx/` の対応ディレクトリに保持（Git対象外）。GitHub Actionsには既存Linux・Windowsテストと新しいsmokeを定義し、該当コミットの結果は完了報告で示す。

手順確認中の修正：PowerShellの `CLI` が既存alias `Clear-Item` と衝突したため関数を `Invoke-Adapter` に変更。Alpineにwslpathがなかったため、証跡JSONだけを標準入力で渡す方式に変更（パッケージは既存のバイナリ転送のまま）。期待した上書き拒否の終了コードがスクリプト成功時に残っていたため成功時にリセット。CI用一時ディレクトリが0700で非root試験の実行ファイル自体に到達できなかったため、検証用親を0755に設定した。環境内部の所有者を書き換えて試験を通したものではない。

異なるLinux環境間の差異はまだ観測できていない。独立ホストのファイルシステム、カーネル、LSM、名前空間・能力設定の差、実機間転送、物理Linuxでの実行は未検証。実環境取得後に上記両経路を実行し、初回／再保存SHA-256、属性比較、ホスト証跡を揃えて初めて完成条件を判定する。任意環境への対応、本番隔離の安全性、Dockerより軽いという結論は出さない。
