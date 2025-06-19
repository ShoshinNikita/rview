# Advanced Setup for Paranoiacs

You don't have to trust `Rview` - it can work isolated from the internet with access only
to the small subset of read-only Rclone commands. Unfortunately, Rclone itself doesn't support
fine-grained permissions. So, we have to use other tools - for example, Nginx.

First, create `entrypoint.sh` with the following content and replace all the variables.
Don't forget to run `chmod +x entrypoint.sh`.

```sh
#!/bin/sh

set -e

apk update && apk add nginx

# Allow only specific requests from Rview to Rclone.
cat <<EOT > /etc/nginx/http.d/rclone.conf
server {
  listen 55720;
  listen [::]:55720;

  # Serve dirs and files with no access to other remotes.
  location /[<your_rclone_target>]/ {
    proxy_pass http://127.0.0.1:5572;
  }
  # This command is required to build the search index. Read more: https://rclone.org/rc/#operations-list
  location /operations/list {
    # Forbid access to other remotes. Note: target must be correctly encoded, e.g., '%2Fdata' instead of '/data'.
    if (\$arg_fs != '<your_rclone_target>') {
        return 403;
        break;
    }
    proxy_pass http://127.0.0.1:5572;
  }
  location / {
    return 403;
  }
}
EOT

# Proxy requests from outside to Rview. Unfortunately, we can't just forward port of 'rview' service
# because it is connected only to an internal network: https://github.com/moby/moby/issues/36174
cat <<EOT > /etc/nginx/http.d/proxy.conf
server {
  listen 8080;
  listen [::]:8080;

  location / {
    proxy_pass http://rview:8080;
  }
}
EOT

# Start Nginx
nginx

# Start Rclone. We bind it to '127.0.0.1', so no requests from other containers in
# the same networks can access it.
#
# Don't forget to change values for '--rc-user' and '--rc-pass'.
rclone rcd \
  --rc-addr 127.0.0.1:5572 \
  --rc-user user \
  --rc-pass pass \
  --rc-template /config/rclone/rclone.gotmpl \
  --rc-serve
```

Second, create `docker-compose.yml`:

```yaml
version: "2"

services:
  rview:
    image: ghcr.io/shoshinnikita/rview:main
    container_name: rview
    volumes:
      - ./var:/srv/var # mount app data directory to cache image thumbnails
    networks:
      - rview # connect rclone and rview
    command: [
      "--rclone-url",    "http://user:pass@rclone:55720", # don't forget to change username and password
      "--rclone-target", "<your_rclone_target>"
    ]

  rclone:
    image: rclone/rclone:1.70
    container_name: rclone
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - ./entrypoint.sh:/entrypoint.sh                             # mount the script you created in the first step
      - ./rclone.gotmpl:/config/rclone/rclone.gotmpl               # template can be found in 'static' dir
      - ~/.config/rclone/rclone.conf:/config/rclone/rclone.conf:ro # mount your Rclone config file
    networks:
      - internet # rclone must have access to the internet
      - rview    # connect rclone and rview
    entrypoint: "/entrypoint.sh"

networks:
  # Network with access to the internet.
  internet:
  # Internal network without access to the internet.
  rview:
    internal: true
```

Finally, you can run `docker compose up` and go to http://localhost:8080.

Optionally, you can check that everything works as expected:

1. Check `rclone` container
    ```sh
    docker exec -it rclone sh

    ping 1.1.1.1 # you should see pings
    exit
    ```
2. Check `rview` container
    ```sh
    docker exec -it rview sh

    ping 1.1.1.1 # no pings
    ping rclone  # you should see pings
    wget -O - -q "http://rclone:5572" # connection refused, no direct access to rclone
    wget -O - -q "http://rclone:55720/[<your_rclone_target>]/" # 401, auth is on
    wget -O - -q "http://user:pass@rclone:55720/[<your_rclone_target>]/" # 200, ok
    wget -O - -q "http://user:pass@rclone:55720/[/bin]/" # 403, no access to other remotes
    wget -O - -q --post-data "" "http://user:pass@rclone:55720/operations/list?fs=<your_rclone_target>&remote=" # 200, ok
    wget -O - -q --post-data "" "http://user:pass@rclone:55720/operations/list?fs=/bin&remote=" # 403, no access to other remotes
    ```
