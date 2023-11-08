---
title: Simple Usage
description: How to use Welder for running tools within containers
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-23'
---

# Simple usage of Welder

Imagine you need to run a single command within a container with a specific SDK (e.g. Java). Normally you would use Docker CLI to 
start it up and pass all the necessary arguments to integrate container's environment with local secrets and configurations:
```bash
docker run --rm -it \
   --volume=$HOME/.m2/repository:/root/.m2/repository \
   --volume=$HOME/.m2/settings.xml:/root/.m2/settings.xml:ro \
   --volume=$HOME/.m2/settings-security.xml:/root/.m2/settings-security.xml:ro \
   --volume=$HOME/.git:/root/.git:ro \
   --volume=$HOME/.ssh:/root/.ssh:ro \
   --volume=$(pwd):/root/src \
   --workdir=/root/src \
   maven:3.6-openjdk-11 \
    clean install
```

The same thing could be done with Welder in the following way (`welder.yaml` descriptor):
```yaml
schemaVersion: 1.8.0
tasks:
  build:
    image: maven:3.6-openjdk-11
    volumes:
      - ~/.m2/repository:${container:home}/.m2/repository
      - ~/.m2/settings.xml:${container:home}/.m2/settings.xml:ro
      - ~/.m2/settings-security.xml:${container:home}/.m2/settings-security.xml:ro
      - ~/.git:${container:home}/.git:ro
      - ~/.ssh:${container:home}/.ssh:ro
    script:
      - mvn clean install
```
Then you can run it by running the following command:
```bash
welder run build
```

You can declare different tasks and invoke them with a simple command if necessary.
The declared tasks can be referenced from the build lifecycle of a project, e.g.:

```yaml
schemaVersion: 1.8.0
modules:
  - name: backend
    build:
      steps:
        - task: build
tasks:
  build:
    image: maven:3.6-openjdk-11
    script:
      - mvn clean install
```

When you invoke `welder make` it'll run the `build` task. It is clear that you can run multiple tasks as steps 
within your build definition. Each task can use different image and script.

For a more detailed description of the build lifecycle, please see [build and test service](/platform/tool/welder/howto/build-and-test-service).