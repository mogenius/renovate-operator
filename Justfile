export CGO_ENABLED := "0"

set dotenv-load

[private]
default:
    just --list --unsorted

# Run the application with flags similar to the production build
run: build jsInstall
    cd src && ../dist/native/renovate-operator

# Build a native binary with flags similar to the production build
build: generate
    cd src && go build -trimpath -gcflags="all=-l" -ldflags="-s -w" -o ../dist/native/renovate-operator ./cmd/main.go

# Build binaries for all targets
build-all: build-linux-amd64 build-linux-arm64 build-linux-armv7

# Build binary for target linux-amd64
build-linux-amd64: generate
    cd src && GOOS=linux GOARCH=amd64 go build -trimpath -gcflags="all=-l" -ldflags="-s -w" -o ../dist/amd64/renovate-operator ./cmd/main.go

# Build docker image for target linux-amd64
build-docker-linux-amd64:
    #!/usr/bin/env sh
    VERSION=$(git describe --tags $(git rev-list --tags --max-count=1))
    set -x
    docker buildx build --platform=linux/amd64 -f Dockerfile \
        --build-arg GOOS=linux \
        --build-arg GOARCH=amd64 \
        -t ghcr.io/mogenius/renovate-operator-dev:$VERSION-amd64 \
        -t ghcr.io/mogenius/renovate-operator-dev:latest-amd64 \
        .

# Build binary for target linux-arm64
build-linux-arm64: generate
    cd src && GOOS=linux GOARCH=arm64 go build -trimpath -gcflags="all=-l" -ldflags="-s -w" -o ../dist/arm64/renovate-operator ./cmd/main.go

# Build docker image for target linux-arm64
build-docker-linux-arm64:
    #!/usr/bin/env sh
    VERSION=$(git describe --tags $(git rev-list --tags --max-count=1))
    set -x
    docker buildx build --platform=linux/arm64 -f Dockerfile \
        --build-arg GOOS=linux \
        --build-arg GOARCH=arm64 \
        -t ghcr.io/mogenius/renovate-operator-dev:$VERSION-arm64 \
        -t ghcr.io/mogenius/renovate-operator-dev:latest-arm64 \
        .

# Build binary for target linux-armv7
build-linux-armv7: generate
    cd src && GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -gcflags="all=-l" -ldflags="-s -w" -o ../dist/armv7/renovate-operator ./cmd/main.go

# Build docker image for target linux-armv7
build-docker-linux-armv7:
    #!/usr/bin/env sh
    VERSION=$(git describe --tags $(git rev-list --tags --max-count=1))
    set -x
    docker buildx build --platform=linux/arm/v7 -f Dockerfile \
        --build-arg GOOS=linux \
        --build-arg GOARCH=arm \
        --build-arg GOARM=7 \
        -t ghcr.io/mogenius/renovate-operator-dev:$VERSION-armv7 \
        -t ghcr.io/mogenius/renovate-operator-dev:latest-armv7 \
        .

# Install tools used by go generate
_install_controller_gen:
    go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

# Execute go generate
generate: _install_controller_gen
    controller-gen crd paths=./src/... output:crd:dir=charts/renovate-operator/crds

# Run tests and linters for quick iteration locally.
check: generate golangci-lint test-unit

# Execute unit tests
test-unit: generate
    cd src && go run gotest.tools/gotestsum@latest --format="testname" --hide-summary="skipped" --format-hide-empty-pkg --rerun-fails="0" -- -count=1 ./...

# Execute golangci-lint
golangci-lint: generate
    cd src && go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run '--fast=false' --sort-results '--max-same-issues=0' '--timeout=1h' ./...


# Install JS dependencies
jsInstall:
    mkdir -p src/static/js
    echo "Downloading Tailwind CSS..."
    curl -s -L -o src/static/js/tailwind.min.js "https://cdn.tailwindcss.com"
    echo "Downloading React..."
    curl -s -L -o src/static/js/react.production.min.js "https://unpkg.com/react@18/umd/react.production.min.js"
    echo "Downloading React-DOM..."
    curl -s -L -o src/static/js/react-dom.production.min.js "https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"
    echo "Downloading Babel Standalone..."
    curl -s -L -o src/static/js/babel.min.js "https://unpkg.com/@babel/standalone/babel.min.js"
    echo "All JavaScript dependencies downloaded successfully!"
