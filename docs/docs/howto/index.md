---
title: Main use-cases for Welder
description: What Welder can do for you?
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-23'
---

# Main use-cases for Welder

## Unified way of executing build for your project

Welder introduces `welder make` command that runs all steps specified in the build configuration. You do not
need to think what script to run, every project can be built with that command. There is also `welder all` that 
runs build and then builds and pushes Docker images.

## Running Bitbucket Pipelines

Unsure if you'd use an additional build configuration? You can still use Welder for simply running
your existing [Bitbucket Pipelines](/howto/running-bitbucket-pipelines/) configuration locally. 
Just navigate to your project's directory and invoke: 

```bash
welder bitbucket-pipelines execute all
```
OR
```bash
welder bitbucket-pipelines execute <clean-step-name>
```

This will run either all the steps in the pipeline or just the specified step. Welder will try to simulate the 
BBP environment and run the pipeline locally. Note: this feature is currently experimental and may not work in all cases.

## Containerized tasks

As described in [Simple usage](/howto/simple-usage/) section, you can use Welder to execute certain
tasks within containers and have declarative configuration for that.

```yaml
schemaVersion: 1.8.0
tasks:
  say-hello-in-alpine:
    image: alpine:latest
    script:
      - echo "hello"
```
Then you can run it by running the following command:
```bash
welder run say-hello-in-alpine
```

This can be useful for running tasks that are not available in the host environment or test certain capabilities of the
Docker images. While it may be seen as an overhead comparing to simple `docker run`, it allows to save your common tasks
and share them with your team.

## Declarative build for Docker image(s)

Often you need to build [multiple Docker images](/howto/build-and-test-service/#docker-images) 
for a service. This can be done using scripts or it can be easily maintained within Welder configuration:

```yaml
schemaVersion: 1.8.0
modules:
  - name: my-service
    dockerImages:
      - dockerFile: ${project:root}/Dockerfile
        tags:
          - docker.simple-container.com/my-service/main-image:latest
          - docker.simple-container.com/my-service/main-image:${git:commit.short}
```
Now you simply can run `welder docker build` and your docker image will be built with both tags. Additionally 
`welder docker push` can push both images to the repository.

## Generating CI/CD pipelines

Welder can [generate](/howto/generating-ci-cd-pipelines) Bamboo and Bitbucket Pipelines 
configurations based on `welder.yaml` file. This is a great way to get started with CI/CD after you've tested 
and configured your build locally.

To generate Bitbucket Pipelines configuration, simply run:

```bash
welder bitbucket-pipelines generate
```

## Multi-module builds and defaults

When you have [multiple modules](/howto/build-and-test-service/#multi-module-builds) in your 
project, you can use Welder to build them separately. For example, if you build your modules into containers in 
the same way and share the same Dockerfile, it's easy to configure Welder to build them individually:

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

Simple run of `welder docker build` will build docker images for both `service-a` and `service-b`.

## Seamless control over the version of your service

You can use [supported expressions](/howto/expressions) to control the version of your service. 
This allows you to easily template the version based on some parameter (e.g. environment variable) and then use it 
everywhere in your build configuration.

```yaml
schemaVersion: 1.8.0
version: 1.0.${env:BITBUCKET_BUILD_NUMBER}
default:
  dockerImages:
    - dockerFile: ${project:root}/Dockerfile
      tags:
        - docker.simple-container.com/my-service:${project:version}
```

## Profiles and conditional customizations of the build configuration

Welder allows to define [profiles](/howto/profiles-and-modes) with different configurations for 
different environments. For example, you can define a profile for production environment and another one for development.

```yaml
# ...
profiles:
  local:
    activation:
      if: "!${mode:ci}"
    build:
      args:
        release-suffix: "-alpha"
      env:
        RELEASE_CHANNEL: alpha
        SLAUTH_TOKEN: "${task:slauthtoken.trim}"
# ...
```

Another example where this can be applicable is when you want to run certain tasks only when build is running on 
different host operating system.

```yaml
# ...
profiles:
  macos:
    activation:
      if: "${os:type.darwin}"
    build:
      env:
        MACOS: "true"
# ...
tasks:
  build-mac-native-binary:
    runOn: host
    runIf: "${os:type.darwin}"
    script:
      - mvn package -Pnative
# ...
```