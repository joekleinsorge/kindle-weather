# syntax=docker/dockerfile:1.26

FROM --platform=$BUILDPLATFORM cgr.dev/chainguard/go:latest@sha256:7587e9368da2f2abbb2e569e5b4d364b69c4e53bddd735c89ecd63146cd0b6ae AS builder

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

FROM cgr.dev/chainguard/static:latest@sha256:207a5673ab31ed83332e54ae33d0f1de4adb5984bd93b8309789889e7bf30ba6

WORKDIR /app
COPY --from=builder --chown=65532:65532 /out/kindle-weather /app/kindle-weather
COPY --chown=65532:65532 css/ /app/css/
COPY --chown=65532:65532 font/ /app/font/

USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=60s --timeout=5s --start-period=15s --retries=3 CMD ["/app/kindle-weather", "--healthcheck"]
ENTRYPOINT ["/app/kindle-weather"]
