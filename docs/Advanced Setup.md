# Advanced Setup for Paranoiacs

You don't have to trust **Rview** - it can work isolated from the internet with access only
to the small subset of read-only Rclone commands. Unfortunately, Rclone itself doesn't support
fine-grained permissions. So, we have to use other tools - Nginx in our case.

First, create `entrypoint.sh` with the following content and replace all the variables.
Don't forget to run `chmod +x entrypoint.sh`.

```sh
#!/bin/sh

set -e

# Start Nginx

apk update && apk add nginx

cat <<EOT > /etc/nginx/http.d/rclone.conf
server {
  listen 8080;
  listen [::]:8080;

  # Serve dirs and files with no access to other remotes.
  location /[<your_rclone_target>]/ {
    proxy_pass http://localhost:5572;
  }
  # This command is required to build the search index. Read more: https://rclone.org/rc/#operations-list
  location /operations/list {
    proxy_pass http://localhost:5572;
  }
  location / {
    return 403;
  }
}
EOT

nginx

# Start Rclone. Don't forget to change values for '--rc-user' and '--rc-pass'!

rclone rcd \
  --rc-addr localhost:5572 \
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
      - ./var:/srv/var # mount app data directory
    ports:
      - "127.0.0.1:8080:8080"
    networks:
      - rview # connect rclone and rview
    command: [
      "--rclone-url",    "http://rclone:8080",
      "--rclone-user",   "user", # don't forget to change username and password
      "--rclone-pass",   "pass",
      "--rclone-target", "<your_rclone_target>"
    ]

  rclone:
    image: rclone/rclone:1.64
    container_name: rclone
    volumes:
      - ./entrypoint.sh:/entrypoint.sh                             # mount the script
      - ./rclone.gotmpl:/config/rclone/rclone.gotmpl               # template can be found in 'static' dir
      - ~/.config/rclone/rclone.conf:/config/rclone/rclone.conf:ro # mount Rclone config file
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

Optionally, you can check that everything is working as expected by using the following commands:

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
    wget -O - -q "http://rclone:8080/[<your_rclone_target>]/" # 401, auth is on
    wget -O - -q "http://user:pass@rclone:8080/[<your_rclone_target>]/" # 200, ok
    wget -O - -q "http://user:pass@rclone:8080/[/bin]/" # 403, no access to other remotes
    wget -O - -q --post-data "" "http://user:pass@rclone:8080/operations/list?fs=/bin&remote=" # 200, ok
    ```
