FROM caddy:latest as production-stage
ADD Caddyfile /etc/caddy/Caddyfile
COPY docs/site /var/www/html
ADD bin/welder-linux-amd64.tar.gz /var/www/html/releases/latest/linux-amd64.tar.gz
ADD bin/welder-darwin-amd64.tar.gz /var/www/html/releases/latest/darwin-amd64.tar.gz
ADD bin/welder-darwin-arm64.tar.gz /var/www/html/releases/latest/darwin-arm64.tar.gz

ADD bin/welder.schema.json /var/www/html/welder.schema.json
ADD welder.sh /var/www/html/welder.sh

EXPOSE 80