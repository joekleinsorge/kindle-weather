# syntax=docker/dockerfile:1.24

FROM --platform=$BUILDPLATFORM cgr.dev/chainguard/go:latest@sha256:072cb18c2146f22265a2d7862d37a92665a632060ae6fc0794750f2e7694ffe1 AS builder

ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY *.go ./
COPY templates/ ./templates/
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -buildid=" -o /out/kindle-weather .

FROM cgr.dev/chainguard/static:latest@sha256:77d8b8925dc27970ec2f48243f44c7a260d52c49cd778288e4ee97566e0cb75b

WORKDIR /app
COPY --from=builder --chown=65532:65532 /out/kindle-weather /app/kindle-weather
COPY --chown=65532:65532 css/ /app/css/
COPY --chown=65532:65532 font/ /app/font/

USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=60s --timeout=5s --start-period=15s --retries=3 CMD ["/app/kindle-weather", "--healthcheck"]
ENTRYPOINT ["/app/kindle-weather"]
