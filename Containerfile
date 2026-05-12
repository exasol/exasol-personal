ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.23

FROM golang:$GO_VERSION-alpine$ALPINE_VERSION AS builder

RUN apk add git
RUN go install github.com/go-task/task/v3/cmd/task@latest

WORKDIR /src
COPY --link . .
RUN task generate build

FROM alpine:$ALPINE_VERSION

COPY --from=builder /src/bin/exasol /bin/
