---
title: 'Custom build images'
description: 'You can customize build images according to your preference'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Custom build images

Sometimes you may need to use a custom build image for your build tasks and the use of the base SDK image is not enough.
In this case Welder allows you to create a custom build image on-the-fly instead of wasting time and effort to 
manage additional build image.

## Custom image

With the `customImage` property you can specify a custom build image in any build task:

```yaml
schemaVersion: 1.8.0
tasks:
  build:
    customImage:
      dockerFile: ${project:root}/Dockerfile
    script:
      - go build ...
```
Where `${project:root}/Dockerfile` is a path to a Dockerfile that will be used to build the image before running your script tasks.

### Build parameters
Like any other Docker image built by Welder you can pass build arguments to the Dockerfile:

```yaml
schemaVersion: 1.8.0
tasks:
  build:
    customImage:
      dockerFile: ${project:root}/Dockerfile
      build:
        contextPath: ${project:root}/modules/server
        args:
          - name: version
            value: ${project:version}
```

## Inline Dockerfile

Welder also allows to use inline Dockerfiles right within `welder.yaml` descriptor to avoid additional Dockerfiles:

```yaml
schemaVersion: 1.8.0
tasks:
  build:
    customImage:
      # adding docker CLI to golang image so we can use it in tests
      inlineDockerFile: |-
        FROM golang:1.18
        ENV DOCKERVERSION=20.10.12
        RUN curl -fsSLO https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKERVERSION}.tgz \
          && tar xzvf docker-${DOCKERVERSION}.tgz --strip 1 \
                         -C /usr/local/bin docker/docker \
          && rm docker-${DOCKERVERSION}.tgz \
    script:
      - go build ...
```

In the above example we use `golang:1.18` image as a base and then install Docker CLI inside to allow docker commands
to be used in tests. 

{{% note %}}
You can use any of the supported [expressions](/platform/tool/welder/howto/expressions) within inline
dockerfile to parameterize it.
{{% /note %}}

