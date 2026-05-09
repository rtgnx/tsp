FROM ghcr.io/tailscale/tailscale:stable AS tailscale
FROM golang:1.26-alpine AS build

WORKDIR /build
COPY . .
ARG TARGET=tsp
RUN go test -v ./...
RUN go mod vendor && \
        CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o /$TARGET ./cmd/$TARGET && \
        apk add upx binutils && \
        strip /${TARGET} && \
        upx /${TARGET} && \
        ls -alh /${TARGET}

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /tsp /tsp
COPY --from=tailscale /usr/local/bin/tailscaled /tailscaled
COPY --from=tailscale /usr/local/bin/tailscale /tailscale

ENTRYPOINT [ "/tsp" ]
