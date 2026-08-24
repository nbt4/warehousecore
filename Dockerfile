# Multi-stage build for WarehouseCore

# Stage 1: Build Frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web

# Copy frontend package files
COPY web/package*.json ./
RUN npm ci

# Copy frontend source
COPY web/ ./

# Build frontend
RUN npm run build

# Stage 2: Build Backend
FROM golang:1.25-alpine AS backend-builder

# Install build dependencies (CGO still needed for webp image processing)
RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled (needed for webp library)
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -o warehousecore ./cmd/server

# Stage 3: Final Image
FROM alpine:latest

# Install Chromium for headless label rendering and other runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    chromium \
    chromium-chromedriver \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont

# Set Chromium environment variables for headless operation
ENV CHROME_BIN=/usr/bin/chromium-browser \
    CHROME_PATH=/usr/lib/chromium/ \
    CHROMIUM_FLAGS="--disable-software-rasterizer --disable-dev-shm-usage"

WORKDIR /root/

# Copy binary from backend builder
COPY --from=backend-builder /app/warehousecore .

# Copy migrations
COPY --from=backend-builder /app/migrations ./migrations

# Copy LED configuration files
COPY --from=backend-builder /app/internal/led/config ./internal/led/config
COPY --from=backend-builder /app/internal/led/schema ./internal/led/schema

# Copy HTML template for label rendering
COPY --from=backend-builder /app/internal/services/label_template.html ./internal/services/

# Copy frontend build from frontend builder
COPY --from=frontend-builder /app/web/dist ./web/dist

# Create .env placeholder
RUN touch .env && mkdir -p /var/lib/branding/logos

EXPOSE 8082

# Health check with longer start period to allow server initialization
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8082/api/v1/health || exit 1

CMD ["./warehousecore"]
