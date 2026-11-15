# Build stage
FROM golang:1.27.4-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /kubeguard \
    ./cmd/kubeguard

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /kubeguard /kubeguard

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/kubeguard"]