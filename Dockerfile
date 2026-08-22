# Build rview.
FROM golang:1.27-alpine3.23 AS builder

WORKDIR /rview

# Install git to embed the commit hash and the build time.
RUN apk add --update --no-cache make git

COPY . .

RUN make build && go clean -cache && ./bin/rview --version


# Download rclone.
FROM ghcr.io/rclone/rclone:1.75 AS rclone-src

RUN rclone --version


# Build the final image. The alpine version should match one in test.Dockerfile.
FROM alpine:3.23

LABEL org.opencontainers.image.title="Rview"
LABEL org.opencontainers.image.description="Web-based UI for 'rclone serve'"
LABEL org.opencontainers.image.source="https://github.com/ShoshinNikita/rview"
LABEL org.opencontainers.image.url="https://github.com/ShoshinNikita/rview"
LABEL org.opencontainers.image.licenses="MIT"

EXPOSE 8080

WORKDIR /srv

# For rclone - https://rclone.org/docs/#config-config-file
ENV XDG_CONFIG_HOME=/config

RUN apk add --update --no-cache vips-tools libheif ca-certificates exiftool && \
	printf "\n" && \
	printf "vips version:     " && vips --version && \
	printf "exiftool version: " && exiftool -ver

COPY --from=rclone-src /usr/local/bin/rclone /usr/local/bin/rclone
COPY --from=builder /rview/bin .

ENTRYPOINT [ "./rview" ]
