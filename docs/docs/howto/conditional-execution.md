---
title: 'Conditional execution'
description: 'Conditions allow to execute certain tasks only if certain conditions are met'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Conditional execution

## runIf

Welder offers conditional execution of tasks. For example, you may want to skip certain tasks when `skip-tests` mode
is enabled:

```yaml
tasks:
  run-tests:
    image: maven:3.5.0
    runIf: "!${profile:skip-tests.active}"
    script: ['mvn test']
```

Welder uses [expr](https://github.com/antonmedv/expr) library to evaluate expressions for `runIf`, so it supports all logical
operators implemented in this library.

## Conditional profiles

The same applies to `if` directive for a profile activation:

```yaml
profiles:
  pipelines:
    activation:
      pipelines: true
    build:
      env:
        PIPELINES_JWT_TOKEN: "${env:PIPELINES_JWT_TOKEN}"
        TESTCONTAINERS_RYUK_DISABLED: "true"
        TESTCONTAINERS_HOST_OVERRIDE: "${env:BITBUCKET_DOCKER_HOST_INTERNAL}"
  local:
    activation:
      if: "!${mode:bamboo} && !${mode:pipelines}"
    build: 
      env:
        TOKEN: "${task:token.trim}" # generate token for local testing against backend
# ...
tasks:
  token:
    runOn: host
    script:
      - generate-token --aud backend -o jwt
```
As shown above, profiles also can be activated when a certain mode is active. For example, you can activate a profile only when
your build is running on Bitbucket Pipelines as shown above.