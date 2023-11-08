FROM caddy:latest as production-stage
ADD Caddyfile /etc/caddy/Caddyfile
COPY site /var/www/html
EXPOSE 80
