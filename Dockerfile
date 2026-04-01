# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o spotify-yearly-discoveries ./cmd/spotify-yearly-discoveries

# Final stage
FROM alpine:3.23

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/spotify-yearly-discoveries .

CMD ["./spotify-yearly-discoveries"]
