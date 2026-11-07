FROM ghcr.io/rclone/rclone:1.76@sha256:7cbeec24a44f206067597c0368be4de90ae19a64d5113c7d1938bd737a81e466 AS rclone-src


FROM golang:1.27.3-alpine3.23@sha256:5d5113301ffcac84e2bfb20c42648803fecaea6789c89bb9b66884e18cbb30a7 AS tester

WORKDIR /rview

# For rclone - https://rclone.org/docs/#config-config-file
ENV XDG_CONFIG_HOME=/config

COPY --from=rclone-src /usr/local/bin/rclone /usr/local/bin/rclone

# Install dependencies (same as in the main Dockerfile).
RUN apk add --update --no-cache vips-tools libheif ca-certificates exiftool grep && \
	printf "\n" && \
	printf "vips version:     " && vips --version && \
	printf "exiftool version: " && exiftool -ver

# Install dev dependencies.
RUN apk add --no-cache make

COPY . .

RUN make test
