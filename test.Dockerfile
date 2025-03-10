FROM rclone/rclone:1.69 AS rclone-src

FROM golang:1.24-alpine3.21 AS tester

WORKDIR /rview

# Add rclone first for better caching.
ENV XDG_CONFIG_HOME=/config
COPY --from=rclone-src /usr/local/bin/rclone /usr/local/bin/rclone

# Install dependencies.
RUN apk add --update --no-cache vips-tools make libheif && \
	vips --version

COPY . .

RUN make test
