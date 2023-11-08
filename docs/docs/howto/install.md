---
title: 'Installation'
description: 'How to install and use Welder'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# How to install and start using Welder

## Installation

```bash
on-host:~$ curl -s "https://welder.simple-container.com/welder.sh" | bash
```

To initialize Welder for your project, you need to run the following command:
```bash
on-host:~$ welder init
```

The wizard will get you through a few simple steps to set you up with your service for use with Welder. 
Follow the instructions to configure `welder.yaml` descriptor.


## IntelliJ Plugin

It is strongly recommended to install the [Welder IntelliJ Plugin](/intellij-idea-plugin) as well
if you use IntelliJ IDEA as the main development IDE. The plugin adds validation, autocompletion and navigation within
`welder.yaml` descriptors.

## Simple usage

The simplest possible usage of Welder is to run a command within a container.

```yaml
schemaVersion: 1.8.0
tasks:
  test:
    image: alpine
    script:
      - uname -a
```
When you run it with `welder run test`, you should see something like:

```
on-host:~$ welder run test 
run [test]  - Running 1 scripts in container 'alpine'...
run [test] Linux 5b8ac2bbb2a6 5.4.0-122-generic #138-Ubuntu SMP Wed Jun 22 15:00:31 UTC 2022 x86_64 Linux
run [test]  - Finished test in 5s
```

Please see [Simple usage](/howto/simple-usage/) for more information.
