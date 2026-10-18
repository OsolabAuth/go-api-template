#!/bin/sh
set -eu

# Run from the repository root, even when invoked from another directory.
cd "$(dirname "$0")/.."
mkdir -p coverage
for report in coverage.out summary.txt index.html common.out common.txt common.html app.out app.txt app.html summary.md; do
    rm -f "coverage/$report"
done

go test -count=1 -coverpkg=./... -covermode=atomic -coverprofile=coverage/coverage.out ./...

# Split one test run so both groups include calls from integration tests.
# A trailing slash keeps internal/commonality out of the common group.
module_path=$(go list -m)
awk -v common_prefix="$module_path/internal/common/" '
    NR == 1 {
        print > "coverage/common.out"
        print > "coverage/app.out"
        next
    }
    index($1, common_prefix) == 1 {
        print > "coverage/common.out"
        common_count++
        next
    }
    {
        print > "coverage/app.out"
        app_count++
    }
    END {
        if (common_count == 0 || app_count == 0) {
            print "coverage: common and app must both contain statements" > "/dev/stderr"
            exit 1
        }
    }
' coverage/coverage.out

go tool cover -func=coverage/coverage.out > coverage/summary.txt
go tool cover -html=coverage/coverage.out -o coverage/index.html
for group in common app; do
    go tool cover -func="coverage/$group.out" > "coverage/$group.txt"
    go tool cover -html="coverage/$group.out" -o "coverage/$group.html"
done

{
    echo '| 対象 | Statement coverage |'
    echo '| --- | --- |'
    printf '| 共通処理 (`internal/common/`) | %s |\n' "$(awk '/^total:/ {print $3}' coverage/common.txt)"
    printf '| API 固有処理（共通処理以外） | %s |\n' "$(awk '/^total:/ {print $3}' coverage/app.txt)"
    printf '| 全体 | %s |\n' "$(awk '/^total:/ {print $3}' coverage/summary.txt)"
} > coverage/summary.md
cat coverage/summary.md
