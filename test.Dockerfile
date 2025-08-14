FROM ghcr.io/rclone/rclone:1.70 AS rclone-src


FROM golang:1.25-alpine3.22 AS tester

WORKDIR /rview

# For rclone - https://rclone.org/docs/#config-config-file
ENV XDG_CONFIG_HOME=/config

COPY --from=rclone-src /usr/local/bin/rclone /usr/local/bin/rclone

# Install dependencies (same as in the main Dockerfile).
RUN apk add --update --no-cache vips-tools libheif ca-certificates exiftool && \
	printf "\n" && \
	printf "vips version:     " && vips --version && \
	printf "exiftool version: " && exiftool -ver

# Install dev dependencies.
RUN apk add --no-cache make

COPY . .

RUN make test
