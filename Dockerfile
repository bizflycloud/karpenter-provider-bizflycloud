# Build stage
FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto

# Declare build arguments AFTER FROM instruction
ARG VERSION=dev
ARG BUILDDATE

# Install build dependencies
RUN apk add --no-cache git ca-certificates make

# Set working directory
WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build with version information
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-X main.version=${VERSION} -X main.buildDate=${BUILDDATE} -w -s" \
    -a -o karpenter-provider-bizflycloud ./cmd/controller

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
ARG VERSION=dev
ARG BUILDDATE
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
