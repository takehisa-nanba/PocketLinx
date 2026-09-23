# 実行基盤の比較と判断保留

2026-09-23時点。今回の保存・復元はランタイムを呼び出さないため、runcの採用有無と独立して利用する。

| 項目 | 既存のunshare + shim | runcをBackend内で利用する案 |
| --- | --- | --- |
| 依存関係 | シェル、unshare、mount、chroot。WSL側はiproute2、iptables等も使用 | runcバイナリ、Linuxカーネル機能。ビルド／配布構成に応じたライブラリ依存を確認する必要がある |
| 常駐 | CLI実行中心。GUI利用時は管理APIとプロキシが動作 | Dockerデーモンを導入せずCLIとして呼び出す構成を検討できる。PocketLinx側の監視設計は別途必要 |
| 再現性 | WSLとLinuxでメタデータ、環境変数、操作の実装が異なる | 共通の実行設定を生成する設計にできるが、ホスト機能差の検出が必要 |
| 安全性 | 既知の入力検証・シェル展開問題、ホスト/devの再帰bind、権限制限不足がある。現状のまま正式採用しない | namespaces、capabilities、seccomp、マウント等を設定できる。設定・バージョン・脆弱性対応はPocketLinx側の責務として残る |
| Windows | WSL内で実行。現状はWSL呼び出しと設定処理が密結合 | WSL内のLinux実行基盤として呼び出す必要がある |
| Linux | root前提の処理があり、Start/Stop/Exec等に未実装がある | 必要権限、cgroups、user namespace等を実ホストで検証する必要がある |
| 軽量性 | 測定前に有利とは判断できない | 追加バイナリ容量・プロセス・初期化時間を含めて測定する必要がある |

今回の判断：どちらも正式採用しない。保存形式にOCI設定やshim引数を埋め込まない。後続で共通のDefinitionから実行設定を組み立て、使い捨て環境で両方式を比較する。

確認すべき条件：ホストマウントの非公開、デバイス制限、環境変数の隔離、最小権限、ネットワークの扱い、停止とPID追跡、停止後のデータ整合性、WSLとLinuxの同じ挙動。

参考：

- [runc公式README](https://github.com/opencontainers/runc)：Linux向けCLI、ビルド依存関係。
- [OCI Linux実行設定](https://github.com/opencontainers/runtime-spec/blob/main/config-linux.md)：名前空間、リソース、ファイルシステム等。
- [既存WSL実装](../pkg/container/wsl_runtime.go)、[Linux実装](../pkg/container/linux_runtime.go)、[shim](../pkg/shim/content.go)。
