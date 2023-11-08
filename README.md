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

#### Publishing

Some of this code in this repo is used by Atlas CLI Plugins (and possibly other projects).
To use the newest version of Welder as a dependency elsewhere, you must add a new version tag manually.
The version that you use should match the one generated in `welder.yaml`.

Publishing a version:
1. Determine the version. You can figure it out manually or grab it from the BB Pipeline execution:
   1. Locate the BB Pipeline execution of the `master` branch that ran when you merged your change
   2. Search for any part of the version that you know.
   You should be able to locate a line that contains it, such as:
   ```
   Executing '/bin/bash "-c" "go build -ldflags "-X main.Version=1.7.864-ea1a419"  -tags=osusergo ...
   ```
2. Copy the version (e.g. `1.7.864-ea1a419` for the example above)
3. Tag the version:
   ```
   git checkout master
   git pull
   git log # verify that the version you're tagging corresponds to the most recent commit
   git tag <version-you-copied>
   git push origin <version-you-copied>
   ```

You should now see your version in the list of git tags for Welder in the Bitbucket UI.

Use `go get -u github.com/simple-container-com/welder` in Atlas CLI Plugins (or elsewhere) to use this new version.
