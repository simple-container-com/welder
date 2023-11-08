---
title: Motivation
description: Description of why Welder is a useful tool for development of services
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-23'
---

# Why Welder?

### Bitbucket Pipelines and scripts
Bitbucket Pipelines is the first choice for Atlassians to run their CI/CD pipelines. While it allows to easily configure
your CI pipeline that is executed in the cloud using variety of containerized SDKs, it lacks the possibility of
running individual steps locally. In addition, sometimes it is hard to figure out what is actually going on in the
container running in the pipeline. And it becomes a little tricky to debug the pipeline itself.
While most of the services use Bitbucket Pipelines as their CI/CD pipeline, Atlassians are also using scripts to
run builds locally. In fact the majority of services would simply invoke the script from their BBP configuration
file, so that the script can also be debugged and executed locally. The typical simplified `bitbucket-pipelines.yml` file
looks like this:

```yaml
image: python:3.8

pipelines:
  default:
    - step: 
        name: Build the service
        script:
          - pipe: atlassian/artifactory-sidekick:v1
          - source .artifactory/activate.sh
          - ./scripts/build.sh
```
There are a few issues with such approach:
* It is declarative only when it comes to CI configuration. You need to actually review the code of `build.sh` to understand what's going to be built and when
* It is not unified (i.e. each project may have different set of scripts that are not always easily locatable in the project). 
  This is especially important with projects implemented in different languages. Each language has its own build configurations and SDKs.
* Scripts may need to have cross-platform compatibility (i.e. they may need to be run on Mac, while in most cases they would run on Linux when on CI)
* As a counter-example of the above issue, scripts may need to be run on the specific environment only (i.e. you'd need to start a Docker container to invoke the script)
* Scripts may need to be run in a specific order (i.e. the build script needs to be run before the deployment script)
* Sometimes build scripts can aim to install additional SDKs or libraries that are not available in the base Docker image. This
  can lead to cross-platform issues (described above) as well as permission issues, since containers aren't always running as root.

### Dockerfile/Buildkit is good enough?
For some trivial services, Dockerfile provides the easy way of building and testing the build of a service locally.
However, you face the following issues when you want to use Docker as the entry-point to your service's build:
* Dockerfile allows to build a specific Docker image only, while your application may need to produce many
* It is not easy to provide secrets to the Docker build, even using Buildkit. Normally you'd want to provide local
  Maven/Gradle/NPM configs to get access to internal repositories.
* It is even harder to run integration tests from Docker build especially if they require Docker themselves to be run.
  For example, you may need to use TestContainers to start-up a database or any other service when you run ITs.
* To actually run the build of docker image, you'd need to use scripts to provide all the necessary environment and configurations.
  This is not ideal, as it is not very declarative.

### Docker Compose?
Docker Compose allows to solve the declarative issues of Dockerfile and Buildkit. However it is not a generic run/build
tool. It can be used for similar purposes, but it is hard to use it because it is geared towards describing an
application and its dependencies and starting this whole stack, rather than building it and running tests.

## Use cases
For the detailed use-cases of what you can do with Welder, please read [Howto](/howto/) section.
