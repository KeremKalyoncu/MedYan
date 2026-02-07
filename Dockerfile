# Multi-stage build for minimal image size
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build API
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o bin/api ./cmd/api

# Build Worker
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o bin/worker ./cmd/worker

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates redis bash curl ffmpeg python3 py3-pip && \
    pip3 install --no-cache-dir --break-system-packages yt-dlp

# Set FFmpeg environment variables for memory optimization
ENV FFREPORT=file=/app/logs/ffmpeg-%t.log:level=32
ENV FFMPEG_THREADS=2
ENV FFMPEG_PATH=/usr/bin/ffmpeg

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/bin/api /app/bin/api
COPY --from=builder /app/bin/worker /app/bin/worker

# Copy entrypoint script
COPY docker-entrypoint.sh /app/
RUN chmod +x /app/docker-entrypoint.sh

# Create necessary directories
RUN mkdir -p logs tmp downloads && \
    chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose ports
EXPOSE 8080 6379

# Disable S3 by default - use local storage
ENV S3_ENDPOINT=disabled

# Health check
HEALTHCHECK --interval=30s --timeout=30s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Start with entrypoint script (runs Redis, Worker, and API)
CMD ["/app/docker-entrypoint.sh"]
