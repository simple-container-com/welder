FROM caddy:latest as production-stage
ADD Caddyfile /etc/caddy/Caddyfile
COPY docs/site /var/www/html
ADD bin/welder.schema.json /var/www/html/welder.schema.json
EXPOSE 80
