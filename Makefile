GORELEASER ?= goreleaser

.PHONY: build snapshot release check

build:
	$(GORELEASER) build --clean --single-target

snapshot:
	$(GORELEASER) release --clean --snapshot

release:
	$(GORELEASER) release --clean

check:
	$(GORELEASER) check
