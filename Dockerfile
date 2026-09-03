# syntax=docker/dockerfile:1.7

FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/slo-forge ./cmd/slo-forge

FROM scratch
COPY --from=build /out/slo-forge /slo-forge
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/slo-forge", "probe", "--url", "http://127.0.0.1:8080/healthz"]
ENTRYPOINT ["/slo-forge"]
CMD ["serve"]
