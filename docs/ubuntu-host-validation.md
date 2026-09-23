# GitHub Actionsの独立Ubuntu VM：直接実行検証

2026-09-24。**独立Linux VM上での保存・復元・実行・変更後再保存は成功。物理Linux実機での検証ではなく、WSLとの双方向往復は未実施。** 製品コード・Definition・保存形式・既存Backendは変更していない。

## 使用ランナーと証跡

通常のGitHub-hosted `ubuntu-24.04`（4 vCPUのVM）を使用した。`ubuntu-slim`やjob containerは使っていない。[GitHubのランナー区分](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)でも通常Ubuntuとコンテナ型slimは区別される。

成功した検証：[Actions run 35930104468](https://github.com/takehisa-nanba/PocketLinx/actions/runs/35930104468)、検証コードコミット `d9c8dccca00518a535012a6919cd704c8c0305b7`。ubuntu-host、既存linux、windows-adapterの3ジョブが成功した。従来のAlpineコンテナ試験は独立して維持している。

| 項目 | 実際の観測 |
| --- | --- |
| OS | Ubuntu 24.04.5 LTS (Noble Numbat) |
| カーネル | 6.17.0-1022-azure、x86_64 |
| CPU | AMD EPYC 7763、ランナーから見える論理CPU数4 |
| 仮想化 | Microsoft、full virtualization。systemd-detect-virt=microsoft |
| ファイルシステム | /dev/sda1、ext4、rw/relatime等 |
| 検証領域 | `/tmp/plx-host-35930104468`。コード・サンプル・ビルド一時領域・キャッシュ・証跡を専用領域へ配置 |
| 権限 | sudoで検証ハーネスを起動。サンプル自体はDefinitionのUID=1000/GID=1001 |

生のプラットフォーム情報は [platform.txt](ubuntu-host-evidence/platform.txt)、直接実行ログは [host-run.txt](ubuntu-host-evidence/host-run.txt) に保存した。アプリケーションの環境変数一覧やGitHub認証情報は収集・アップロードしない。

## 固定した素材と実行方式

素材は公式Alpine 3.22.1 x86_64 minirootfs：

```text
https://dl-cdn.alpinelinux.org/alpine/v3.22/releases/x86_64/alpine-minirootfs-3.22.1-x86_64.tar.gz
SHA-256: 0e5cc5702ad72a4e151f219976ba946d50161c3acce210ef3b122a529aba1270
BusyBox: 1.37.0-r18
musl: 1.2.5-r10
```

アーカイブのSHA-256を照合して専用領域に展開し、fixtureがBusyBoxとmuslだけをサンプルrootfsへコピーする。`PLX_FIXTURE_BASE`を検証ツールに追加し、UbuntuにAlpineのライブラリをインストール・上書きしない。未指定時は従来どおり `/` を使うため既存Alpine/WSL試験は維持する。素材個別のハッシュは [materials.sha256](ubuntu-host-evidence/materials.sha256)、パッケージバージョンの原記録はartifactのalpine-packages.txtにある。

このジョブにはDockerコマンドがない。fixture構築、共通CLIのsave/restore/run、再保存、検証ツールはすべてUbuntuホストのプロセスとして実行する。製品のchroot内部がAlpineユーザーランドであることと、検証プロセスをDockerコンテナで動かすことを混同しない。

## 権限・往復・属性の結果

| 検証 | 結果 |
| --- | --- |
| mount/PID名前空間 | ホスト上のunshare --mount --pid --forkが成功。既存GoヘルパーのPID1・mount名前空間確認も通過 |
| mount伝播とbind mount | 専用名前空間内のmake-rprivate、bind、umountが成功。ランナーホストのmount名前空間は変更しない |
| chroot、UID/GID、補助グループ設定 | 既存Backendで実行成功。出力uid=1000/gid=1001を確認 |
| 初期保存→別ディレクトリ復元 | 成功。保存・復元は既存共通処理 |
| Definitionからの実行 | command/workdir/env/uid/gid/source/volume配置を手動補完せず成功 |
| source/volume変更・削除→再保存 | 成功。source生成ファイルと永続データへの実行時書き込みも保存 |
| 再復元・再実行 | runtime-writeとeditedを確認、削除マーカーは再出現しない |
| 内容・mode・UID/GID・相対リンク・Definition | 送信時snapshotと復元直後snapshotが完全一致。mtimeは追加snapshot比較の対象外 |
| 既存パッケージ・既存復元先 | 上書きを拒否。保存済みSHA-256とsnapshotの不変を確認 |
| 非特権CLI起動 | ホストUID/GID 65534で明示エラー。別ユーザーへのフォールバックなし、環境不変 |

実行出力：

```text
初期: hello|/workspace|persistent-data|uid=1000|gid=1001
変更後: hello|/workspace|updated-data|uid=1000|gid=1001
        edited
再復元: hello|/workspace|runtime-write|uid=1000|gid=1001
        edited
```

証跡のstart/changed/finish各フォルダーにoutput.txt、result.txt、permission-error.txtを保存した。start/changedにはファイル属性snapshotもある。`--stopped`は停止検出ではなく呼び出し元の申告という契約を維持する。本試験では同期サンプルの終了後に保存し、外部ライターを設けない。本番Backendでの停止確認・排他は別途必要。

## 失敗と修正

[初回run 35929945613](https://github.com/takehisa-nanba/PocketLinx/actions/runs/35929945613)では、mount名前空間とサンプル実行までは成功した。しかし非特権CLIの補助試験が、`/home/runner/work/_temp/.../plx-env` のexec段階で `permission denied` になった。UID 65534がランナーの親ディレクトリを通過できなかったため、想定した「CLI内部のroot要件エラー」に到達しなかった。

ランナーの親ディレクトリ権限やサンプルの所有者は変更せず、専用検証領域を `/tmp/plx-host-<run_id>` に移した。再試験ではCLIから `experimental chroot requires root ...; no user fallback`、復元環境では `open ...: permission denied` を取得し、補助試験も成功した。製品の権限エラーを無視したり、実行ユーザーを変えて通したものではない。

## artifactの回収

[成功artifact](https://github.com/takehisa-nanba/PocketLinx/actions/runs/35930104468/artifacts/10781131385)は `ubuntu-host-d9c8dccca00518a535012a6919cd704c8c0305b7`。保持期間7日。各実行でコミットSHA付きの別名を使う。

- `start/package.plxenv`：Ubuntu VMで作成・保存した初期専用サンプル。
- `changed/package.plxenv`：Ubuntu VMで復元・変更・再保存した専用サンプル。
- 各パッケージのSHA-256、files.jsonとそのSHA-256、試験state。
- start/changed/finishの実行結果、権限不足ログ、ホスト・素材の情報。

今回Windowsへ実際にダウンロードし、ZIPと両パッケージのハッシュを照合した。

```text
artifact ZIP:
d9049a9da7192c736843d3d9ef4eecc4cdfe94bd1a80d9a48a2dc48569a57ef3
start/package.plxenv:
0cb724f889d3afee9cc366b0872a0a21886c5552855d07ef212c43f7093f7994
changed/package.plxenv:
554c263007dcd610dbeb1ad5ce73cc6910f602b0cf32d9cee3fe628bb05b5fbc
```

手元の回収先は `images/ubuntu-host-35930104468/evidence`。パッケージ本体とZIPはGit対象外。合成サンプルのテキスト証跡だけをdocs/ubuntu-host-evidenceに記録した。ハッシュは完全性検証であり、配布者認証ではない。

GitHubへサインインしActions画面からダウンロードするか、認証済みGitHub CLIで `gh run download 35930104468 -R takehisa-nanba/PocketLinx -n ubuntu-host-d9c8dccca00518a535012a6919cd704c8c0305b7 -D C:\Evidence\ubuntu-host` を使う。[GitHubのartifact回収手順](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/download-workflow-artifacts)参照。認証トークンをファイル・引数例・artifactへ格納しない。

## WSL受信・変更・再保存の手順と実施範囲

**Windowsへの回収とハッシュ照合まで実施。WSLでの復元・実行・再保存は今回未実施。** 手元で新しい合成サンプルを作るコマンドは自動承認レビューから `blocked by policy` と拒否され、具体的な追加理由は返されなかった。そのローカル実行を別経路で再試行していない。

既存の検証用バイナリを用意し、初期artifactから受信側編集を行う手順：

```powershell
.\scripts\roundtrip-windows.ps1 -Stage exchange -Distro PocketLinx-Verify -WindowsCLI .\plx.exe -Root /opt/pocketlinx/ubuntu-received-001 -Artifacts C:\Evidence\wsl-return-001 -Sample /mnt/c/path/to/PocketLinx/examples/environment -InputBundle C:\Evidence\ubuntu-host\start\package.plxenv
```

この手順はpackageと証跡のSHA-256、復元属性、Definition実行、source/volumeの変更・削除・実行時書き込み、再保存を確認する。既存環境と同名のRootやArtifactsを使わない。既存ディストリビューションやDockerの停止・初期化は不要。

## WSL→Ubuntuへの安全な受け渡し案

新しいサーバーや公開URL入力による任意パッケージ実行は導入しない。最初の方式は、**新規fixtureから生成した合成データだけを専用のリポジトリ内テスト資産として渡す**ものとする。今回は設計までで、受信ジョブおよびこの方向の実行は未実装・未検証。

1. WSLで空の新規検証環境をfixtureから作る。既存ユーザー環境をsaveの入力にしない。
2. rootfsが固定素材のBusyBox/musl、Definitionが専用サンプル、source/volumeが既知スクリプト・固定文字列・生成ファイル・マーカーだけであることを、パス集合・ファイルハッシュ・内容で確認する。未知ファイルを含めない。
3. `.plxenv`、SHA-256、files.json、そのハッシュ、stateの5点だけを専用ディレクトリへ追加する。ホストログ、ホーム、鍵、Git設定、認証情報は追加しない。試験資産のSHA-256と生成手順をレビュー可能なコード変更として固定する。
4. 将来のUbuntuホストジョブはその固定したテスト資産だけを受け取り、ハッシュ照合・復元後属性確認後に専用サンプルを実行する。外部URLや任意artifact IDを入力に取る特権実行ジョブにはしない。
5. WSL起点ならUbuntuでexchangeしてartifactをWSLへ戻してfinishする。Ubuntu起点の戻りならUbuntuでfinishする。両方の終了証跡が揃ってから双方向達成と判定する。

## 変更ファイル・限界

変更は `.github/workflows/environment.yml`、`.gitattributes`、検証ツール `scripts/wsl-fixture/main.go`。追加は `scripts/verify-ubuntu-host.sh`、本書、`docs/ubuntu-host-evidence/` のテキスト証跡。既存Go/Windows/Alpine試験は維持し、製品ロジック・保存形式・実験用Backend・mainは変更しない。

単体検査としてfixture/checkerのGoテストが成功し、GitHub Actionsでは既存Go全体・race・fuzz・Windows・Alpine試験と新Ubuntuホスト試験が成功した。独立Ubuntu VM単体の完成条件は達成。WSLとの実際の往復、物理Linux実機、任意の開発環境、安全な本番隔離、Dockerより軽量であることは未達成・本試験の証明対象外。
