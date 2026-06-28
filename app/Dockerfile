# syntax=docker/dockerfile:1.17

# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w" -o reflector .

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /app/reflector /reflector

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/reflector"]
