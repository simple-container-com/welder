---
title: 'Debugging build containers'
description: 'The way you can investigate issues with build containers'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Debugging build containers

## "Verbose" mode

Verbose mode is a way to see the debug information of Welder lifecycle, it prints out the most important messages, 
such as container configuration and all commands (including service ones) it runs inside it.

```bash
on-host:~$ welder make --verbose
```

## "Shell" mode

You can use "shell" mode to instantiate build container and open active terminal inside it to run commands manually.
This approach allows to debug certain commands or investigate issues with build containers and environment.

```bash
on-host:~$ welder run shell bash -t build 
in-container:~$ mvn clean install
...
```

`-t` parameter is used to tell Welder which task's definition should be used to create build container. Alternatively
you may want to use `-m` to specify which module you'd like to use as a base for the build container's config.
