FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine AS builder
WORKDIR /workspace


ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

ENV CGO_ENABLED=0
ARG VERSION=dev

COPY src/go.mod src/go.sum ./
RUN go mod download
COPY src .
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -tags timetzdata -trimpath -gcflags="all=-l" -ldflags="-s -w -X main.Version=${VERSION}" -o renovate-operator ./cmd/main.go

FROM --platform=$BUILDPLATFORM node:24.16.0-alpine AS js-downloader
WORKDIR /workspace
RUN apk add --no-cache curl
RUN mkdir -p src/static/js && \
    echo "Downloading Tailwind CSS..." && \
    curl -s -L -o src/static/js/tailwind.min.js "https://cdn.tailwindcss.com" && \
    echo "Downloading Babel Standalone..." && \
    curl -s -L -o src/static/js/babel.min.js "https://unpkg.com/@babel/standalone@8.0.1/babel.min.js" && \
    echo "All JavaScript dependencies downloaded successfully!"
RUN mkdir -p /bundle && \
    npm install --prefix /bundle "react@19.2.7" "react-dom@19.2.7" esbuild --save=false && \
    echo "import React from 'react'; import { createRoot } from 'react-dom/client'; export { React, createRoot };" \
        > /bundle/entry.mjs && \
    /bundle/node_modules/.bin/esbuild /bundle/entry.mjs \
        --bundle --format=esm --log-level=silent \
        --outfile=src/static/js/react-bundle.esm.js && \
    echo "Bundled React 19 successfully!"


FROM scratch
WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /workspace/renovate-operator /app/renovate-operator
COPY --from=builder /workspace/static /app/static
COPY --from=js-downloader /workspace/src/static/js /app/static/js
USER 1000:1000
ENTRYPOINT ["/app/renovate-operator"]
