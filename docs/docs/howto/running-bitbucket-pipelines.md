---
title: 'Running Bitbucket Pipelines'
description: 'How to use Welder for running Bitbucket Pipelines locally'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Running Bitbucket Pipelines locally

If you don't see the value in having an additional `welder.yaml` file, you can still use Welder for running
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
BBP environment and run the pipeline locally. 

!!! warning
    This feature is currently experimental and may not work in all cases.
