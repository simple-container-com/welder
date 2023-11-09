## Welder

    One Tool to rule them all,
       one Tool to find them,
    One Tool to bring them all
       and in the containers bind them

### Motivation

Welder is a CLI tool that allows running multi-module multi-SDK builds with the extensive use of Docker containers.
The goal of Welder is to provide seamless build and deploy experience across environments, such as local and CI/CD.

It is focused on the build and deploy lifecycle of an application, trying to make the following operations easier:

* Build application using preferred SDK
* Build Docker images for the application
* Deploy application with the chosen deploy strategy

#### Dockerfile/Buildkit is good enough?

* Dockerfile allows to build a specific Docker image, while your application may need to produce many
* It is hard to provide secrets to the Docker build, even using Buildkit
* It is even harder to run integration tests from Docker build especially if they require Docker themselves

#### Docker Compose?
Docker Compose is not a generic run/build tool. While it can be used for similar purposes, it is hard to use it because
it is geared towards describing an application and its dependencies and starting this whole stack. 

#### Batect?
There are a few tools that work similar to Welder. For example [Batect](https://batect.dev) allows to define 
arbitrary containers and tasks that should be executed inside of them. While the proper comparision of Welder to 
Batect is still a TODO, the [comparision of Batect to other tools](https://batect.dev/Comparison.html) provides
some thoughts for motivation.

### How to build 

#### Prerequisites

* [Docker](https://docs.docker.com/install/) 
* Set-up Go build environment

#### Building

This project uses [Mage](https://magefile.org) as a build tool, so the main requirement is a Golang.

**Using Golang:**
```
# Install tools
cat tools.go | grep _ | awk -F'"' '{print $2}' | xargs -tI % go get %
# Run build
go run -mod=mod build.go build:all
```

**Using Welder:**
```
welder make
```