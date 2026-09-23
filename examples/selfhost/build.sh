#!/bin/sh
set -eu
mkdir -p out
id
go version
cat SOURCE_COMMIT
sha256sum -c SOURCE_FILES.sha256 > out/source-check.txt
# No cached test results: execute tests again after restore. JSON preserves skips.
go test -count=1 -json ./... > out/tests.json
go build -o out/plx-env ./cmd/plx-env
GOOS=windows GOARCH=amd64 go build -o out/plx-env.exe ./cmd/plx-env
sha256sum out/plx-env out/plx-env.exe > out/binaries.sha256
cat out/binaries.sha256
printf 'PASS: tests and both builds completed\n'
