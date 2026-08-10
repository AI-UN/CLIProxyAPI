FROM golang:1.26-bookworm AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./

RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

# Distroless static requires a fully static binary, so dynamic library plugins are unavailable.
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./CLIProxyAPI ./cmd/server/

RUN install -D -m 0755 ./CLIProxyAPI /runtime/CLIProxyAPI/CLIProxyAPI \
    && install -D -m 0644 ./config.example.yaml /runtime/CLIProxyAPI/config.example.yaml \
    && mkdir -p /runtime/CLIProxyAPI/logs

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder --chown=65532:65532 /runtime/CLIProxyAPI /CLIProxyAPI

WORKDIR /CLIProxyAPI

EXPOSE 8317

ENV HOME=/home/nonroot \
    TZ=Asia/Shanghai \
    WRITABLE_PATH=/CLIProxyAPI

USER nonroot:nonroot

CMD ["/CLIProxyAPI/CLIProxyAPI"]
