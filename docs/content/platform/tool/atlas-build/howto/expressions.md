---
title: 'Expressions'
description: 'Supported expressions allow to parameterize and conditionally execute build tasks'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Expressions

## Supported expressions

| expression                        | description                                                                                                                 |
|-----------------------------------|-----------------------------------------------------------------------------------------------------------------------------|
| `${arg:<name>:<default>}`         | [Arguments](/platform/tool/welder/howto/arguments-and-environment) passed to the CLI                                   |
| `${env:<name>:<default>}`         | [Environment variables](/platform/tool/welder/howto/arguments-and-environment) of the host environment                 |
| `${host:wd}`                      | Working directory on the host environment                                                                                   |
| `${host:projectRoot}`             | Root of the project on the host environment                                                                                 |
| `${container:home}`               | Home directory in the container (changes with the username)                                                                 |
| `${git:commit.short}`             | Current git commit (short hash value)                                                                                       |    
| `${git:commit.full}`              | Current git commit (full hash value)                                                                                        |    
| `${git:branch.raw}`               | Current git branch                                                                                                          |    
| `${git:branch.clean}`             | Current git branch (name with `/` symbol replaced with `-`) - useful for use in version                                     |    
| `${date:time}` `${date:dateOnly}` | Current formatted date                                                                                                      |    
| `${task:<name>.trim}`             | Execute task with `<name>`, trim spaces from both ends of the output and return value of the output                         |    
| `${task:<name>.raw}`              | Execute task with `<name>` and render value of the output                                                                   |    
| `${task:<name>.success}`          | Execute task with `<name>` and render `true` if execution was a success                                                     |    
| `${task:<name>.failed}`           | Execute task with `<name>` and render `false` if execution failed                                                           |    
| `${user:home}`                    | Print home directory of the currently executing user (on host environment)                                                  |    
| `${user:username}`                | Print username of the currently executing user (on host environment)                                                        |    
| `${user:id}`                      | Print id of the currently executing user (on host environment)                                                              |    
| `${user:name}`                    | Print full name of the currently executing user (on host environment)                                                       |    
| `${profile:<name>.active}`        | Renders `true` if a specific profile is active                                                                              |    
| `${mode:bamboo}`                  | Renders `true` whether the build is running in Bamboo                                                                       |
| `${mode:pipelines}`               | Renders `true` whether the build is running in Bitbucket Pipelines                                                          |
| `${mode:ci}`                      | Renders `true` whether the build is running in any CI environment                                                           |
| `${mode:sox}`                     | Renders `true` whether the build is running in the compliant environment                                                    |
| `${mode:skip-tests}`              | Renders `true` whether the build is running in the `skip-tests` mode                                                        |
| `${mode:on-host}`                 | Renders `true` whether the build is running with `on-host` mode                                                             |
| `${mode:verbose}`                 | Renders `true` whether the build is running in verbose mode                                                                 |
| `${mode:no-cache}`                | Renders `true` whether the build is running in `no-cache` mode                                                              |
| `${mode:sync-mode}`               | Renders volumes sync mode (e.g. `bind,copy,external,volume,add`). [more info](/platform/tool/welder/howto/performance) |
| `${project:name}`                 | Prints project's name as specified in the descriptor                                                                        |
| `${project:root}`                 | Prints path to root directory of the project                                                                                |
| `${project:version}`              | Prints version of the project as configured in the descriptor                                                               |
| `${project:module.name}`          | Prints name of the current module                                                                                           |
| `${project:module.path}`          | Prints path of the current module                                                                                           |
| `${project:env}`                  | Prints deployment environment (only available within a deployment task)                                                     |
| `${os:type.linux}`                | Renders `true` when current host operating system is Linux                                                                  |
| `${os:type.darwin}`               | Renders `true` when current host operating system is Darwin (MacOS)                                                         | 
| `${os:type.windows}`              | Renders `true` when current host operating system is Windows                                                                |
| `${os:name}`                      | Name of the operating system on the host (e.g. `darwin&#124;linux&#124;windows`)                                            |
| `${os:arch}`                      | Architecture on the host (e.g. `amd64`)                                                                                     |

## Examples

### Processing task outputs into values

```yaml
# ...
default:
  build:
    volumes:
      - "${task:gomodcache.trim}:${container:home}/go/pkg/mod"
# ...
tasks:
  gomodcache:
    runOn: host
    description: Returns directory of go mod cache on host environment
    script:
      - go env 2>/dev/null | grep GOMODCACHE | awk -F '=' '{print $2}' | sed 's/"//g'
```

Another example where our app needs to use Docker credentials to access the PAC registry:

```yaml
# ...
profiles:
  local:
  build:
    env:
      DOCKER_APN_USERNAME: ${user:username}
      DOCKER_APN_PASSWORD: ${task:local-docker-password.trim}
# ...
tasks:
  local-docker-password:
    runOn: host
    description: Print Docker password for packages.atlassian.com
    script:
      - welder docker effective-config | jq -r '.auths["docker.simple-container.com"].auth' | base64 -d | awk -F ":" '{ print $2 }'
```

### Using git commit hash as a part of the version

```yaml
schemaVersion: "1.0"
version: 1.0.0-${git:commit.short}
```

### Enable profile only on certain host operating system

```yaml
schemaVersion: 1.8.0
profiles:
  darwin:
    activation:
      if: "${os:type.darwin}"
    build:
      env:
        MACOS: "true"
```

### Enable profile only when build is running on CI (Bamboo or Bitbucket Pipelines)

```yaml
schemaVersion: 1.6.0
profiles:
  ci:
    activation:
      if: "${mode:ci}"
    build:
      env:
        CI: "true"
```