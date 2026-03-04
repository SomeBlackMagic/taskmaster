# syntax=docker/dockerfile:1

# Stage 1: build
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/taskmaster ./cmd/taskmaster
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/testapp    ./test/testapp

# Stage 2: minimal runtime image
FROM scratch
COPY --from=builder /bin/taskmaster /bin/taskmaster
ENTRYPOINT ["/bin/taskmaster"]
