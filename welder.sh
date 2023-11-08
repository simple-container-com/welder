#!/usr/bin/env bash

set -e;

PLATFORM=linux
if [ "$(uname)" == "Darwin" ]; then
  PLATFORM=darwin
fi

ARCH=amd64
case $(uname -m) in
    x86_64) ARCH="amd64" ;;
    arm)    ARCH="arm64" ;;
esac
BINDIR=~/.local/bin

mkdir -p $BINDIR

if [[ ! -f "$BINDIR/welder" ]]; then
  (
    cd $BINDIR &&
    curl -fL "https://welder.simple-container.com/releases/latest/${PLATFORM}-${ARCH}.tar.gz" | tar -xzp build  &&
    chmod +x build &&
    export PATH="$PATH:$BINDIR" &&
    cd -

    path_export="export PATH=\"\$PATH:$BINDIR\""
    if [[ "$(cat ~/.bashrc | grep "$path_export")" == "" ]]; then
      echo "$path_export" >> ~/.bashrc
    fi
  )
fi

$BINDIR/build $@
