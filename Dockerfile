# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /awg-converter ./cmd/awg-converter

FROM alpine:3.20
ARG VERSION=dev
LABEL org.opencontainers.image.title="mikwg awg-converter" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/nskforward/mikwg" \
      org.opencontainers.image.licenses="GPL-3.0-only"
RUN adduser -D -u 10001 awg
COPY --from=build /awg-converter /awg-converter
USER awg
ENTRYPOINT ["/awg-converter"]
