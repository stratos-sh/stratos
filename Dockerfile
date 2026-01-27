# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /workspace

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev) -X main.commit=$(git rev-parse HEAD 2>/dev/null || echo unknown) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o stratos \
    ./cmd/stratos

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy binary from builder
COPY --from=builder /workspace/stratos .

# Run as non-root user
USER 65532:65532

ENTRYPOINT ["/stratos"]
