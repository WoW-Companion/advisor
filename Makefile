PLATFORMS := windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: verify release-prep dist cross-check bench integration advice-benchmark offline-benchmark
verify:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi
	go vet ./...
	go test ./...
	go run ./cmd/release-prep -check
	@if command -v cargo >/dev/null 2>&1; then \
		echo "cargo check --locked (overlay/src-tauri)"; \
		(cd overlay/src-tauri && cargo check --locked) || exit 1; \
	else \
		echo "verify: cargo not found; skipping the overlay Rust compile check"; \
	fi

# Bundles changes/*.md into a new CHANGELOG section and bumps VERSION and the overlay version fields
# (BUMP=minor for a minor release). Run it once, as the last commit of a release.
release-prep:
	go run ./cmd/release-prep

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

# The profession and stat advice benchmark: plans every profession at skill 1, 75, 150, 225 and 285 and checks the plans,
# trainer and vendor prices, trainer places and gathering nodes, and the stat priorities and suffixes, against a real
# dataset and the recorded Forever guide steps. Needs the
# network unless BENCH_DATASET names a dataset directory; not part of verify. BENCH_REPORT=<file> receives the
# result tables in Markdown. The last line is the summary, "advice-benchmark: N checks, S same, D differ (...)",
# so the share of advice that agrees can be read per run. See docs/professions.md.
advice-benchmark:
	@set -e; dir="$(BENCH_DATASET)"; log="$$(mktemp)"; \
	if [ -z "$$dir" ]; then \
		dir="$$(go run ./cmd/wowc-advisor dataset update --cache "$${TMPDIR:-/tmp}/wowc-advice-benchmark" | sed -n 's/^dataset .* ready at //p')"; \
	fi; \
	echo "advice-benchmark: dataset $$dir"; \
	status=0; WOWC_BENCH_DATASET="$$dir" WOWC_BENCH_REPORT="$(BENCH_REPORT)" go test ./internal/professions ./internal/stats -run 'AdviceBenchmark|CostsBenchmark|StatBenchmark|TailoringGuideBands' -count=1 -v > "$$log" 2>&1 || status=$$?; \
	cat "$$log"; go run ./internal/benchsummary/cmd < "$$log" || status=1; rm -f "$$log"; exit $$status

# The same profession and stat comparison on a fixed sample that needs no network: real dataset rows kept in
# testdata/advice_snapshot (rebuilt by scripts/advice-snapshot.sh), the planner's, the gear finder's, the talent
# plan's and the profession route's answers beside the dataset rows (items, armor, talent builds, profession
# routes) and the recorded Forever guide steps. It also runs in make verify (go test ./...).
# BENCH_REPORT=<file> receives the result tables in Markdown. The last line is the summary, "advice-benchmark:
# N checks, S same, D differ (...)". See docs/professions.md.
offline-benchmark:
	@log="$$(mktemp)"; status=0; WOWC_BENCH_REPORT="$(BENCH_REPORT)" go test ./internal/professions ./internal/gear ./internal/talents ./internal/paths -run 'OfflineBenchmark' -count=1 -v > "$$log" 2>&1 || status=$$?; \
	grep -E '^(---|FAIL|ok|    offline_)' "$$log" || true; go run ./internal/benchsummary/cmd < "$$log" || status=1; rm -f "$$log"; exit $$status
