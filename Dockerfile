# SPDX-License-Identifier: BSD-3-Clause
# Copyright (c) 2026, Maciej Borzecki <maciej.borzecki@gmail.com>
# All rights reserved.
#
# Multi-stage Docker build for goodwe and goodwe-daemon.
# Runtime image: alpine:3.21 (~7 MB base).

# ---- Builder ----
FROM golang:1.26-alpine AS builder

ARG VERSION=dev

WORKDIR /src

# Cache module downloads separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" \
    -o /out/goodwe-daemon ./cmd/goodwe-daemon/ && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" \
    -o /out/goodwe ./cmd/goodwe/

# ---- Runtime ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

# Create the data directory with proper permissions.
RUN mkdir -p /var/lib/goodwe

COPY --from=builder /out/goodwe-daemon /usr/local/bin/goodwe-daemon
COPY --from=builder /out/goodwe /usr/local/bin/goodwe
COPY docker/entrypoint.sh /entrypoint.sh

WORKDIR /var/lib/goodwe
VOLUME /var/lib/goodwe
EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
CMD ["goodwe-daemon"]