FROM golang:1.25-alpine AS build

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .


# Build the server binary
RUN go build -o server ./cmd/example_server/main.go

# --- Runtime stage ---
FROM alpine:3.20 AS runtime
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=build /build/server ./server

# Set the entrypoint to run the server binary
ENTRYPOINT ["./server"]
