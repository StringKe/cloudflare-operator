# Build stage
FROM golang:1.24-alpine AS builder

# Enable Go toolchain auto-download for newer Go versions
ENV GOTOOLCHAIN=auto

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the mockserver binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /mockserver ./test/mockserver/cmd/main.go

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /mockserver /app/mockserver

EXPOSE 8787

ENTRYPOINT ["/app/mockserver"]
