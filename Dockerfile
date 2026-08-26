# Stage 1: build
FROM golang:1.26.7-trixie AS builder

WORKDIR /build

# Download dependencies before copying source to leverage Docker layer cache.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source and build a statically linked binary.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/party2 ./cmd/party2

# Stage 2: minimal runtime image.
# distroless/static contains CA certificates and timezone data but no shell,
# no package manager, and no runtime toolchain.
FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder /out/party2 /party2

# Structured JSON logs go to stderr.
# The application reads PARTY2_DB_DSN from the environment.
EXPOSE 8080

ENTRYPOINT ["/party2"]
