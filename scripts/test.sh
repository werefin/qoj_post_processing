#!/usr/bin/env bash
# full verification pass: build, vet, fmt, race tests, cross-arch build
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

echo "== build =="
go build ./...

echo "== vet =="
go vet ./...

echo "== fmt =="
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
	echo "not gofmt'd:"
	echo "$unformatted"
	exit 1
fi

echo "== test (race) =="
go test ./... -race -count=1

# guards the amd64-CLMUL / portable-fallback split: every target must
# at least compile, even though only the current arch can run tests
echo "== cross-arch build =="
for pair in "linux/arm64" "linux/386" "linux/amd64" "darwin/amd64" "darwin/arm64"; do
	os="${pair%/*}"
	arch="${pair#*/}"
	echo "  $pair"
	GOOS="$os" GOARCH="$arch" go build -o /dev/null ./...
done

echo "== ALL GREEN =="
