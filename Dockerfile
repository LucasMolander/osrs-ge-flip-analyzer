# Step 1: Build the optimized Go binary in a Go builder environment
FROM golang:1.25.9-alpine AS builder

# Install SSL certificates and build tools
RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source directories
COPY cmd/ ./cmd/
COPY core/ ./core/
COPY web/ ./web/

# Compile statically linked binary with stripped debug symbols for minimum size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ge-analyzer ./cmd/ge-analyzer

# Step 2: Create a minimal runtime container
FROM alpine:latest

# Install CA certificates to enable secure HTTPS requests to OSRS Wiki APIs
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/ge-analyzer .

# Copy frontend static assets into runtime container
COPY web/frontend/ ./web/frontend/

# Expose the default port (Cloud Run overrides this via PORT env variable)
EXPOSE 8080

# Run the analyzer. If PORT environment variable is set, it will run as a web server.
# Otherwise, it can be run with CLI arguments overridden.
ENTRYPOINT ["./ge-analyzer"]
