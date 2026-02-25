.PHONY: tag docker-build

GIT := $(shell git pull --quiet 2>/dev/null || true)
LATEST_TAG := $(shell git tag --sort=-version:refname | head -n 1)-3
LATEST_TAG := $(if $(LATEST_TAG),$(LATEST_TAG),0.0.1)
tag:
	@echo "Latest tag: $(LATEST_TAG)"

docker-build:
	@echo "Building for tag: $(LATEST_TAG)"
# 	@echo "Building AMD64..."
# 	podman build --platform linux/amd64 \
# 		-t ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG)-amd64 \
# 		-f ./Dockerfile .
	@echo "Building ARM64..."
	podman build --platform linux/arm64 \
		-t ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG)-arm64 \
		-f ./Dockerfile .
	@echo "Creating manifest..."
	podman manifest rm ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG) 2>/dev/null || true
	podman manifest create ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG)
# 	@echo "Adding AMD64 to manifest..."
# 	podman manifest add ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG) \
# 		ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG)-amd64
	@echo "Adding ARM64 to manifest..."
	podman manifest add ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG) \
		ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG)-arm64
	@echo "Inspecting manifest:"
	podman manifest inspect ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG)
	@echo "Pushing manifest:"
	podman manifest push --all ghcr.io/9it-full-service/renovate-operator:$(LATEST_TAG)
