# Build rview.
FROM golang:1.21-alpine AS builder

WORKDIR /rview

# Add git to embed commit hash and build time.
RUN apk update && apk add make git

COPY . .

RUN make build && ./bin/rview --version


# Download rclone.
FROM rclone/rclone:1.63 AS rclone-src

RUN rclone --version


# Build the final image.
FROM alpine:3.18

LABEL org.opencontainers.image.title="Rview"
LABEL org.opencontainers.image.description="Web-based UI for Rclone"
LABEL org.opencontainers.image.source="https://github.com/ShoshinNikita/rview"
LABEL org.opencontainers.image.licenses="MIT"

EXPOSE 8080

WORKDIR /srv

# For rclone - https://rclone.org/docs/#config-config-file
ENV XDG_CONFIG_HOME=/config

# Add rclone first for better caching.
COPY --from=rclone-src /usr/local/bin/rclone /usr/local/bin/rclone

# Install vips.
RUN apk add --update --no-cache \
		vips-tools=8.14.3-r0 \
		libheif=1.16.2-r0 \
		ca-certificates && \
	vips --version

# Add rview.
COPY --from=builder /rview/bin .

ENTRYPOINT [ "./rview" ]
