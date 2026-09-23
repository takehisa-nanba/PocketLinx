# PocketLinx / Docker ウォーム基準測定

測定日：2026-09-24 JST（開始UTC 2026-09-23 15:10:31）。**同じWindows PC上で基準値を取得した。Dockerより軽いという達成判定は行わない。** 既存環境を止めていないためコールド起動と各製品に分離した総リソースは未測定。

## PC・実行基盤

| 項目 | 実測・使用構成 |
| --- | --- |
| CPU | AMD Ryzen 5 7530U、6コア／12論理CPU |
| RAM | OS観測16,486,756,352バイト（約15.35 GiB） |
| OS | Windows 11 Home、10.0.26200（WSLの表示では26200.9457） |
| WSL | 2.7.3.0、Linux 6.6.114.1-microsoft-standard-WSL2、amd64 |
| PocketLinx側 | 既存PocketLinx-Verify、Alpine 3.22.1、実験用chroot Backend |
| PocketLinxソース基準 | `f7056d0b71db74a8214479f6b05176be41b9fcef`。製品コードは変更せず既存バイナリを使用。SHA-256はmetadata.json |
| Docker | Desktop 4.92.0 (240144)、Engine/Windows CLI 29.8.0、desktop-linux、containerd 2.3.5、runc 1.5.1 |
| 既存負荷 | 他のDockerコンテナ3個が稼働。Windowsアプリ、Docker Desktop、共有VM、キャッシュ等の影響あり |

WSLの`/proc/meminfo`はMemTotal 7,795,808 KiB、SwapTotal 2,097,152 KiBを報告した。これはVMの論理的メモリ情報であり、Windows上で物理常駐している量ではない。既存のDocker Desktop、WSLディストリビューション、利用者のコンテナ・イメージ・ボリュームは停止・削除・変更していない。

## 処理内容と測定境界

PocketLinxはWindows版CLIから既存wslbridgeを通じて保存・復元・実行する。計時はWindowsのProcess.Start直前からWaitForExit完了まで。stdout/stderrをパイプで取得し、画面描画を含めない。Windowsのプロセス・メモリ観測は計時区間の前後で行う。各操作5回、実行は事前ウォームアップ1回を除外し、PocketLinx/Dockerの実行順を交互にした。OSキャッシュは削除せず、CPU周波数・電源モード・バックグラウンド負荷も固定していない。

Dockerの専用イメージはPocketLinxのfixtureが構築した**同じBusyBox・musl・ソース・データ**をtarにしてimportしたAlpine/amd64最小ユーザーランド。通常のAlpineフルイメージとの比較ではない。DefinitionからCMD、WORKDIR、ENV、USERをDocker設定へ対応付け、出力 `hello|/workspace|persistent-data|uid=1000|gid=1001` の一致を各実行で確認した。

| 項目 | PocketLinx | Docker |
| --- | --- | --- |
| ウォーム実行 | `plx-env wsl ... --trusted-sample run`。毎回WSL確認とLinux CLI・ヘルパーを起動 | `docker run --rm --network none IMAGE`。毎回コンテナ作成・起動・削除を含む。Desktop/daemonは起動済み |
| rootfs／データ | rootfs読み取り専用、source/volumeをbind接続 | rootfs書き込み可能。source/データもコンテナ層に格納。名前付きvolumeは不使用 |
| 隔離 | 検証用chrootと専用mount/PID名前空間。ネットワーク隔離なし | 通常Docker/runcの隔離、network none。安全性・機能は同等ではない |
| 復元 | Windowsパッケージの転送・照合と、新しいLinuxディレクトリへの完全復元 | 既に存在するレイヤーを含むアーカイブの`docker load`。キャッシュされたレイヤーを再利用するため新規展開と同等ではない |
| 再保存 | 変更後のrootfs/source/volume/Definitionを完全保存してWindowsへ転送 | 停止した測定専用コンテナをcommitし、image save。別々に計測 |

