# Build stage
FROM golang:1.21-alpine as builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates make

# Set working directory
WORKDIR /workspace

# Build arguments
ARG VERSION=dev
ARG BUILDDATE

# Copy the Go Modules manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build with version information
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-X main.version=${VERSION} -X main.buildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ') -w -s" \
    -a -o karpenter-provider-bizflycloud ./cmd/karpenter-provider-bizflycloud

# Runtime stage
FROM alpine:3.18

# Add certificates and non-root user
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 65532 nonroot && \
    adduser -u 65532 -G nonroot -D nonroot

# Copy binary from builder stage
WORKDIR /
COPY --from=builder --chown=nonroot:nonroot /workspace/karpenter-provider-bizflycloud /karpenter-provider-bizflycloud

# Add version and build information
LABEL org.opencontainers.image.title="Karpenter Provider for BizFly Cloud" \
      org.opencontainers.image.description="Karpenter Provider implementation for BizFly Cloud" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.vendor="BizFly Cloud" \
      org.opencontainers.image.source="https://github.com/bizflycloud/karpenter-provider-bizflycloud" \
      org.opencontainers.image.created="${BUILDDATE}"

# Use non-root user
USER 65532:65532

# Define the entrypoint
ENTRYPOINT ["/karpenter-provider-bizflycloud"]