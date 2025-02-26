# `Rview` - web-based UI for `rclone serve`

<p align="center">
  <img src="docs/screenshots/dir_home.jpg" width="90%">
  <br>
  <i> More screenshots <a href="./docs/screenshots.md">here</a> </i>
</p>

***

- [Features](#features)
- [Limitations](#limitations)
- [Demo](#demo)
- [Run](#run)
- [Configuration](#configuration)
- [Development](#development)
  - [Metrics \& Pprof](#metrics--pprof)
- [Thanks](#thanks)

## Features

- :framed_picture: **Automatic thumbnail generation**: You don't have to download hundreds of MiBs to preview your images.
  Image thumbnails are generated with the help of [libvips](https://github.com/libvips/libvips), an extremely
  fast image processing library.
- :iphone: **Mobile-friendly**: `Rview` can be installed as a PWA, desktop and mobile versions have feature parity.
- :mag: **Search**: You can search for files by their name. Search tips can be found [here](./docs/search.md).
- :feather: **Lightweight & minimalistic**: All pages are rendered on the server side using Go templates. JavaScript
  is used only to make UI interactive.

## Limitations

- `Rview` is read-only and there are no plans to change that. You should use Rclone directly
  to upload, edit, or delete files.
- `Rview` does not provide any authentication mechanism. It is therefore highly recommended to use a proxy
  such as Nginx or Caddy. Enabling gzip compression is also recommended, as it can significantly improve response time.

## Demo

Check out the live demo [here](https://rview.0x5f3759df.stream), credentials for Basic Auth: `rview:rview`.

## Run

1. You have to install [docker](https://docs.docker.com/) and [docker compose](https://docs.docker.com/compose/).
2. Let's consider you use Rclone S3 backend, and your `~/.config/rclone/rclone.conf` looks like this:

   ```ini
   [my-s3]
   type = s3
   provider = Other
   access_key_id = <key id>
   secret_access_key = <access key>
   endpoint = <endpoint>
   ```

3. Create `compose.yml`:

   ```yml
   services:
     rview:
       image: ghcr.io/shoshinnikita/rview:main
       container_name: rview
       volumes:
         - ./var:/srv/var                                          # mount app data directory
         - ~/.config/rclone/rclone.conf:/config/rclone/rclone.conf # mount Rclone config file
       ports:
         - "127.0.0.1:8080:8080"
       command: "--rclone-target=my-s3:" # pass Rclone target from the config file
   ```

4. Run this command:

   ```sh
   docker compose up
   ```

5. Go to http://localhost:8080.

> [!TIP]
> `rclone.conf` is not required when mounting a local directory:
> <details>
>   <summary><code>compose.yaml</code></summary>
>
>   ```yml
>   services:
>     rview:
>       image: ghcr.io/shoshinnikita/rview:main
>       container_name: rview
>       volumes:
>         - ./var:/srv/var      # mount app data directory
>         - /dir/to/mount:/data # mount data directory
>       ports:
>         - "127.0.0.1:8080:8080"
>       command: "--rclone-target=/data"
>   ```
>
>   </div>
> </details>

> [!TIP]
> You can run `Rview` with an existing Rclone instance and without access to the
> internet - [read more](./docs/advanced_setup.md).

## Configuration

```
--rclone-target                   Rclone target, required

--rclone-url                      Url of an existing rclone instance, optional. If url is
                                  not specified, a local rclone instance will be launched
                                  with the default config file. Url should include credentials
                                  for Basic Auth, e.g., http://user:pass@rclone:80

--rclone-cache-size               Max size of original file cache (default: 300Mi)

--rclone-dir-cache-ttl            Time to cache dir entries for. Set to 0 to disable. (default: 5m)

--dir                             Directory for app data: thumbnails and etc. (default: ./var)

--port                            Server port (default: 8080)

--image-preview-mode              Available image preview modes:
                                    - thumbnails (default): generate thumbnails
                                    - original: show original images
                                    - none: don't show preview for images

--thumbnails-format               Available thumbnail formats:
                                    - avif (default): AVIF images can be significantly smaller than
                                            JPEGs (-43% on average) and supported by all modern
                                            browsers. However, generation of .avif thumbnails takes
                                            more time (+32% on average) and requires more resources.
                                    - jpeg: fast thumbnail generation, large files

--thumbnails-cache-size           Max size of thumbnail cache (default: 500Mi)

--thumbnails-workers-count        Number of workers for thumbnail generation (default: # of threads)

--read-static-files-from-disk     Read static files directly from disk

--log-level                       Set the minimal log level. One of: debug, info (default),
                                  warn, error

--version                         Print version and exit
```

## Development

First, you have to install the following dependencies:

1. [Rclone](https://github.com/rclone/rclone) - instructions can be found [here](https://rclone.org/install/).
2. [libvips](https://github.com/libvips/libvips) - on Ubuntu you can install it with the following command:

	```bash
	sudo apt-get install libvips-tools
	```

After completion of these steps you should be able to run `Rview`:

```bash
# Build and run
make build && make run
# Or just
make

# Build, run tests and lint code
make check
```

By default `make run` uses environment variables from `.env` file. You can redefine these variables in `.env.local` file.

### Metrics & Pprof

`Rview` exposes Prometheus metrics on `/debug/metrics`. The list of all metrics can be found [here](pkg/metrics/metrics.go)

[Pprof endpoints](https://pkg.go.dev/net/http/pprof#hdr-Usage_examples) are available on `/debug/pprof/`.

## Thanks

Special thanks to these open-source projects:

- [Rclone](https://github.com/rclone/rclone) - rsync for cloud storage.
- [libvips](https://github.com/libvips/libvips) - A fast image processing library with low memory needs.
- [Material Icon Theme](https://github.com/PKief/vscode-material-icon-theme) - Material Design icons for VS Code.
- [Feather](https://github.com/feathericons/feather) - Simply beautiful open source icons.