変更後保存は、PocketLinx側でfixtureのeditとDefinitionによる実行を済ませた状態を使った。同じ変更済みソース・生成ファイル・永続データをDocker専用コンテナへコピーした。Windowsからのdocker cpによる所有者差は、**測定用Dockerコンテナだけ**でUID/GIDを明示設定して準備した。編集・コピー・所有者準備の時間は保存計時に含めない。Docker側の保存後イメージを実行して `runtime-write` と `edited` を確認した。編集方法や所有者準備を含む全開発工程が同等という主張ではない。

## 実行・保存・復元の生データと中央値

単位ms。各欄は実行順。全45操作の時刻、stdout/stderr、メモリ前後値は [raw.json](raw.json)、時間集計は [summary.json](summary.json)。

| 操作 | 1 | 2 | 3 | 4 | 5 | 中央値 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| PocketLinx ウォーム実行 | 263.96 | 266.79 | 261.81 | 263.29 | 261.30 | **263.29** |
| Docker ウォームrun（create/remove含む） | 447.63 | 454.36 | 429.84 | 432.68 | 437.08 | **437.08** |
| PocketLinx 復元＋転送 | 270.32 | 275.14 | 283.68 | 281.98 | 264.34 | **275.14** |
| PocketLinx 変更後再保存＋転送 | 273.46 | 273.28 | 271.13 | 274.02 | 271.96 | **273.28** |
| Docker commit | 276.95 | 284.20 | 254.50 | 272.08 | 282.80 | **276.95** |
| Docker image save | 252.15 | 224.67 | 242.30 | 252.91 | 228.07 | **242.30** |
| Docker image load（キャッシュあり） | 284.75 | 290.03 | 278.94 | 299.12 | 296.37 | **290.03** |
| RSS計測付きLinux CLI呼び出し（WSL越し） | 120.52 | 102.98 | 103.10 | 102.54 | 107.36 | 103.10 |
| RSS計測付きDocker payload run | 443.00 | 435.88 | 444.85 | 430.15 | 445.99 | 443.00 |

最後の2行はRSS取得のための別経路。Linux CLI行はWindowsアダプターを迂回し、wsl.exeからBusyBox timeを通して直接Linux CLIを呼ぶ。上の通常実行と混ぜて平均しない。BusyBox time自体のwall表示はPocketLinx全回0.01秒、Docker内サンプル全回0.00秒（表示分解能以下）。正確に0秒という意味ではない。

## RSSとWindows／共有VMのメモリ

BusyBox `time -v` の最大RSS、単位KiB：

| 測定対象 | 各回 | 中央値 |
| --- | --- | ---: |
| Linux版PocketLinx CLI実行のwait報告値 | 3584, 3712, 3712, 3456, 3712 | 3712 |
| Docker内サンプルシェルのwait報告値 | 896, 896, 896, 896, 896 | 896 |

**境界が異なるため、上のRSSを製品全体の優劣として比較しない。** PocketLinxの値はWindows CLIやWSL VM全体を含まず、ヘルパーを含む全プロセスの同時常駐量の合計でもない。Dockerの値はdaemon/containerd/runc/Desktop/VMを含まない。PocketLinx RSSとDocker Desktop全体のメモリを比較しない。

WindowsのWorkingSet観測（MiB）。操作後の5値と中央値：

| 観測対象・タイミング | 各回 | 中央値 |
| --- | --- | ---: |
| 共有vmmemWSL、PocketLinx run後 | 957.46, 1017.79, 1012.30, 1011.80, 1015.58 | 1012.30 |
| 同、Docker run後 | 999.96, 1013.70, 1002.36, 1007.77, 1016.72 | 1007.77 |
| Docker関連WindowsプロセスのWS単純合計、PocketLinx run後 | 566.24, 572.39, 572.15, 572.13, 572.56 | 572.15 |
| 同、Docker run後 | 569.23, 572.39, 572.82, 572.05, 570.05 | 572.05 |

