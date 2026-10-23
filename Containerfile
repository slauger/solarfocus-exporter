# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
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
