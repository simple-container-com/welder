---
title: 'Build and test services'
description: 'Description of how to use Welder for building services'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Build and test services

One of the goals of Welder was to provide a way of executing one simple command to build a service. Given the
variety of SDKs developers are operating with, it is sometimes hard to know how exactly to build a service.
Welder introduces one simple command that should run a build no matter which SDK is used: `welder make`.

## Simple build

For any project that wants to incorporate `welder make` into its build process, it should have the following 
`welder.yaml` descriptor in the root of the project:

```yaml
schemaVersion: 1.8.0
modules:
  - name: my-service
    build:
      steps:
        - step:
            image: maven:3.5.0
            script:
              - mvn clean install
```

Instead of `step` under set of `steps` you can also run a task in the following way:

```yaml
schemaVersion: 1.8.0
modules:
  - name: my-service
    build:
      steps:
        - task: build
tasks:
  build:
    image: maven:3.5.0
    script:
      - mvn clean package
```

### Running certain tasks "on host"

Sometimes you may want to just execute a task on the host machine. This might be useful when you're certain that commands
executed by the task are cross-platform and must be present on the host machine.

```yaml
tasks:
  build:
    runOn: host
    script:
      - generate-token --audience "bitbucket.org" --scopes account --output header
```

In the above example we are generating OAuth token for Bitbucket. This could be used within other commands to access
Bitbucket APIs.

### Default build configuration and profiles

All build steps can inherit environment variables and argument values declared in the `default` section of the descriptor.

```yaml
schemaVersion: 1.8.0
default:
  build:
    steps:
      - task: build
    env:
      MAVEN_OPTS: ${arg:maven-opts:-Xmx2048m}
modules:
  - name: microservice1
  - name: microservice2
tasks:
  build:
    image: maven:3.5.0
    script:
      - mvn clean package
```

In the example above we are setting the `MAVEN_OPTS` environment variable to `-Xmx2048m` if the `maven-opts` argument is not set. 
This value as well as the build steps defined in the `build` section will be used by both `microservice1` and `microservice2`.

By using [profiles](/howto/profiles-and-modes) you can also specify environment variables, arguments
and steps. The inheritance of the values will be done in the following order: `default`, `profile`, `module`, `task`.

### Integration tests

Welder containers provide the access to Docker daemon by default, so every SDK used within a container should 
be able to easily use Docker and start up test dependency services (such as databases, queues etc.). This allows to
use frameworks like TestContainers within your build and produce binaries intended for the target platform, which 
will be used for deployments.

You can even start Docker Compose configurations from within Welder containers without installing Compose on your
host machine, e.g.:

```yaml
tasks:
  integration-test:
    image: docker/compose:1.24.0
    script:
      - |
        echo "Running integration tests with Docker Compose..."
        docker-compose -p release-tracker up --abort-on-container-exit \
            --force-recreate --build --remove-orphans \
            --exit-code-from integration-tests integration-tests
```

### Docker images

Welder allows to declare multiple Docker images that should be built for every declared module:
```yaml
schemaVersion: 1.8.0
modules:
  - name: my-service
    build:
      steps:
        - task: build
    dockerImages:
      - name: frontend
        dockerFile: Dockerfile.frontend
        tags:
          - docker.simple-container.com/my-service/frontend:latest
      - name: backend
        dockerFile: Dockerfile.backend
        tags:
          - docker.simple-container.com/my-service/backend:latest
# ...
```

Then you can easily build both of these images via `welder docker build` and push to repository via `welder docker push`.
If you have multiple Docker images declared in your descriptor you can pass the name of the image to the command, e.g.
`welder docker build frontend`.

Sometimes it is useful to perform certain actions after docker build or push are finished, e.g. trigger some job in CI system.
This can be done via `runAfterBuild` and `runAfterPush` properties of every Docker image:

```yaml
    dockerImages:
      - name: my-service
        dockerFile: ${project:root}/Dockerfile
        tags:
          - docker.simple-container.com/my-service:latest
        runAfterBuild: # run this command after build is finished
          runOn: host
          script:
            - "echo 'Built image: ${docker:tags[0].image}, tag: ${docker:tags[0].tag}'"
        runAfterPush: # run this command after push is finished
          runOn: host
          script:
            - "echo 'Pushed image: ${docker:tags[0].digest}'" # accessing digest of the first tag
```

Welder will run `runAfterBuild` script on the host machine after docker build is finished and `runAfterPush` 
script on the host machine after docker push is finished.

## Multi-module builds

Welder allows to have multiple modules in a single project. This is useful when you have a monorepo with multiple
different SDKs, operational and deployment configurations, CI/CD configs etc. Also sometimes you may want to have
integration tests as a separate module.

```yaml
schemaVersion: 1.8.0
modules:
  - name: frontend
    build:
      steps:
        - task: build-frontend
  - name: backend
    build:
      steps:
        - task: build-backend
```

When your project consists of multiple microservices it might be convenient to have separate module for each of them.

```yaml
schemaVersion: 1.8.0
default:
  dockerImages:
    - dockerFile: ${project:root}/Dockerfile
      tags:
        - docker.simple-container.com/my-service/${project:module.name}:latest
        - docker.simple-container.com/my-service/${project:module.name}:${git:commit.short}
modules:
  - name: service-a
  - name: service-b
```

### Contextual builds

Welder can detect how the current directory relates to the project structure. Imagine you have multiple modules in 
your project and each of them has its own directory. In this case you can define the path to the module, which will allow
Welder to run the build steps only for the current module if build is started from its directory, e.g.:

```yaml
modules:
  - name: microservice1
    path: ${project:root}/microservice1
  - name: microservice2
    path: ${project:root}/microservice2
```

That is when your current directory is `microservice1` and you run `welder make` it will run only build steps for
`microservice1` module. And the same applies to `microservice2`.

!!! note
    This only works when there is no conflicting paths between modules in which case this behavior is not guaranteed.
