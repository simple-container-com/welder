---
title: 'Improving performance'
description: 'How you can improve performance of your Welder tasks'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Performance tuning

Docker is known to be quite slow when running on MacOS due to its Linux nature and the fact that it is running via
virtual machine. Here is the list of what Welder offers in attempt to improve this experience.

## Reuse build containers

Given starting a build container can be time-consuming, and you may want to incrementally test your changes by running
the build or some other tasks that require SDKs and tools within a container, it is possible to keep the container 
running and re-use it for the next build. Simply add the `--reuse-containers` (or short `-R`) flag to any Welder command.

```bash
welder make -R
```

The subsequent execution of the build will reuse the existing build containers (which will significantly reduce the init time).

## "On-host" mode

Even though Welder's main goal is to provide the easy way of running tasks in containers, sometimes you simply want
to execute commands specified in your build configuration without Docker overhead. This can be achieved by adding the
`--on-host` (or short `-H`) flag to any Welder command.

```bash
welder make --on-host
```

This will override `image` option for all build steps and simply invoke all of them on the host machine. 

## Parallel mode

Welder allows to run build for multiple modules in parallel. For this purpose there are `--parallel` and `--parallel-count` flags.
The first one enables parallel builds and the second controls how many concurrent builds can be run at the same time.

```bash
# this will run the build with concurrency of 2
welder make --parallel --parallel-count=2
```

## Volume synchronization modes

Welder configures Docker containers in the way that they are able to share volumes with the host.
In addition to the volume synchronization modes provided by Docker, Welder introduces additional modes that 
ensure the greater portability of the build configuration. Some of these modes can be useful for better performance and
others are useful to guarantee the ability to work in various environments.

### bind (default)
This is the normal mode that is used primarily for development and testing on localhost (where Docker is installed on the 
same machine). It is the default mode for Docker containers and works well in most cases. However, on some systems it can
be quite slow, which makes builds running in Docker containers take a lot longer than they would on a localhost.

### copy and add (max portability, min performance)

These modes were introduced to guarantee functioning across CI/CD environments. When `add` mode is enabled, contents of 
the volumes are copied from the host to the container while building an intermediate build image and then the changed 
files in the container are copied back to the host. `copy` forces Welder to always copy the contents of the volumes
without adding `ADD` directives to the intermediate build image.

### volume (better performance on MacOS)

This mode is used when you want to synchronize volumes in a separate process (e.g. mutagen). In some cases this may greatly
increase the performance of your builds within containers due to asynchronous synchronization of the files. 
It assumes that there is a constant sync process between the host and the container via a volume. This is especially
useful on MacOS where Docker is running on a separate file system (within the virtual machine).

Volumes for this mode have to be pre-created and synced to the container beforehand:
```bash
on-host:~$ welder volumes --parallel --watch
```

This command will create temporary Docker containers and attach volumes to them. It will also start Mutagen process to 
continuously sync the contents of the volumes.
Once the above command stops producing the output, it means the synchronization is done and build can be started 
with the `--volume-mode=volume` flag.
