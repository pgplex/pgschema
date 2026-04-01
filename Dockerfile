# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git

# Set working directory
WORKDIR /build

# Copy go mod files for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version info
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# Build with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X github.com/pgplex/pgschema/cmd.GitCommit=${GIT_COMMIT} -X 'github.com/pgplex/pgschema/cmd.BuildDate=${BUILD_DATE}'" \
    -a \
    -o pgschema .

# Final stage - Alpine for debugging capabilities
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata && \
    adduser -D -g '' pgschema

# Copy binary from builder
COPY --from=builder /build/pgschema /usr/local/bin/pgschema

# Switch to non-root user
USER pgschema

# Set entrypoint
ENTRYPOINT ["pgschema"]