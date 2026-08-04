export CGO_ENABLED := "0"

set dotenv-load

container_tool  := `command -v docker >/dev/null 2>&1 && echo docker || echo podman`
container_build := if container_tool == "podman" { "build" } else { "buildx build" }
image           := "ghcr.io/mogenius/renovate-operator-dev"
tag             := `git describe --tags $(git rev-list --tags --max-count=1) 2>/dev/null || echo "dev"`

[private]
default:
    just --list --unsorted

# Run the application with flags similar to the production build
run *args: build jsInstall
    cd src && ../dist/native/renovate-operator {{args}}

# Build a native binary with flags similar to the production build
build: generate
    #!/usr/bin/env sh
    VERSION=$(git describe --tags $(git rev-list --tags --max-count=1) 2>/dev/null || echo "dev")
    cd src && go build -tags timetzdata -trimpath -gcflags="all=-l" -ldflags="-s -w -X main.Version=${VERSION}" -o ../dist/native/renovate-operator ./cmd/main.go

# Build binaries for all targets
build-all: build-linux-amd64 build-linux-arm64 build-linux-armv7

# Build binary for target linux-amd64
build-linux-amd64: generate
    cd src && GOOS=linux GOARCH=amd64 go build -tags timetzdata -trimpath -gcflags="all=-l" -ldflags="-s -w" -o ../dist/amd64/renovate-operator ./cmd/main.go

# Build docker image for target linux-amd64
build-docker-linux-amd64:
    {{container_tool}} {{container_build}} --platform=linux/amd64 -f Dockerfile \
        --build-arg GOOS=linux \
        --build-arg GOARCH=amd64 \
        -t {{image}}:{{tag}}-amd64 \
        -t {{image}}:latest-amd64 \
        .

# Build binary for target linux-arm64
build-linux-arm64: generate
    cd src && GOOS=linux GOARCH=arm64 go build -tags timetzdata -trimpath -gcflags="all=-l" -ldflags="-s -w" -o ../dist/arm64/renovate-operator ./cmd/main.go

# Build docker image for target linux-arm64
build-docker-linux-arm64:
    {{container_tool}} {{container_build}} --platform=linux/arm64 -f Dockerfile \
        --build-arg GOOS=linux \
        --build-arg GOARCH=arm64 \
        -t {{image}}:{{tag}}-arm64 \
        -t {{image}}:latest-arm64 \
        .

# Build binary for target linux-armv7
build-linux-armv7: generate
    cd src && GOOS=linux GOARCH=arm GOARM=7 go build -tags timetzdata -trimpath -gcflags="all=-l" -ldflags="-s -w" -o ../dist/armv7/renovate-operator ./cmd/main.go

# Build docker image for target linux-armv7
build-docker-linux-armv7:
    {{container_tool}} {{container_build}} --platform=linux/arm/v7 -f Dockerfile \
        --build-arg GOOS=linux \
        --build-arg GOARCH=arm \
        --build-arg GOARM=7 \
        -t {{image}}:{{tag}}-armv7 \
        -t {{image}}:latest-armv7 \
        .

# Install tools used by go generate
_install_controller_gen:
    go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

# Execute go generate
generate: _install_controller_gen
    controller-gen crd paths=./src/... output:crd:dir=charts/renovate-operator/crd

# Run tests and linters for quick iteration locally.
check: generate golangci-lint test-unit test-helm

# Execute unit tests
test-unit: generate
    cd src && go run gotest.tools/gotestsum@latest --format="testname" --hide-summary="skipped" --format-hide-empty-pkg --rerun-fails="0" -- -count=1 ./...

# Execute helm unit tests
test-helm:
    helm unittest ./charts/renovate-operator/ --file "unittests/**/*.yaml"

# Execute the over-the-wire webhook signing-token integration test (see src/integration/README.md)
test-integration:
    cd src && go test -tags integration -count=1 -v ./integration/...

# Execute golangci-lint
golangci-lint: generate
    cd src && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run --config=.golangci.yml '--timeout=1h' ./...


# Install JS dependencies
jsInstall:
    #!/usr/bin/env sh
    set -e
    mkdir -p src/static/js
    echo "Downloading Tailwind CSS..."
    curl -s -L -o src/static/js/tailwind.min.js "https://cdn.tailwindcss.com"
    echo "Downloading Babel Standalone..."
    curl -s -L -o src/static/js/babel.min.js "https://unpkg.com/@babel/standalone@8.0.1/babel.min.js"
    echo "Bundling React 19..."
    BUNDLE_DIR=$(mktemp -d)
    npm install --prefix "$BUNDLE_DIR" "react@19.2.8" "react-dom@19.2.8" esbuild --save=false --silent
    echo "import React from 'react'; import { createRoot } from 'react-dom/client'; export { React, createRoot };" > "$BUNDLE_DIR/entry.mjs"
    "$BUNDLE_DIR/node_modules/.bin/esbuild" "$BUNDLE_DIR/entry.mjs" \
        --bundle --format=esm --log-level=silent \
        --outfile=src/static/js/react-bundle.esm.js
    rm -rf "$BUNDLE_DIR"
    echo "All JavaScript dependencies ready!"

docker image:
    podman build --platform linux/arm64 \
        -t {{image}}-arm64 \
        -f ./Dockerfile .
    @echo "Creating manifest..."
    podman manifest rm {{image}} 2>/dev/null || true
    podman manifest create {{image}}
    @echo "Adding ARM64 to manifest..."
    podman manifest add {{image}} {{image}}-arm64
    @echo "Inspecting manifest:"
    podman manifest inspect {{image}}
    @echo "Pushing manifest:"
    podman manifest push --all {{image}}
