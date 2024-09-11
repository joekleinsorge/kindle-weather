FROM --platform=$BUILDPLATFORM cgr.dev/chainguard/go:latest AS builder
ARG TARGETARCH
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -a -installsuffix cgo -o kindle-weather .

FROM cgr.dev/chainguard/static:latest
WORKDIR /app
COPY --from=builder /app/kindle-weather .
COPY css/ ./css/
COPY font/ ./font/
EXPOSE 8080
USER nonroot
ENTRYPOINT ["./kindle-weather"]
