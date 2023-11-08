//go:build tools
// +build tools

// this file references indirect dependencies that are used during the build

package main

import (
	_ "github.com/atombender/go-jsonschema/pkg/generator"
	_ "github.com/go-bindata/go-bindata/v3"
	_ "github.com/magefile/mage/mage"
)
