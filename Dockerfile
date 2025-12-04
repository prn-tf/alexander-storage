# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o /bin/alexander-server \
    ./cmd/alexander-server

# Final stage
FROM alpine:3.23

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 alexander && \
    adduser -D -u 1000 -G alexander alexander

# Create data directories
RUN mkdir -p /data /config && \
    chown -R alexander:alexander /data /config

# Copy binary from builder
COPY --from=builder /bin/alexander-server /usr/local/bin/

# Copy default config
COPY configs/config.yaml.example /config/config.yaml

# Switch to non-root user
USER alexander

# Expose ports
EXPOSE 8080 8443

# Set data volume
VOLUME ["/data", "/config"]

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the server
ENTRYPOINT ["alexander-server"]
CMD ["--config", "/config/config.yaml"]
