FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS builder
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

FROM --platform=$BUILDPLATFORM node:26.13.0-alpine AS js-downloader
WORKDIR /workspace
RUN apk add --no-cache curl
RUN mkdir -p src/static/js && \
    echo "Downloading Tailwind CSS..." && \
    curl -s -L -o src/static/js/tailwind.min.js "https://cdn.tailwindcss.com" && \
    echo "Downloading Babel Standalone..." && \
    curl -s -L -o src/static/js/babel.min.js "https://unpkg.com/@babel/standalone@8.0.1/babel.min.js" && \
    echo "All JavaScript dependencies downloaded successfully!"
# React's production build: without the NODE_ENV define, esbuild bundles the
# development one, several times slower and five times the size.
RUN mkdir -p /bundle && \
    npm install --prefix /bundle "react@19.3.1" "react-dom@19.3.1" "@floating-ui/dom@1.8.0" esbuild --save=false && \
    echo "import React from 'react'; import { createRoot } from 'react-dom/client'; import { createPortal } from 'react-dom'; export { React, createRoot, createPortal };" \
        > /bundle/react.entry.mjs && \
    /bundle/node_modules/.bin/esbuild /bundle/react.entry.mjs \
        --bundle --format=esm --log-level=silent \
        --define:process.env.NODE_ENV='"production"' --minify \
        --outfile=src/static/js/react-bundle.esm.js && \
    echo "export { computePosition, autoUpdate, offset, flip, shift } from '@floating-ui/dom';" \
        > /bundle/floating-ui.entry.mjs && \
    /bundle/node_modules/.bin/esbuild /bundle/floating-ui.entry.mjs \
        --bundle --format=esm --log-level=silent \
        --define:process.env.NODE_ENV='"production"' --minify \
        --outfile=src/static/js/floating-ui.esm.js && \
    echo "Bundled React 19 and Floating UI successfully!"


FROM scratch
WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /workspace/renovate-operator /app/renovate-operator
COPY --from=builder /workspace/static /app/static
COPY --from=js-downloader /workspace/src/static/js /app/static/js
USER 1000:1000
ENTRYPOINT ["/app/renovate-operator"]
