---
title: 'Arguments and environment'
description: 'Arguments allow to parametrize build configuration'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Arguments and environment variables

## Basic usage

You can use `${arg:<name>:<default>}` and `${env:<name>:<default>}` expressions pretty much everywhere across `welder.yaml` files.
`${arg:<name>:<default>}` is used to get arguments passed to the build script.
`${env:<name>:<default>}` is used to get environment variables from the host environment (e.g. `$HOME`).

Example: 
```yaml
${arg:build-parameters:}
```

## Arguments


```yaml
schemaVersion: 1.8.0
tasks:
  hello:
    image: ubuntu
    env:
      NAME: "${arg:name:John Doe}"
    script:
      - echo "HELLO $NAME!"
```
```bash
on-host:~$ welder run hello
run [hello]  - Running 1 scripts on host...
run [hello]  - Executing script: 'echo "HELLO $NAME!"'
run [hello] HELLO John Doe!
run [hello]  - Finished hello in 0s
```
```bash
on-host:~$ welder run hello -a name="Ilia"
run [hello]  - Running 1 scripts on host...
run [hello]  - Executing script: 'echo "HELLO $NAME!"'
run [hello] HELLO Ilia!
run [hello]  - Finished hello in 0s
```

## Environment variables

Atlas build allows to read environment variables from the host environment using `${env:<name>:<default>}` expression.

```yaml
schemaVersion: 1.8.0
tasks:
  say-hello:
    image: alpine:latest
    env:
      NAME: "John Doe"
    script:
      - echo "Hello, ${env:NAME} (aka ${NAME})!"
```

Simple run of `welder run say-hello` will result in the following output:
```bash
on-host:~$ NAME=Peter welder run say-hello 
run [say-hello]  - Running 1 scripts in container 'alpine:latest'...
run [say-hello] Hello, Peter (aka John Doe)!
run [say-hello]  - Finished say-hello in 3s
```

Note the difference between `${env:NAME}` and `${NAME}` (the latter is a variable within container whereas the former is 
taken from the host environment).