これは操作直後の観測値であり、その操作が新たに消費した量でも、各製品へ帰属させた量でもない。Docker関連プロセスの合計は共有ページの重複を含み得るため、Windowsの物理メモリ消費合計とは一致しない。共有VMの値を両製品へ割り振ったり、各製品のRSSへ加算したりしない。

前後の各プロセスPID/WorkingSet/PrivateBytesとホスト空き物理メモリはraw.jsonに全件保存。[statistics.json](statistics.json)には各操作の前後の5値・中央値・最小最大を収録した。run後のホスト空き物理メモリ中央値はPocketLinx 2614.59 MiB、Docker 2630.02 MiB。共有キャッシュや他アプリの変動があるため差を製品の節約量としない。Linux MemAvailableは測定前6,720,820→後6,667,852 KiB（各1回）。個別ディストリビューションの消費量ではない。

## ディスク・実行ファイル・パッケージ

静的なファイルサイズや配置量は1回の読取り。パッケージは保存5回それぞれのサイズを記録した。ディスク補足検査は計時後に実施し、[supplemental.json](supplemental.json)へ別記した。

| 項目 | 値 |
| --- | ---: |
| PocketLinx Windows CLI | 4,304,384 bytes |
| PocketLinx Linux CLI | 4,267,652 bytes（du 4,168 KiB） |
| Docker Windows CLI | 44,785,072 bytes |
| PocketLinx初回パッケージ | 1,483,776 bytes |
| PocketLinx変更後パッケージ、5回／中央値 | 全回1,484,800 bytes |
| Docker保存tar、5回／中央値 | 全回946,176 bytes |
| PocketLinx初期環境のdu | 1,488 KiB |
| PocketLinx変更済み環境のdu | 1,492 KiB |
| PocketLinx今回の測定領域全体 | 10,404 KiB（初期環境、5復元先、Docker用rootfs等） |
| PocketLinxディストリビューション全体のdu -x | 78,504 KiB。以前の検証成果物を含む |
| Docker保存イメージinspect Size | 2,531,081 bytes。共有レイヤーがあり、ディスク専有量ではない |
| Docker測定コンテナSizeRw / SizeRootFs | 32,768 / 1,601,536 bytes |
| Docker Desktopインストールファイル長合計 | 3,527,130,319 bytes。多機能製品全体の配置量 |
| Windows測定成果物（測定末時点） | 15,279,486 bytes。パッケージ・tar・生データ等 |
| PocketLinx WSL VHDX長、前→後 | 180,355,072 → 213,909,504 bytes |
| DockerデータVHDX長、前→後 | 4,473,225,216 → 4,473,225,216 bytes |
| Docker main VHDX長、前→後 | 100,663,296 → 100,663,296 bytes |

Docker保存tar内のOCI manifestは3レイヤーを `application/vnd.oci.image.layer.v1.tar+gzip` と報告し、圧縮サイズは935169/389/383 bytesだった。PocketLinxは非圧縮の完全保存。従って946,176と1,484,800 bytesの差を同一形式の効率差としない。DockerにはDefinitionやsource/volumesという独立した可搬構造はなく、Docker設定とコンテナ層として格納する。名前付きボリュームを使う構成のバックアップ費用は含まない。

VHDX長は仮想容量でも正確な物理割当量でもなく、成長単位や既存空き領域の影響を受ける。Docker VHDXには既存イメージ・ボリューム等が含まれる。差分を今回の機能だけへ帰属できない。全体の`docker system df`観測もmetadata.jsonに残した。CLIサイズの機能範囲も異なり、それだけでランタイムが軽いとは判断しない。

正常終了後の専用領域の`.plx-*`残留一時ファイルはWindows・WSLとも0件。**保存・復元中の一時ファイルの瞬間最大占有量は未測定**。稼働中Docker内部の一時領域やVHDXの正確な物理割当も分離できていない。推測値で補っていない。

## 再実施手順と成果物

