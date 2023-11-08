FROM caddy:latest as production-stage
ADD Caddyfile /etc/caddy/Caddyfile
COPY docs/site /var/www/html
COPY bin/*.tar.gz /var/www/html/releases/latest/

ADD bin/welder.schema.json /var/www/html/welder.schema.json
ADD welder.sh /var/www/html/welder.sh

EXPOSE 80