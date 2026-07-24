ARG BIN_NAME=ebird-rss
ARG BIN_VERSION=<unknown>

FROM golang:1-alpine AS builder
ARG BIN_NAME
ARG BIN_VERSION

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-X main.version=${BIN_VERSION}" -o ./out/${BIN_NAME} .

FROM scratch
ARG BIN_NAME
COPY --from=builder /src/out/${BIN_NAME} /usr/bin/${BIN_NAME}

ENTRYPOINT ["/usr/bin/ebird-rss"]

LABEL license="GPL-3.0"
LABEL org.opencontainers.image.licenses="GPL-3.0"
LABEL maintainer="Chris Dzombak <https://www.dzombak.com>"
LABEL org.opencontainers.image.authors="Chris Dzombak <https://www.dzombak.com>"
LABEL org.opencontainers.image.url="https://github.com/cdzombak/ebird-rss"
LABEL org.opencontainers.image.documentation="https://github.com/cdzombak/ebird-rss"
LABEL org.opencontainers.image.source="https://github.com/cdzombak/ebird-rss"
LABEL org.opencontainers.image.version="${BIN_VERSION}"
LABEL org.opencontainers.image.title="${BIN_NAME}"
LABEL org.opencontainers.image.description="Generate an RSS, Atom, or JSON feed of your most recent bird sightings from an eBird CSV export"
