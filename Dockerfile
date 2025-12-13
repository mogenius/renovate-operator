FROM --platform=$BUILDPLATFORM golang:1.25-alpine as builder
WORKDIR /workspace


ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

ENV CGO_ENABLED=0

COPY src/go.mod src/go.sum ./
RUN go mod download
COPY src .
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -trimpath -gcflags="all=-l" -ldflags="-s -w" -o renovate-operator ./cmd/main.go

FROM --platform=$BUILDPLATFORM alpine:latest as js-downloader
WORKDIR /workspace
RUN apk add --no-cache curl
RUN mkdir -p src/static/js && \
    echo "Downloading Tailwind CSS..." && \
    curl -s -L -o src/static/js/tailwind.min.js "https://cdn.tailwindcss.com" && \
    echo "Downloading React..." && \
    curl -s -L -o src/static/js/react.production.min.js "https://unpkg.com/react@18/umd/react.production.min.js" && \
    echo "Downloading React-DOM..." && \
    curl -s -L -o src/static/js/react-dom.production.min.js "https://unpkg.com/react-dom@18/umd/react-dom.production.min.js" && \
    echo "Downloading Babel Standalone..." && \
    curl -s -L -o src/static/js/babel.min.js "https://unpkg.com/@babel/standalone/babel.min.js" && \
    echo "All JavaScript dependencies downloaded successfully!"


FROM scratch
WORKDIR /app
COPY --from=builder /workspace/renovate-operator /app/renovate-operator
COPY --from=builder /workspace/static /app/static
COPY --from=js-downloader /workspace/src/static/js /app/static/js
USER 1000:1000
ENTRYPOINT ["/app/renovate-operator"]
