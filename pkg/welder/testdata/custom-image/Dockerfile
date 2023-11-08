FROM alpine:latest
ARG module=unknown

USER root

RUN mkdir -p /build && \
    echo "module=${module}; user=$(whoami)" > /build/created-in-dockerfile && \
    chmod -R a+r /build
