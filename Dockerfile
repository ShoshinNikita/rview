# Build rview.
FROM golang:1.24-alpine3.21 AS builder

WORKDIR /rview

# Add git to embed commit hash and build time.
RUN apk add --update --no-cache make git

COPY . .

RUN make build && go clean -cache && ./bin/rview --version


# Download rclone.
FROM rclone/rclone:1.69 AS rclone-src

RUN rclone --version


# Build the final image. The version should match one in test.Dockerfile.
FROM alpine:3.21

LABEL org.opencontainers.image.title="Rview"
LABEL org.opencontainers.image.description="Web-based UI for 'rclone serve'"
LABEL org.opencontainers.image.source="https://github.com/ShoshinNikita/rview"
LABEL org.opencontainers.image.url="https://github.com/ShoshinNikita/rview"
LABEL org.opencontainers.image.licenses="MIT"

EXPOSE 8080

WORKDIR /srv

# For rclone - https://rclone.org/docs/#config-config-file
ENV XDG_CONFIG_HOME=/config

# Add rclone first for better caching.
COPY --from=rclone-src /usr/local/bin/rclone /usr/local/bin/rclone

# Install vips.
RUN apk add --update --no-cache vips-tools libheif ca-certificates && \
	vips --version

# Add rview.
COPY --from=builder /rview/bin .

ENTRYPOINT [ "./rview" ]
