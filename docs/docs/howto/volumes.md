---
title: 'Volumes'
description: 'How to use volumes properly with Welder containers'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Using volumes to perform builds

Volumes in Welder are pretty much the same as Docker volumes with a few specifics. The main reason why volumes 
are important for your builds is sharing some essential files and directories between host environment and a container.
Containers only provide the SDK for building and running your application, but you need to provide secrets and caches
to be download dependencies and publish artifacts.

`volumes` section can be declared under any `build` or `deploy` section within the descriptor as well as under any
task definition. Volumes are inherited from `default` section and can be overridden by the `profile` section.

## Maven

With Maven-based projects it is recommended to configure the following volumes in your descriptor:
```yaml
  build:
    volumes:
      - ~/.m2/repository:${container:home}/.m2/repository:delegated
      - ~/.m2/settings.xml:${container:home}/.m2/settings.xml:ro
      - ~/.m2/settings-security.xml:${container:home}/.m2/settings-security.xml:ro
```

These volumes will allow to share your host's configured Maven settings and security settings with build containers as 
well as reuse cached artifacts within local Maven repository.

## Gradle

For Gradle-based projects the following volumes configuration is recommended to share caches and settings with build
containers: 
```yaml
  build:
    volumes:
      - ~/.gradle/init.gradle:${container:home}/.gradle/init.gradle:ro
      - ~/.gradle/gradle.encrypted.properties:${container:home}/.gradle/gradle.encrypted.properties:ro
      - ~/.gradle/gradle.properties:${container:home}/.gradle/gradle.properties:ro
      - ~/.gradle/caches:${container:home}/.gradle/caches:delegated
      - ~/.gradle/wrapper:${container:home}/.gradle/wrapper:delegated
      - ~/.gradle/daemon:${container:home}/.gradle/daemon:delegated
```

## NodeJS

The following volumes configuration should allow to build NodeJS-based projects:
```yaml
  build:
    volumes:
      - ~/.npm:${container:home}/.npm:delegated
      - ~/.npmrc:${container:home}/.npmrc:ro
      - ~/.yarnrc:${container:home}/.yarnrc:ro
      - ~/.ssh:${container:home}/.ssh:ro
```

## Python

```yaml
  build:
    volumes:
      - ~/.cache/pip:${container:home}/.cache/pip
```

## Golang
For Golang projects you may want to configure git to get access to internal repos, and also to share your locally installed
packages:
```yaml
default:
  build:
    volumes:
      - "~/.gitconfig:${container:home}/.gitconfig:ro"
      - "~/.ssh:${container:home}/.ssh:ro"
      - "${task:gomodcache.trim}:${container:home}/go/pkg/mod"
    env:
      GOPRIVATE: "stash.atlassian.com,bitbucket.org" # allows to acces internal repos
      GOMODCACHE: ${container:home}/go/pkg/mod # shares installed go modules with container
# ...
tasks:
  gomodcache:
    runOn: host
    description: Returns directory of go mod cache on host
    script:
      - |-
        GOCACHEDIR="$( go env 2>/dev/null | grep GOMODCACHE | awk -F '=' '{print $2}' | sed 's/"//g' )"
        if [ -z "$GOCACHEDIR" ]; then echo "/tmp/modcache"; else echo "$GOCACHEDIR" ; fi
```

## GnuPG and signing of the artifacts

When you need to sign the built artifacts with GnuPG you may also want to share available keys with the container:
```yaml
  build:
    volumes:
      - ~/.gnupg:${container:home}/.gnupg
```
!!! note
    This volume can be added conditionally, for example only when build is running on the compliant environment (sox).

