---
title: 'Generating CI/CD pipelines'
description: 'Description of how to use Welder for generating of CI/CD configurations'
platform: platform
product: welder
category: devguide
subcategory: learning
guides: tutorials
date: '2022-08-22'
---

# Generating CI/CD pipelines with Welder

## Generating Bamboo Specs

```bash
welder bamboo generate
```

This will generate directory `bamboo-specs` directory with Bamboo specs that are ready to be imported in Bamboo to generate
CI/CD pipelines according to definitions from `welder.yaml` file.

## Generating Bitbucket Pipelines

```bash
welder bitbucket-pipelines generate
```

This will generate directory `bitbucket-pipelines.yml` file according to definitions from `welder.yaml` file.

{{% note %}}
Generated CI/CD configurations will still run Welder under the hood. You may still want to make some manual changes 
to the generated files or extend them with your own customizations.
{{% /note %}}

