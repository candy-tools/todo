default: help

#==========================================================================================
##@ Testing
#==========================================================================================
# Cover profile written by `make test` and read by `make coverage`, so verify runs the suite once.
COVER_PROFILE ?= coverage.out

test: ## run the go tests, writing the cover profile the coverage gate reads
	@rm -f $(COVER_PROFILE)
	@go test ./... -covermode=atomic -coverprofile=$(COVER_PROFILE)

lint: ## run the go linter (needs golangci-lint)
	@golangci-lint run

vet: ## run go vet
	@go vet ./...

license-check: ## check for invalid licenses
	# depends on https://github.com/elastic/go-licence-detector
	@go list -m -mod=readonly -json all | go-licence-detector -includeIndirect -rules allowedLicenses.json -overrides overrideLicenses.json

COVERAGE_THRESHOLD ?= 70
# Reads the profile of `make test` (no test run of its own): each package under app/ and internal/ must reach the
# threshold; app/metainfo is left out, as it holds only linker-stamped vars. A profile block is
# "dir/file.go:pos numStmts count", so a package is the dir of its blocks.
.PHONY: coverage
coverage: ## gate per-package coverage of app/ and internal/ on the cover profile of `make test`
	@if [ ! -f $(COVER_PROFILE) ]; then \
		echo "❌ $(COVER_PROFILE) not found: run make test first"; \
		exit 1; \
	fi
	@go list ./app/... ./internal/... | grep -v '/app/metainfo$$' | awk -v threshold=$(COVERAGE_THRESHOLD) ' \
		FNR == 1 && NR == 1 { next } \
		NR == FNR { \
			pkg = $$1; sub(/\/[^\/]*:.*$$/, "", pkg); \
			if (!($$1 in stmts)) { stmts[$$1] = $$2; total[pkg] += $$2 } \
			if ($$3 > 0 && !($$1 in hit)) { hit[$$1] = 1; covered[pkg] += $$2 } \
			next \
		} \
		{ \
			if (!total[$$1]) { print "⚠️ No coverage data for " $$1; fail = 1; next } \
			pct = 100 * covered[$$1] / total[$$1]; \
			if (pct < threshold) { printf "❌ Coverage in %s is %.1f%%, below %s%%!\n", $$1, pct, threshold; fail = 1 } \
			else printf "✅ %s %.1f%%\n", $$1, pct \
		} \
		END { exit fail }' $(COVER_PROFILE) -

.PHONY: verify
verify: ## run all checks; runs every check and fails if any fail
	@fail=0; \
	for target in test vet license-check lint coverage; do \
		echo "==================== make $$target ===================="; \
		$(MAKE) --no-print-directory $$target || fail=1; \
	done; \
	if [ $$fail -ne 0 ]; then \
		echo "❌ verify failed (see above)"; \
		exit 1; \
	fi; \
	echo "✅ verify passed"

#==========================================================================================
##@ Running
#==========================================================================================
EXAMPLE_FILE = $${TMPDIR:-/tmp}/todo-example.json
run: ## run on a fresh import of example.md in a temp file (never touches ./.todo.json)
	@go run . import --force -f $(EXAMPLE_FILE) example.md && go run . -f $(EXAMPLE_FILE)

#==========================================================================================
##@ Building
#==========================================================================================
build: ## use goreleaser to build for the current OS/Arch
	# depends on https://goreleaser.com
	@goreleaser build --snapshot --clean --single-target

#==========================================================================================
##@ Release
#==========================================================================================
.PHONY: check-branch
check-branch:
	@current_branch=$$(git symbolic-ref --short HEAD) && \
	if [ "$$current_branch" != "main" ]; then \
		echo "Error: You are on branch '$$current_branch'. Please switch to 'main'."; \
		exit 1; \
	fi

.PHONY: check-git-clean
check-git-clean: # check if git repo is clean
	@git diff --quiet

tag: check-git-clean check-branch ## create and push a git tag to publish a new release
	@[ "${version}" ] || ( echo ">> version is not set, usage: make tag version=\"v1.2.3\" "; exit 1 )
	@git tag -d $(version) || true
	@git tag -a $(version) -m "Release version: $(version)"
	@git push --delete origin $(version) || true
	@git push origin $(version) || true

clean: ## remove build artifacts
	@rm -rf todo coverage.out dist

#==========================================================================================
#  Help
#==========================================================================================
.PHONY: help
help: # Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
