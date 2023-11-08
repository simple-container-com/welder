---
title: 'Profiles and modes'
description: 'Profiles allow to tweak build configuration in a certain way'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Profiles

Profiles allow to specify different values of environment variables, arguments, steps and volumes when the specific
profile is activated.

For example, the behavior of the build may change depending on the host system or the type of CI, e.g. it may publish
artifacts into different repositories if the build is executed in a compliant CI environment or a non-compliant one.

```yaml
schemaVersion: 1.8.0
version: 1.0
default:
  build:
    args:
      namespace: dev
  dockerImages:
    - dockerFile: Dockerfile
      tags:
        - docker.simple-container.com/${arg:namespace}/service:${project:version}
profiles:
  sox:
    activation:
      sox: true
    build:
      args:
        namespace: sox
modules:
  - name: microservice
```

In the example above, whenever the profile `sox` is activated, the `namespace` argument will be set to `sox` and the
built Docker image will be prefixed with the `sox` namespace. By default, it would have `dev` namespace.

## Modes

Welder introduces a few modes that conditionally can activate profiles or can be used within expressions. Each
mode is activated by either passing a flag to `welder` command or implicitly derived from host environment 
build is running on.

Each mode can either activate a certain profile or be used from an [expression](/howto/expressions) 
to conditionally run a task.

There are the following flags:
* `--sox` activates "sox" mode which can be used to change configuration of the build
* `--skip-tests` activates "skip-tests" mode which can be used to skip certain tasks considered to be tests
* `--on-host` activates "on-host" mode which forces all steps and tasks to run on host environment instead of a Docker container
* `--verbose` activates "verbose" mode that produces debug messages from Welder
* `--no-cache` activates "no-cache" mode which disables caching of the built Docker images and forces them to be rebuilt every time
* `--sync-mode=<value>` activates one of the supported [volume](/howto/volumes) synchronization modes

Modes derived from environment are:
* `bamboo` activated whenever build is running on Bamboo
* `pipelines` activated when build is running on Bitbucket Pipelines

## Inheritance

All build configuration properties, including `args`, `env`, `volumes` and `steps` can be inherited from either default
config, profile or module configuration.
The inheritance of the values will be done in the following order: `default`, `profile`, `module`, `task`. That is 
`task` definitions has the highest priority and overrides all other configurations, whereas `default` has the lowest.
