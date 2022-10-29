FROM golang:1.19-alpine AS builder

WORKDIR /rview

# Add git to embed commit hash and build time.
RUN apk update && apk add make git

COPY . .

RUN make build && ./bin/rview --version


FROM rclone/rclone:1.60

LABEL org.opencontainers.image.source=https://github.com/ShoshinNikita/rview
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /srv

COPY --from=builder /rview/bin .

ENTRYPOINT [ "./rview" ]
