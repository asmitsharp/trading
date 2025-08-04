# ---- Build Stage ----
FROM golang:1.23.1-alpine AS builder

WORKDIR /app

# Install git for go mod and build tools
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the Go app (adjust the output binary name as needed)
RUN CGO_ENABLED=0 GOOS=linux go build -o trading ./cmd/main_rest.go

# ---- Run Stage ----
FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder
COPY --from=builder /app/trading .

# Expose the port your app listens on (change if needed)
EXPOSE 8080

# Run the binary
CMD ["./trading"]
