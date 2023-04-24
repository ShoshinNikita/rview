# Build rview.
FROM golang:1.20.1-alpine AS builder

WORKDIR /rview

# Add git to embed commit hash and build time.
RUN apk update && apk add make git

COPY . .

RUN make build && ./bin/rview --version


# Download rclone.
FROM rclone/rclone:1.62 AS rclone-src

RUN rclone --version


# Build the final image.
FROM alpine:3.16

LABEL org.opencontainers.image.source=https://github.com/ShoshinNikita/rview
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /srv

# Install vips
RUN apk add --update --no-cache vips-dev vips-tools ca-certificates && \
	vips --version

# Add rclone, rview and demo files.
COPY --from=rclone-src /usr/local/bin/rclone /usr/local/bin/rclone
COPY ./tests/testdata ./demo
COPY --from=builder /rview/bin .

# For rclone - https://rclone.org/docs/#config-config-file
ENV XDG_CONFIG_HOME=/config

ENTRYPOINT [ "./rview" ]
CMD [ "--rclone-target=./demo" ]
