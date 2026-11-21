#!/usr/bin/env bash
# Cross-builds wowc-advisor for every release platform into dist/ as one archive each, plus
# SHA256SUMS. The release workflow runs this same script (via `make dist`).
#   VERSION  release number (default: the VERSION file)
#   COMMIT   commit it was built from (default: git's short HEAD)
#   ONLY     "os/arch" to build a single target
set -euo pipefail
cd "$(dirname "$0")/.."

version=${VERSION:-$(tr -d '[:space:]' <VERSION)}
commit=${COMMIT:-$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)}
platforms=${ONLY:-"windows/amd64 windows/arm64 darwin/arm64 darwin/amd64 linux/amd64 linux/arm64"}
ldflags="-s -w -X main.version=$version -X main.commit=$commit"

rm -rf dist
mkdir -p dist/stage
scripts/notices.sh >dist/stage/THIRD-PARTY-NOTICES.txt

for p in $platforms; do
	os=${p%/*}
	arch=${p#*/}
	exe=wowc-advisor
	[ "$os" = windows ] && exe=wowc-advisor.exe
	name="wowc-advisor_${version}_${os}_${arch}"
	dir="dist/stage/$name"
	mkdir -p "$dir"
	echo "build $name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "$ldflags" -o "$dir/$exe" ./cmd/wowc-advisor
	cp LICENSE "$dir/LICENSE"
	cp dist/stage/THIRD-PARTY-NOTICES.txt "$dir/THIRD-PARTY-NOTICES.txt"
	cp packaging/README-quickstart.txt "$dir/README-quickstart.txt"
	if [ "$os" = windows ]; then
		(cd dist/stage && zip -qr "../$name.zip" "$name")
	else
		tar -C dist/stage -czf "dist/$name.tar.gz" "$name"
	fi
done

rm -rf dist/stage
cd dist
if command -v sha256sum >/dev/null; then sha256sum *.zip *.tar.gz 2>/dev/null >SHA256SUMS || true
else shasum -a 256 *.zip *.tar.gz 2>/dev/null >SHA256SUMS || true; fi
echo "dist: $(wc -l <SHA256SUMS | tr -d ' ') archive(s) for $version ($commit)"
