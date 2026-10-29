# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
# git lets go build stamp the module version from the copied .git into the build info.
# Build stage only, not in the image; a pinned Alpine version would break on every base update.
# hadolint ignore=DL3018
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/solarfocus-exporter ./cmd/solarfocus-exporter

FROM scratch
COPY --from=build /out/solarfocus-exporter /solarfocus-exporter
USER 65534:65534
EXPOSE 9851
ENTRYPOINT ["/solarfocus-exporter"]