既存Go製品コード・Definitionを変更せず、WSLにplx-envと既存fixtureを配置する。Windows側パスと対応するWSL側パスは同じ新規専用ディレクトリを指すよう指定する。PowerShell 7で：

```powershell
.\scripts\measure-baseline.ps1 -Artifacts C:\Measurements\plx-run-001 -LinuxArtifacts /mnt/c/Measurements/plx-run-001 -WindowsCLI .\plx.exe -Distro PocketLinx-Verify -Sample /mnt/c/path/to/PocketLinx/examples/environment
.\scripts\summarize-baseline.ps1 -Raw C:\Measurements\plx-run-001\raw.json -Output C:\Measurements\plx-run-001\statistics.json
.\scripts\test-baseline.ps1
```

各再実行は別の成果物ディレクトリを使う。Dockerタグ・測定用コンテナ名とWSLディレクトリはランダムな `plx-measure-*` プレフィックス。測定用runの`--rm`はその場で新規作成する一時コンテナだけを削除する。保存用の新規イメージ・停止コンテナと成果物は保持し、既存資産へpruneや停止を行わない。

今回の本計測：`images/baseline-20260924-c`、Docker prefix `plx-measure-71c0b8a485bd`、WSL `/opt/pocketlinx/plx-measure-71c0b8a485bd`。パッケージ類はGitへ含めず、関連するJSONのみこのディレクトリへ保存した。metadata.jsonのCreatedDockerResourcesで新規資産を確認できる。

最初の2回はPowerShell標準alias `Measure` との衝突で計時開始前に中断。関数をInvoke-Measurementへ変更して本計測を実施した。準備済みの専用イメージ・ディレクトリは保持され、本計測のDiskBeforeに含まれる。計測後、WSLの管理コマンド出力をUTF-16として読む修正と、duの子ディレクトリを個別取得する修正を行った。元の生データは書き換えていない（metadata.jsonのWSL表示ラベルは文字化けが残るが版番号は取得済み）。現在の再実施スクリプトにはこの修正と追加ディスク情報取得を含む。タイマー境界や本計測の45値は変更していない。

## 観測から次に調べること

- 小さい専用サンプルでは、Linux内の実作業よりWindowsからの起動・制御経路が大きい。PocketLinxのWSL確認回数・ヘルパー起動・全ファイル検査を個別に測る価値がある。ただし差分時間から各コストを断定しない。
- Docker runはこの条件で約437ms。コンテナ作成・削除、隔離設定、daemon経路を含むので、PocketLinxと同じ責務に分解してから比較する必要がある。
- メモリでは数MiBのプロセスRSSより約1GiBの共有VMやWindows側常駐部分の観測量が大きい。ただし既存コンテナ3個等を含み、PocketLinx専用コストやDocker専用コストは算出できない。
- データ規模は約1.5MBに限られる。大容量source/volumeでの全量ハッシュ、コピー、保存時一時ファイル最大量を次に測る。圧縮有無の比較も別試験とする。
- コールド起動、製品単独起動時のVM増分、実行中全プロセスの同時メモリ、Windows CLIの瞬間ピーク、物理ディスク割当は未測定。既存環境を安全に停止できる専用測定枠が必要だが、今回は停止していない。

今回は計測スクリプト・集計・報告だけを追加し、最適化は行わない。独立Linuxの双方向試験も今回の範囲外。

## ファイルとテスト

追加：`scripts/measure-baseline.ps1`、`scripts/summarize-baseline.ps1`、`scripts/test-baseline.ps1`、本書、raw/metadata/summary/statistics/supplementalのJSON。変更：GitHub Actionsにパーサー・集計テストと関連パスのトリガーを追加。README等の別変更は対象外。

実Windowsで本計測45操作と保存後Docker出力確認が成功。オフラインテストではPowerShell構文、9種類×5回の中央値の再計算一致、観測欠落を0に変換しないことを確認した。CIでは性能値の閾値判定や実PC計測はせず、このオフラインテストと既存Go・Alpine試験を実行する。該当コミットのGitHub Actions結果は完了報告で示す。
