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


FROM scratch
WORKDIR /app
COPY --from=builder /workspace/renovate-operator /app/renovate-operator
COPY --from=builder /workspace/static /app/static
USER 1000:1000
ENTRYPOINT ["/app/renovate-operator"]
