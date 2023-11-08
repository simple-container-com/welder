#!/usr/bin/env bash

set -e;

PLATFORM=linux
if [ "$(uname)" == "Darwin" ]; then
  PLATFORM=darwin
fi

BINDIR=~/.local/share/atlassian/atlas/plugin/

mkdir -p $BINDIR

if [[ ! -f "$BINDIR/build" ]]; then
  (
    cd $BINDIR &&
    curl -fL "https://statlas.prod.simple-container.com/atlas-cli-plugin-build/releases/latest/${PLATFORM}-amd64.tar.gz" | tar -xzp build  &&
    chmod +x build &&
    cd -
  )
fi

$BINDIR/build $@
