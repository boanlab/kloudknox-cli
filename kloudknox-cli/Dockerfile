# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 BoanLab @ Dankook University

### Builder stage

ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags "-s -w -X github.com/boanlab/kloudknox-cli/cmd.Version=${VERSION}" \
        -o /out/kkctl .

### Runtime stage

FROM alpine:3.21

RUN apk --no-cache add ca-certificates \
    && rm -rf /var/cache/apk/*

COPY --from=builder /out/kkctl /usr/local/bin/kkctl

WORKDIR /work

ENTRYPOINT ["/usr/local/bin/kkctl"]
