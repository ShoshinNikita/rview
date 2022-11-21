FROM golang:1.19-alpine AS builder

WORKDIR /rview

# Add git to embed commit hash and build time.
RUN apk update && apk add make git

COPY . .

RUN make build && ./bin/rview --version



FROM rclone/rclone:1.60 AS rclone-src

RUN rclone --version



FROM alpine:3.16

LABEL org.opencontainers.image.source=https://github.com/ShoshinNikita/rview
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /srv

# Install vips
RUN apk add --update --no-cache vips-dev vips-tools ca-certificates && \
	vips --version

# Add rclone
COPY --from=rclone-src /usr/local/bin/rclone /usr/local/bin/rclone

COPY --from=builder /rview/bin .

# For rclone
ENV XDG_CONFIG_HOME=/config

ENTRYPOINT [ "./rview" ]
