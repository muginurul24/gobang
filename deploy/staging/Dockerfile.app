FROM docker.io/library/golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGET_PACKAGE=./apps/api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/onixggr "${TARGET_PACKAGE}"

FROM docker.io/library/alpine:3.22

RUN apk add --no-cache ca-certificates curl tzdata

RUN addgroup -S onixggr && adduser -S -G onixggr onixggr

COPY --from=builder /out/onixggr /usr/local/bin/onixggr
COPY --from=builder /src/migrations /app/migrations
COPY --from=builder /src/seeds /app/seeds
COPY --from=builder /src/docs /app/docs

USER onixggr
WORKDIR /app

ENTRYPOINT ["/usr/local/bin/onixggr"]
