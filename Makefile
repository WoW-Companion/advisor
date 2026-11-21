PLATFORMS := windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: verify dist cross-check bench integration
verify:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi
	go vet ./...
	go test ./...

# The release archives and SHA256SUMS for every platform in dist/ (git-ignored): the same script the
# release workflow runs. VERSION and COMMIT override the VERSION file and git's HEAD.
dist:
	scripts/dist.sh

# Compiles the advisor for all six release platforms with no cgo, and writes nothing. About 10 s
# warm, so it is its own target and not part of verify.
cross-check:
	@for p in $(PLATFORMS); do \
		echo "cross-check $$p"; \
		CGO_ENABLED=0 GOOS=$${p%/*} GOARCH=$${p#*/} go build -trimpath -o /dev/null ./cmd/wowc-advisor || exit 1; \
	done

bench:
	go test ./internal/store -run '^$$' -bench . -benchmem

# Runs every tool against a real dataset and prints the results table. Needs the network and is not
# part of verify. DATASET=<dir> uses a built dataset; otherwise the latest release is downloaded,
# and when there is none it is built from the data repo (DATA_REPO, BUILD). See docs/eval.md.
DATA_REPO ?= ../data
BUILD ?= 1.60.1.71037
integration:
	@set -e; work="$$(mktemp -d)"; trap 'rm -rf "$$work"' EXIT; dir="$(DATASET)"; \
	if [ -z "$$dir" ]; then \
		echo "integration: dataset update (latest release)"; \
		dir="$$(go run ./cmd/wowc-advisor dataset update --cache "$$work/cache" | sed -n 's/^dataset .* ready at //p')" || dir=""; \
	fi; \
	if [ -z "$$dir" ]; then \
		echo "integration: no release; building build $(BUILD) from $(DATA_REPO)"; \
		(cd "$(DATA_REPO)" && go run ./cmd/wowc-data fetch -build $(BUILD) -cache "$$work/data" && \
			go run ./cmd/wowc-data build -build $(BUILD) -cache "$$work/data" -out "$$work/dist" && \
			go run ./cmd/wowc-data verify-manifest "$$work/dist/$(BUILD)"); \
		dir="$$work/dist/$(BUILD)"; \
	fi; \
	echo "integration: dataset $$dir"; \
	WOWC_INTEGRATION_DATASET="$$dir" go test -tags integration ./integration -run TestIntegration -count=1 -v
