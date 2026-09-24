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

FROM cgr.dev/chainguard/static:latest@sha256:41e17ed83c594a64a9396b6ab96dd26d5ddc290dacf4c177464712ff21ad534f

WORKDIR /app
COPY --from=builder --chown=65532:65532 /out/kindle-weather /app/kindle-weather
COPY --chown=65532:65532 css/ /app/css/
COPY --chown=65532:65532 font/ /app/font/

USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=60s --timeout=5s --start-period=15s --retries=3 CMD ["/app/kindle-weather", "--healthcheck"]
ENTRYPOINT ["/app/kindle-weather"]
