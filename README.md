# Rview

<!-- TODO: description, features, screenshot, no auth protection (!) -->

- [Run](#run)
- [Configuration](#configuration)
- [Development](#development)

## Run

1. You have to install [docker](https://docs.docker.com/) and [docker compose](https://docs.docker.com/compose/) (optional, but recommended).
2. Let's consider you use Rclone S3 backend, and your `~/.config/rclone/rclone.conf` looks like this:

	```ini
	[my-s3]
	type = s3
	provider = Other
	access_key_id = <key id>
	secret_access_key = <access key>
	endpoint = <endpoint>
	```

3. Create `docker-compose.yml` with the following content:

	```yml
	version: "2"
	services:
	  rview:
	    image: ghcr.io/shoshinnikita/rview:main
	    container_name: rview
	    volumes:
	      - ./var:/srv/var # mount app data directory
	      - ~/.config/rclone/rclone.conf:/config/rclone/rclone.conf # mount Rclone config file
	    ports:
	      - "127.0.0.1:8080:8080"
	    command: "--rclone-target=my-s3:" # specify Rclone target from the config file
	```

4. Run `docker compose up`
5. Go to `http://localhost:8080`

## Configuration

| Flag                            | Default Value              | Description                                  |
| ------------------------------- | -------------------------- | -------------------------------------------- |
| `--debug-log-level`             | `false`                    | display debug log messages                   |
| `--dir`                         | `./var`                    | directory for app data (thumbnails and etc.) |
| `--port`                        | `8080`                     | server port                                  |
| `--rclone-port`                 | `8181`                     | port of a rclone instance                    |
| `--rclone-target`               | no default value, required | rclone target                                |
| `--read-static-files-from-disk` | `false`                    | read static files directly from disk         |
| `--resizer`                     | `true`                     | enable or disable image resizer              |
| `--resizer-max-age`             | `4320h` (180 days)         | max age of resized images                    |
| `--resizer-max-total-size`      | `209715200` (200 MiB)      | max total size of resized images, bytes      |
| `--resizer-workers-count`       | number of logical CPUs     | number of image resize workers               |

## Development

First, you have to install the following dependencies:

1. [rclone](https://github.com/rclone/rclone) - instructions can be found [here](https://rclone.org/install/).
2. [vips](https://github.com/libvips/libvips) - you can install it with this command:

	```bash
	sudo apt-get install libvips-tools
	```

After completion of these steps you should be able to run **Rview**:

```bash
# Build and run
make build && make run
# Or just
make

# Build, run tests and lint code
make check
```

By default `make run` uses environment variables from `.env` file. You can redefine these variables via `.env.local` file.
